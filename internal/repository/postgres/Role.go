package postgres

import (
	models "emplacc-api/internal/domain"
	"emplacc-api/internal/ports"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type roleRepository struct {
	db *gorm.DB
}

func NewRoleRepository(db *gorm.DB) ports.RoleRepository {
	return &roleRepository{
		db: db,
	}
}

func (r *roleRepository) GetAllRoles(limit, offset int) ([]models.Role, int64, error) {
	var totalCount int64
	if err := r.db.Session(&gorm.Session{NewDB: true}).
		Table("roles").
		Where("roles.deleted = FALSE").
		Count(&totalCount).Error; err != nil {
		return nil, 0, err
	}

	var roles []models.Role
	if err := r.db.Session(&gorm.Session{NewDB: true}).
		Table("roles").
		Where("roles.deleted = FALSE").
		Limit(limit).Offset(offset).
		Find(&roles).Error; err != nil {
		return nil, 0, err
	}

	return roles, totalCount, nil
}

func (r *roleRepository) GetRoleByUserId(userID uuid.UUID) (*models.Role, error) {
	var role models.Role

	// JOIN между roles и user_roles через таблицу связей
	err := r.db.Session(&gorm.Session{NewDB: true}).Table("roles").Joins("JOIN user_roles ur ON ur.role_id = roles.id").
		Where("ur.user_id = ? AND ur.deleted = FALSE AND roles.deleted = FALSE", userID).
		First(&role).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("role not found for user")
		}
		return nil, err
	}

	return &role, nil
}

func (r *roleRepository) GetRoleById(roleID uuid.UUID) (*models.Role, error) {
	var role models.Role
	res := r.db.
		Model(&models.Role{}).
		Where("roles.id = ? AND roles.deleted = FALSE", roleID).
		First(&role)
	if res.Error != nil {
		if res.Error == gorm.ErrRecordNotFound {
			return nil, errors.New("role not found")
		}
		return nil, res.Error
	}

	return &role, nil
}

func (r *roleRepository) CreateRole(role models.Role) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		return tx.Session(&gorm.Session{NewDB: true}).
			Model(&models.Role{}).
			Omit(clause.Associations).
			Create(&role).Error
	})
}

func (r *roleRepository) UpdateRole(roleID uuid.UUID, updateData map[string]interface{}) (bool, error) {
	var affected int64
	err := r.db.Transaction(func(tx *gorm.DB) error {
		res := tx.Session(&gorm.Session{NewDB: true}).
			Model(&models.Role{}).
			Where("id = ? AND deleted = FALSE", roleID).
			Updates(updateData)
		if res.Error != nil {
			return res.Error
		}
		affected = res.RowsAffected
		return nil
	})

	if err != nil {
		return false, err
	}

	return affected > 0, nil
}

func (r *roleRepository) DeleteRole(roleID uuid.UUID) (bool, error) {
	delTrue := true
	now := time.Now()
	update := map[string]interface{}{"deleted": &delTrue, "updated_at": &now}

	var affected int64
	err := r.db.Transaction(func(tx *gorm.DB) error {
		// сама роль
		res := tx.Session(&gorm.Session{NewDB: true}).
			Model(&models.Role{}).
			Where("id = ? AND deleted = FALSE", roleID).
			Updates(update)
		if res.Error != nil {
			return res.Error
		}
		affected = res.RowsAffected
		if affected == 0 {
			return errors.New("role not found")
		}
		// помечаем связи user_roles как удалённые (если используешь soft delete там)
		if res := tx.Session(&gorm.Session{NewDB: true}).
			Model(&models.UserRole{}).
			Where("role_id = ?", roleID).
			Updates(update); res.Error != nil {
			return res.Error
		}
		return nil
	})

	if err != nil {
		return false, err
	}

	return affected > 0, nil
}
