package ports

import (
	"time"
)

// SessionData is the value persisted/returned by SessionRepository.
// It is part of the SessionRepository port contract.
type SessionData struct {
	UserID            string
	ExpiresAt         time.Time
	AbsoluteExpiresAt time.Time
	ExpireReason      string
}

type SessionRepository interface {
	Save(hash string, data SessionData) error
	Find(hash string) (*SessionData, error)
	Rotate(hash string, newExpiresAt time.Time) error
	Expire(hash string, reason string) error
	Delete(hash string) error
}
