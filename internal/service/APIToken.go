package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	models "emplacc-api/internal/domain"
	"emplacc-api/internal/ports"

	"github.com/google/uuid"
)

type CreateTokenResult struct {
	ID        string `json:"id"`
	PlainText string `json:"token"` // показывается только при создании
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

type TokenInfo struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	CreatedAt  string  `json:"created_at"`
	LastUsedAt *string `json:"last_used_at,omitempty"`
	ExpiresAt  *string `json:"expires_at,omitempty"`
}

type APITokenService interface {
	Create(userID uuid.UUID, name string, expiresIn *time.Duration) (*CreateTokenResult, error)
	ListByUser(userID uuid.UUID) ([]TokenInfo, error)
	Revoke(userID uuid.UUID, tokenID uuid.UUID) error
	ValidateToken(plain string) (*models.User, error)
}

type apiTokenService struct {
	repo     ports.APITokenRepository
	userRepo ports.UserRepository
}

func NewAPITokenService(repo ports.APITokenRepository, userRepo ports.UserRepository) APITokenService {
	return &apiTokenService{repo: repo, userRepo: userRepo}
}

func generateToken() (plain, hash string, err error) {
	raw := make([]byte, 32)
	if _, err = rand.Read(raw); err != nil {
		return
	}
	plain = "emplacc_" + hex.EncodeToString(raw)
	sum := sha256.Sum256([]byte(plain))
	hash = hex.EncodeToString(sum[:])
	return
}

func (s *apiTokenService) Create(userID uuid.UUID, name string, expiresIn *time.Duration) (*CreateTokenResult, error) {
	plain, hash, err := generateToken()
	if err != nil {
		return nil, fmt.Errorf("generate token: %w", err)
	}

	now := time.Now()
	t := models.APIToken{
		ID:        uuid.New(),
		UserID:    userID,
		Name:      name,
		TokenHash: hash,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if expiresIn != nil {
		exp := now.Add(*expiresIn)
		t.ExpiresAt = &exp
	}

	if err := s.repo.Create(t); err != nil {
		return nil, err
	}

	return &CreateTokenResult{
		ID:        t.ID.String(),
		PlainText: plain,
		Name:      name,
		CreatedAt: now.Format(time.RFC3339),
	}, nil
}

func (s *apiTokenService) ListByUser(userID uuid.UUID) ([]TokenInfo, error) {
	tokens, err := s.repo.ListByUser(userID)
	if err != nil {
		return nil, err
	}
	result := make([]TokenInfo, 0, len(tokens))
	for _, t := range tokens {
		info := TokenInfo{
			ID:        t.ID.String(),
			Name:      t.Name,
			CreatedAt: t.CreatedAt.Format(time.RFC3339),
		}
		if t.LastUsedAt != nil {
			s := t.LastUsedAt.Format(time.RFC3339)
			info.LastUsedAt = &s
		}
		if t.ExpiresAt != nil {
			s := t.ExpiresAt.Format(time.RFC3339)
			info.ExpiresAt = &s
		}
		result = append(result, info)
	}
	return result, nil
}

func (s *apiTokenService) Revoke(userID uuid.UUID, tokenID uuid.UUID) error {
	return s.repo.Revoke(userID, tokenID)
}

func (s *apiTokenService) ValidateToken(plain string) (*models.User, error) {
	sum := sha256.Sum256([]byte(plain))
	hash := hex.EncodeToString(sum[:])

	t, err := s.repo.FindByHash(hash)
	if err != nil {
		return nil, errors.New("token not found")
	}
	if t.ExpiresAt != nil && time.Now().After(*t.ExpiresAt) {
		return nil, errors.New("token expired")
	}

	// Обновляем last_used_at
	now := time.Now()
	_ = s.repo.UpdateLastUsed(t.ID, now)

	user, err := s.userRepo.GetUserById(t.UserID)
	if err != nil {
		return nil, errors.New("user not found")
	}
	return user, nil
}
