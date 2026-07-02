// Package storage is the outbound adapter for S3-compatible object storage
// (MinIO / rustfs). It implements ports.StoragePort; nothing above the adapter
// layer imports the minio driver.
package storage

import (
	"context"
	"fmt"
	"mime/multipart"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"emplacc-api/internal/ports"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// presignedURLExpiry — максимум по S3 Signature V4: 7 дней (604800 сек).
// Для долгосрочного хранения используй /media/refresh endpoint.
const presignedURLExpiry = 7 * 24 * time.Hour

// presignCacheTTL — как долго переиспользуем один и тот же presigned URL для объекта.
// В пределах окна ссылка стабильна → браузер кэширует картинку (не перекачивает на
// каждый запрос), а подпись не пересчитывается зря. Сама ссылка валидна 7 дней.
const presignCacheTTL = time.Hour

type presignEntry struct {
	url string
	exp time.Time
}

type minioStorage struct {
	client        *minio.Client // внутренний endpoint для загрузки объектов
	presignClient *minio.Client // публичный endpoint для генерации presigned URL
	bucket        string
	useSSL        bool
	presignCache  sync.Map // objectPath -> presignEntry
}

// New constructs the MinIO-backed storage adapter from S3_* env vars.
func New() (ports.StoragePort, error) {
	endpoint := os.Getenv("S3_ENDPOINT")
	accessKey := os.Getenv("S3_ACCESS_KEY")
	secretKey := os.Getenv("S3_SECRET_KEY")
	bucket := os.Getenv("S3_BUCKET")
	publicURL := os.Getenv("S3_PUBLIC_URL")
	useSSL := os.Getenv("S3_USE_SSL") == "true"

	if endpoint == "" || accessKey == "" || secretKey == "" || bucket == "" {
		return nil, fmt.Errorf("S3_ENDPOINT, S3_ACCESS_KEY, S3_SECRET_KEY, S3_BUCKET must be set")
	}

	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("minio client: %w", err)
	}

	ctx := context.Background()
	exists, err := client.BucketExists(ctx, bucket)
	if err != nil {
		return nil, fmt.Errorf("bucket check: %w", err)
	}
	if !exists {
		if err := client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
			return nil, fmt.Errorf("make bucket: %w", err)
		}
	}

	// Удаляем публичную политику bucket — доступ только через presigned URLs
	if err := client.SetBucketPolicy(ctx, bucket, ""); err != nil {
		// Не фатально, логируем но продолжаем
		_ = err
	}

	if publicURL == "" {
		scheme := "http"
		if useSSL {
			scheme = "https"
		}
		publicURL = fmt.Sprintf("%s://%s", scheme, endpoint)
	}

	// Второй клиент для presigned URLs — использует публичный endpoint
	// чтобы подпись совпадала с hostname в URL (S3 подписывает Host header)
	var presignClient *minio.Client
	if publicURL != "" {
		pubEndpoint := strings.TrimRight(publicURL, "/")
		pubEndpoint = strings.TrimPrefix(pubEndpoint, "https://")
		pubEndpoint = strings.TrimPrefix(pubEndpoint, "http://")
		if idx := strings.Index(pubEndpoint, "/"); idx >= 0 {
			pubEndpoint = pubEndpoint[:idx]
		}
		pubSSL := strings.HasPrefix(publicURL, "https://")
		presignClient, _ = minio.New(pubEndpoint, &minio.Options{
			Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
			Secure: pubSSL,
		})
	}
	if presignClient == nil {
		presignClient = client
	}

	return &minioStorage{client: client, presignClient: presignClient, bucket: bucket, useSSL: useSSL}, nil
}

func (s *minioStorage) RefreshURL(objectPath string) (string, error) {
	// Кэш на presignCacheTTL: в пределах окна возвращаем ту же ссылку (браузер кэширует
	// картинку, подпись не считается заново на каждый запрос).
	if v, ok := s.presignCache.Load(objectPath); ok {
		if e, ok := v.(presignEntry); ok && time.Now().Before(e.exp) {
			return e.url, nil
		}
	}
	// presignClient настроен на публичный endpoint — подпись валидна для публичного URL
	presigned, err := s.presignClient.PresignedGetObject(
		context.Background(),
		s.bucket,
		objectPath,
		presignedURLExpiry,
		url.Values{},
	)
	if err != nil {
		return "", fmt.Errorf("presign: %w", err)
	}
	u := presigned.String()
	s.presignCache.Store(objectPath, presignEntry{url: u, exp: time.Now().Add(presignCacheTTL)})
	return u, nil
}

func (s *minioStorage) FreshAvatarURL(storedValue string) string {
	if storedValue == "" {
		return ""
	}
	objectPath := storedValue
	// Если это старый presigned URL (содержит X-Amz-Signature) — извлекаем путь
	if strings.Contains(storedValue, "X-Amz-Signature") {
		u, err := url.Parse(storedValue)
		if err != nil {
			return ""
		}
		parts := strings.SplitN(strings.TrimPrefix(u.Path, "/"), "/", 2)
		if len(parts) < 2 {
			return ""
		}
		objectPath = parts[1]
	}
	fresh, err := s.RefreshURL(objectPath)
	if err != nil {
		return ""
	}
	return fresh
}

func (s *minioStorage) UploadFile(file multipart.File, header *multipart.FileHeader, folder string) (string, string, error) {
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext == "" {
		ext = ".bin"
	}
	objectName := fmt.Sprintf("%s/%s_%d%s", folder, uuid.New().String(), time.Now().UnixMilli(), ext)

	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	_, err := s.client.PutObject(
		context.Background(),
		s.bucket,
		objectName,
		file,
		header.Size,
		minio.PutObjectOptions{ContentType: contentType},
	)
	if err != nil {
		return "", "", fmt.Errorf("put object: %w", err)
	}

	presignedURL, err := s.RefreshURL(objectName)
	if err != nil {
		return "", "", err
	}

	return presignedURL, objectName, nil
}
