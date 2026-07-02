package service

import (
	models "emplacc-api/internal/domain"
	"emplacc-api/internal/dto/request"
	"emplacc-api/internal/dto/response"
	"emplacc-api/internal/ports"
	"emplacc-api/internal/utils"
	"errors"
	"time"

	"github.com/google/uuid"
)

type UserService interface {
	GetAllUsers(page, pageSize int) ([]models.User, int64, error)
	SearchUsers(query string, page, pageSize int) ([]models.User, int64, error)
	GetUserById(userId uuid.UUID) (*models.User, error)
	CreateUser(req request.UserCreateRequest) (uuid.UUID, error)
	UpdateUser(userId uuid.UUID, req request.UpdateUserRequest) error
	UpdateAvatarURL(userId uuid.UUID, avatarURL string) error
	DeleteUser(userId uuid.UUID) error
	BanUser(userId uuid.UUID) error
	RestoreUser(req request.RestoreUserRequest) (uuid.UUID, error)
	AddUserRole(req request.AddRoleUserRequest) (response.AddRoleUserResponse, error)
	RemoveUserRole(req request.RemoveRoleUserRequest) (response.RemoveRoleUserResponse, error)
	CreateSystemUser() (uuid.UUID, error)
}

type userService struct {
	repo ports.UserRepository
}

func NewUserService(repo ports.UserRepository) UserService {
	return &userService{
		repo: repo,
	}
}

func (s *userService) SearchUsers(query string, page, pageSize int) ([]models.User, int64, error) {
	offset := (page - 1) * pageSize
	return s.repo.SearchUsers(query, pageSize, offset)
}

func (s *userService) CreateSystemUser() (uuid.UUID, error) {
	// Идемпотентно: если системный пользователь уже есть — возвращаем его ID
	existing, err := s.repo.GetUserByEmail("system@system")
	if err == nil && existing != nil {
		return existing.ID, nil
	}

	newUUID := uuid.New()
	now := time.Now()
	user := models.User{
		ID:            newUUID,
		Email:         "system@system",
		IsActive:      true,
		CreatedAt:     now,
		UpdatedAt:     now,
		EmailVerified: true,
		FirstName:     "Системный",
		LastName:      "Пользователь",
		LastLogin:     now,
		Deleted:       false,
	}

	if err := s.repo.CreateUser(user); err != nil {
		return uuid.Nil, err
	}
	return newUUID, nil
}

func (s *userService) GetAllUsers(page, pageSize int) ([]models.User, int64, error) {
	offset := (page - 1) * pageSize
	return s.repo.GetAllUsers(pageSize, offset)
}

func (s *userService) GetUserById(userId uuid.UUID) (*models.User, error) {
	return s.repo.GetUserById(userId)
}

func (s *userService) CreateUser(req request.UserCreateRequest) (uuid.UUID, error) {
	newUUID := uuid.New()
	now := time.Now()

	user := models.User{
		ID:            newUUID,
		Email:         utils.GetString(req.Email),
		IsActive:      utils.GetBool(req.IsActive),
		CreatedAt:     now,
		UpdatedAt:     now,
		EmailVerified: utils.GetBool(req.EmailVerified),
		FirstName:     utils.GetString(req.FirstName),
		LastName:      utils.GetString(req.LastName),
		LastLogin:     now,
		Deleted:       false,
		TgID:          "",
		TgUserID:      0,
		Profession:    "",
		UserRoles:     nil,
	}

	err := s.repo.CreateUser(user)
	if err != nil {
		return uuid.Nil, err
	}

	return newUUID, nil
}

func (s *userService) UpdateUser(userId uuid.UUID, req request.UpdateUserRequest) error {
	updateData := make(map[string]interface{})
	if req.Email != nil {
		updateData["email"] = *req.Email
	}
	if req.IsActive != nil {
		updateData["is_active"] = *req.IsActive
	}
	if req.TgId != nil {
		updateData["tg_id"] = *req.TgId
	}
	if req.TgUserId != nil {
		updateData["tg_user_id"] = *req.TgUserId
	}
	if req.Profession != nil {
		updateData["profession"] = *req.Profession
	}
	if req.EmailVerified != nil {
		updateData["email_verified"] = *req.EmailVerified
	}
	if req.FirstName != nil {
		updateData["first_name"] = *req.FirstName
	}
	if req.LastName != nil {
		updateData["last_name"] = *req.LastName
	}
	if req.LastLogin != nil {
		updateData["last_login"] = *req.LastLogin
	}

	if len(updateData) == 0 {
		return errors.New("no fields to update")
	}

	updateData["updated_at"] = time.Now()

	updated, err := s.repo.UpdateUser(userId, updateData)
	if err != nil {
		return err
	}

	if !updated {
		return errors.New("user not found")
	}

	return nil
}

func (s *userService) DeleteUser(userId uuid.UUID) error {
	deleted, err := s.repo.DeleteUser(userId)
	if err != nil {
		return err
	}

	if !deleted {
		return errors.New("user not found")
	}

	return nil
}

func (s *userService) BanUser(userId uuid.UUID) error {
	deleted, err := s.repo.BanUser(userId)
	if err != nil {
		return err
	}

	if !deleted {
		return errors.New("user not found")
	}

	return nil
}

func (s *userService) RestoreUser(req request.RestoreUserRequest) (uuid.UUID, error) {
	userId, err := s.repo.RestoreUser(req)
	if err != nil {
		return uuid.Nil, err
	}

	return userId, nil
}

func (s *userService) AddUserRole(req request.AddRoleUserRequest) (response.AddRoleUserResponse, error) {
	user, err := s.repo.GetUser(req.UserId)
	if err != nil {
		return response.AddRoleUserResponse{}, err
	}

	role, err := s.repo.GetRole(req.RoleId)
	if err != nil {
		return response.AddRoleUserResponse{}, err
	}

	assignerId, err := uuid.Parse(req.AssignerId)
	if err != nil {
		return response.AddRoleUserResponse{}, err
	}

	now := time.Now()
	del := false

	userRole := models.UserRole{
		UserID:     user.ID,
		RoleID:     role.ID,
		AssignedAt: &now,
		Deleted:    &del,
		AssignedBy: &assignerId,
	}

	err = s.repo.CreateUserRole(userRole)
	if err != nil {
		return response.AddRoleUserResponse{}, err
	}

	addResponse := response.AddRoleUserResponse{
		UserId:  user.ID.String(),
		RoleId:  role.ID.String(),
		Message: "Роль успешно добавлена",
	}

	return addResponse, nil
}

func (s *userService) RemoveUserRole(req request.RemoveRoleUserRequest) (response.RemoveRoleUserResponse, error) {
	user, err := s.repo.GetUser(req.UserId)
	if err != nil {
		return response.RemoveRoleUserResponse{}, err
	}

	role, err := s.repo.GetRole(req.RoleId)
	if err != nil {
		return response.RemoveRoleUserResponse{}, err
	}

	deleted, err := s.repo.RemoveUserRole(user.ID, role.ID)
	if err != nil {
		return response.RemoveRoleUserResponse{}, err
	}

	if !deleted {
		return response.RemoveRoleUserResponse{}, errors.New("user role not found")
	}

	deleteResponse := response.RemoveRoleUserResponse{
		RoleId:  role.ID.String(),
		UserId:  user.ID.String(),
		Message: "Роль успешно удалена",
	}

	return deleteResponse, nil
}
func (s *userService) UpdateAvatarURL(userId uuid.UUID, avatarURL string) error {
	_, err := s.repo.UpdateUser(userId, map[string]interface{}{"avatar_url": avatarURL})
	return err
}
