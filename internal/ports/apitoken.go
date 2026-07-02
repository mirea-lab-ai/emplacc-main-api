package ports

import (
	"time"

	models "emplacc-api/internal/domain"

	"github.com/google/uuid"
)

type APITokenRepository interface {
	Create(t models.APIToken) error
	ListByUser(userID uuid.UUID) ([]models.APIToken, error)
	FindByHash(hash string) (*models.APIToken, error)
	Revoke(userID uuid.UUID, tokenID uuid.UUID) error
	UpdateLastUsed(tokenID uuid.UUID, at time.Time) error
}
