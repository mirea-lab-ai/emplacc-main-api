package postgres

import (
	models "emplacc-api/internal/domain"
	"emplacc-api/internal/ports"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type subscriptionRepository struct {
	db *gorm.DB
}

func NewSubscriptionRepository(db *gorm.DB) ports.SubscriptionRepository {
	return &subscriptionRepository{
		db: db,
	}
}

func (r *subscriptionRepository) GetAllSubscriptions(limit, offset int) ([]models.Subscription, int64, error) {
	var totalCount int64
	if err := r.db.Session(&gorm.Session{NewDB: true}).
		Model(&models.Subscription{}).
		Where("deleted = FALSE").
		Count(&totalCount).Error; err != nil {
		return nil, 0, err
	}

	var subs []models.Subscription
	if err := r.db.Session(&gorm.Session{NewDB: true}).
		Model(&models.Subscription{}).
		Where("deleted = FALSE").
		Limit(limit).Offset(offset).
		Find(&subs).Error; err != nil {
		return nil, 0, err
	}

	return subs, totalCount, nil
}

func (r *subscriptionRepository) GetSubscriptionsByUserId(userUUID uuid.UUID, limit, offset int) ([]models.Subscription, int64, error) {
	var totalCount int64
	if err := r.db.Session(&gorm.Session{NewDB: true}).
		Model(&models.Subscription{}).
		Where("deleted = FALSE AND user_id = ?", userUUID).
		Count(&totalCount).Error; err != nil {
		return nil, 0, err
	}

	var subs []models.Subscription
	if err := r.db.Session(&gorm.Session{NewDB: true}).
		Model(&models.Subscription{}).
		Where("deleted = FALSE AND user_id = ?", userUUID).
		Limit(limit).Offset(offset).
		Find(&subs).Error; err != nil {
		return nil, 0, err
	}

	return subs, totalCount, nil
}

func (r *subscriptionRepository) GetSubscriptionBySubObject(subObjUUID uuid.UUID, typeId int8, limit, offset int) ([]models.Subscription, int64, error) {
	var totalCount int64
	if err := r.db.Session(&gorm.Session{NewDB: true}).
		Model(&models.Subscription{}).
		Where("deleted = FALSE AND subscription_id = ? AND type_id = ?", subObjUUID, typeId).
		Count(&totalCount).Error; err != nil {
		return nil, 0, err
	}

	var subs []models.Subscription
	if err := r.db.Session(&gorm.Session{NewDB: true}).
		Model(&models.Subscription{}).
		Where("deleted = FALSE AND subscription_id = ? AND type_id = ?", subObjUUID, typeId).
		Limit(limit).Offset(offset).
		Find(&subs).Error; err != nil {
		return nil, 0, err
	}

	return subs, totalCount, nil
}

func (r *subscriptionRepository) GetSubscriptionById(subId uuid.UUID) (*models.Subscription, error) {
	var subscription models.Subscription
	result := r.db.Session(&gorm.Session{NewDB: true}).
		Model(&models.Subscription{}).
		Where("deleted = FALSE AND id = ?", subId).
		First(&subscription)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, errors.New("subscription not found")
		}
		return nil, result.Error
	}

	return &subscription, nil
}

func (r *subscriptionRepository) CreateSubscription(sub models.Subscription) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		return tx.Session(&gorm.Session{NewDB: true}).
			Model(&models.Subscription{}).
			Create(&sub).Error
	})
}

func (r *subscriptionRepository) DeleteSubscription(subUUID uuid.UUID) (bool, error) {
	updateData := map[string]interface{}{"deleted": true}

	var affected int64
	err := r.db.Transaction(func(tx *gorm.DB) error {
		res := tx.Session(&gorm.Session{NewDB: true}).
			Model(&models.Subscription{}).
			Where("id = ?", subUUID).
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

func (r *subscriptionRepository) TaskExists(taskID uuid.UUID) (bool, error) {
	var count int64
	err := r.db.Session(&gorm.Session{NewDB: true}).
		Model(&models.Task{}).
		Where("id = ? AND deleted = FALSE", taskID).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *subscriptionRepository) ProblemExists(problemID uuid.UUID) (bool, error) {
	var count int64
	err := r.db.Session(&gorm.Session{NewDB: true}).
		Model(&models.Problem{}).
		Where("id = ? AND deleted = FALSE", problemID).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
