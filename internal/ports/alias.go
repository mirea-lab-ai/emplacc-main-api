package ports

import (
	models "emplacc-api/internal/domain"

	"github.com/google/uuid"
)

type UserAliasRepository interface {
	ListByOwner(ownerID uuid.UUID) ([]models.UserAlias, error)
	Create(alias models.UserAlias) error
	Delete(id, ownerID uuid.UUID) (bool, error)
	ExistsByOwnerAlias(ownerID uuid.UUID, alias string) (bool, error)
}
