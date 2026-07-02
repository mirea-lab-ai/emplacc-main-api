package ports

import "mime/multipart"

// StoragePort — выходной порт объектного хранилища (S3/MinIO/rustfs).
// Реализация-адаптер живёт в internal/infra/storage.
type StoragePort interface {
	UploadFile(file multipart.File, header *multipart.FileHeader, folder string) (string, string, error)
	RefreshURL(objectPath string) (string, error)
	// FreshAvatarURL returns a fresh presigned URL for the stored value.
	// Accepts either an object path ("avatars/uuid.jpg") or a legacy
	// presigned URL containing X-Amz-Signature (auto-extracts the path).
	// Returns "" on error or empty input.
	FreshAvatarURL(storedValue string) string
}
