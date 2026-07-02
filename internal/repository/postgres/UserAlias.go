package postgres

import (
	models "emplacc-api/internal/domain"
	"emplacc-api/internal/ports"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type userAliasRepository struct {
	db *gorm.DB
}

func NewUserAliasRepository(db *gorm.DB) ports.UserAliasRepository {
	return &userAliasRepository{db: db}
}

func (r *userAliasRepository) ListByOwner(ownerID uuid.UUID) ([]models.UserAlias, error) {
	var aliases []models.UserAlias
	err := r.db.Session(&gorm.Session{NewDB: true}).
		Model(&models.UserAlias{}).
		Preload("Target").
		Where("owner_user_id = ?", ownerID).
		Order("alias asc").
		Find(&aliases).Error
	return aliases, err
}

func (r *userAliasRepository) Create(alias models.UserAlias) error {
	return r.db.Session(&gorm.Session{NewDB: true}).Create(&alias).Error
}

func (r *userAliasRepository) Delete(id, ownerID uuid.UUID) (bool, error) {
	res := r.db.Session(&gorm.Session{NewDB: true}).
		Where("id = ? AND owner_user_id = ?", id, ownerID).
		Delete(&models.UserAlias{})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

func (r *userAliasRepository) ExistsByOwnerAlias(ownerID uuid.UUID, alias string) (bool, error) {
	var count int64
	err := r.db.Session(&gorm.Session{NewDB: true}).
		Model(&models.UserAlias{}).
		Where("owner_user_id = ? AND alias = ?", ownerID, alias).
		Count(&count).Error
	return count > 0, err
}
