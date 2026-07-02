package ports

import (
	models "emplacc-api/internal/domain"
	"emplacc-api/internal/dto/request"

	"github.com/google/uuid"
)

type UserRepository interface {
	GetAllUsers(limit, offset int) ([]models.User, int64, error)
	SearchUsers(query string, limit, offset int) ([]models.User, int64, error)
	GetUserById(userId uuid.UUID) (*models.User, error)
	GetUserByEmail(email string) (*models.User, error)
	CreateUser(user models.User) error
	UpdateUser(userId uuid.UUID, updateData map[string]interface{}) (bool, error)
	DeleteUser(userId uuid.UUID) (bool, error)
	BanUser(userId uuid.UUID) (bool, error)
	RestoreUser(req request.RestoreUserRequest) (uuid.UUID, error)
	GetUser(userID string) (*models.User, error)
	GetRole(roleID string) (*models.Role, error)
	CreateUserRole(userRole models.UserRole) error
	RemoveUserRole(userID uuid.UUID, roleID uuid.UUID) (bool, error)
	CreateUserWithID(req request.UserCreateRequest, userID uuid.UUID) error
}
