package service

import (
	models "emplacc-api/internal/domain"
	"emplacc-api/internal/dto/request"
	"emplacc-api/internal/ports"
	"errors"
	"time"

	"github.com/google/uuid"
)

type RoleService interface {
	GetAllRoles(page, pageSize int) ([]models.Role, int64, error)
	GetRoleById(roleID uuid.UUID) (*models.Role, error)
	CreateRole(req request.RoleCreateRequest) (uuid.UUID, error)
	UpdateRole(roleID uuid.UUID, req request.RoleUpdateRequest) error
	DeleteRole(roleID uuid.UUID) error
	GetRoleByUserId(userID uuid.UUID) (*models.Role, error)
}

type roleService struct {
	repo ports.RoleRepository
}

func NewRoleService(repo ports.RoleRepository) RoleService {
	return &roleService{
		repo: repo,
	}
}

func (s *roleService) GetAllRoles(page, pageSize int) ([]models.Role, int64, error) {
	offset := (page - 1) * pageSize
	return s.repo.GetAllRoles(pageSize, offset)
}

func (s *roleService) GetRoleById(roleID uuid.UUID) (*models.Role, error) {
	return s.repo.GetRoleById(roleID)
}

func (s *roleService) CreateRole(req request.RoleCreateRequest) (uuid.UUID, error) {
	now := time.Now()
	del := false
	role := models.Role{
		ID:          uuid.New(),
		Name:        req.Name,
		Description: req.Description,
		Deleted:     &del,
		CreatedAt:   &now,
	}

	err := s.repo.CreateRole(role)
	if err != nil {
		return uuid.Nil, err
	}

	return role.ID, nil
}

func (s *roleService) UpdateRole(roleID uuid.UUID, req request.RoleUpdateRequest) error {
	update := make(map[string]interface{})
	if req.Name != nil {
		update["name"] = *req.Name
	}
	if req.Description != nil {
		update["description"] = *req.Description
	}
	if len(update) == 0 {
		return errors.New("no fields to update")
	}
	now := time.Now()
	update["updated_at"] = &now

	updated, err := s.repo.UpdateRole(roleID, update)
	if err != nil {
		return err
	}

	if !updated {
		return errors.New("role not found")
	}

	return nil
}

func (s *roleService) DeleteRole(roleID uuid.UUID) error {
	deleted, err := s.repo.DeleteRole(roleID)
	if err != nil {
		return err
	}

	if !deleted {
		return errors.New("role not found")
	}

	return nil
}

func (s *roleService) GetRoleByUserId(userID uuid.UUID) (*models.Role, error) {
	return s.repo.GetRoleByUserId(userID)
}
