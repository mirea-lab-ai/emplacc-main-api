package service

import (
	"errors"
	"strings"
	"time"

	models "emplacc-api/internal/domain"
	"emplacc-api/internal/ports"

	"github.com/google/uuid"
)

type UserAliasService interface {
	List(ownerID uuid.UUID) ([]models.UserAlias, error)
	Create(ownerID, targetID uuid.UUID, alias string) (*models.UserAlias, error)
	Delete(ownerID, id uuid.UUID) (bool, error)
}

type userAliasService struct {
	repo ports.UserAliasRepository
}

func NewUserAliasService(repo ports.UserAliasRepository) UserAliasService {
	return &userAliasService{repo: repo}
}

func (s *userAliasService) List(ownerID uuid.UUID) ([]models.UserAlias, error) {
	return s.repo.ListByOwner(ownerID)
}

func (s *userAliasService) Create(ownerID, targetID uuid.UUID, alias string) (*models.UserAlias, error) {
	alias = strings.TrimSpace(alias)
	alias = strings.TrimPrefix(alias, "@")
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return nil, errors.New("псевдоним пустой")
	}
	if len([]rune(alias)) > 50 {
		return nil, errors.New("псевдоним слишком длинный (макс. 50)")
	}
	if strings.ContainsAny(alias, " \t\n\r") {
		return nil, errors.New("псевдоним не должен содержать пробелов")
	}
	if targetID == uuid.Nil {
		return nil, errors.New("не указан целевой пользователь")
	}
	exists, err := s.repo.ExistsByOwnerAlias(ownerID, alias)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, errors.New("такой псевдоним у вас уже есть")
	}
	m := models.UserAlias{
		ID:           uuid.New(),
		OwnerUserID:  ownerID,
		Alias:        alias,
		TargetUserID: targetID,
		CreatedAt:    time.Now(),
	}
	if err := s.repo.Create(m); err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *userAliasService) Delete(ownerID, id uuid.UUID) (bool, error) {
	return s.repo.Delete(id, ownerID)
}
