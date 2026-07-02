package ports

import (
	models "emplacc-api/internal/domain"

	"github.com/google/uuid"
)

type RoleRepository interface {
	GetAllRoles(limit, offset int) ([]models.Role, int64, error)
	GetRoleById(roleID uuid.UUID) (*models.Role, error)
	CreateRole(role models.Role) error
	UpdateRole(roleID uuid.UUID, updateData map[string]interface{}) (bool, error)
	DeleteRole(roleID uuid.UUID) (bool, error)
	GetRoleByUserId(userID uuid.UUID) (*models.Role, error)
}
