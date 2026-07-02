package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"emplacc-api/internal/ports"
	"emplacc-api/internal/repository/redis"
)

const (
	SessionTokenTTL    = 3 * time.Hour
	SessionAbsoluteTTL = 7 * 24 * time.Hour
	SessionPrefix      = "sess_"
)

type SessionCreateResult struct {
	PlainToken        string    `json:"session_token"`
	ExpiresAt         time.Time `json:"expires_at"`
	AbsoluteExpiresAt time.Time `json:"absolute_expires_at"`
	UserID            string    `json:"user_id"`
}

type SessionValidation struct {
	UserID  string
	Expired bool
	Reason  string
}

type SessionService interface {
	Create(userID string) (*SessionCreateResult, error)
	Validate(plainToken string) (*SessionValidation, error)
	Rotate(plainToken string) (*SessionCreateResult, error)
	Expire(plainToken string, reason string) error
}

type sessionService struct {
	repo ports.SessionRepository
}

func NewSessionService(repo ports.SessionRepository) SessionService {
	return &sessionService{repo: repo}
}

func generateSessionToken() (plain, hash string, err error) {
	raw := make([]byte, 32)
	if _, err = rand.Read(raw); err != nil {
		return
	}
	plain = SessionPrefix + hex.EncodeToString(raw)
	sum := sha256.Sum256([]byte(plain))
	hash = hex.EncodeToString(sum[:])
	return
}

func tokenHash(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

func (s *sessionService) Create(userID string) (*SessionCreateResult, error) {
	plain, hash, err := generateSessionToken()
	if err != nil {
		return nil, fmt.Errorf("generate token: %w", err)
	}

	now := time.Now()
	data := ports.SessionData{
		UserID:            userID,
		ExpiresAt:         now.Add(SessionTokenTTL),
		AbsoluteExpiresAt: now.Add(SessionAbsoluteTTL),
		ExpireReason:      "",
	}

	if err := s.repo.Save(hash, data); err != nil {
		return nil, err
	}

	return &SessionCreateResult{
		PlainToken:        plain,
		ExpiresAt:         data.ExpiresAt,
		AbsoluteExpiresAt: data.AbsoluteExpiresAt,
		UserID:            userID,
	}, nil
}

func (s *sessionService) Validate(plainToken string) (*SessionValidation, error) {
	data, err := s.repo.Find(tokenHash(plainToken))
	if err != nil {
		return nil, err
	}

	now := time.Now()

	// Принудительное завершение (выход или долгое отсутствие)
	if data.ExpireReason != "" {
		return &SessionValidation{
			UserID:  data.UserID,
			Expired: true,
			Reason:  data.ExpireReason,
		}, nil
	}

	// Абсолютный срок (не должен быть достигнут пока Redis TTL не истёк, но на всякий случай)
	if now.After(data.AbsoluteExpiresAt) {
		_ = s.repo.Expire(tokenHash(plainToken), redis.ReasonLongAbsence)
		return &SessionValidation{
			UserID:  data.UserID,
			Expired: true,
			Reason:  redis.ReasonLongAbsence,
		}, nil
	}

	// Короткий срок истёк → нужна ротация
	if now.After(data.ExpiresAt) {
		return &SessionValidation{
			UserID:  data.UserID,
			Expired: true,
			Reason:  "", // нормальное истечение → ротация
		}, nil
	}

	return &SessionValidation{UserID: data.UserID, Expired: false}, nil
}

func (s *sessionService) Rotate(plainToken string) (*SessionCreateResult, error) {
	hash := tokenHash(plainToken)
	data, err := s.repo.Find(hash)
	if err != nil {
		return nil, err
	}
	if data.ExpireReason != "" {
		return nil, fmt.Errorf("session expired: %s", data.ExpireReason)
	}
	if time.Now().After(data.AbsoluteExpiresAt) {
		_ = s.repo.Expire(hash, redis.ReasonLongAbsence)
		return nil, fmt.Errorf("session absolute expired")
	}

	newExpiry := time.Now().Add(SessionTokenTTL)
	if err := s.repo.Rotate(hash, newExpiry); err != nil {
		return nil, err
	}

	return &SessionCreateResult{
		PlainToken:        plainToken,
		ExpiresAt:         newExpiry,
		AbsoluteExpiresAt: data.AbsoluteExpiresAt,
		UserID:            data.UserID,
	}, nil
}

func (s *sessionService) Expire(plainToken string, reason string) error {
	return s.repo.Expire(tokenHash(plainToken), reason)
}
