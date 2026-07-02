package ports

import (
	models "emplacc-api/internal/domain"

	"github.com/google/uuid"
)

type SubscriptionRepository interface {
	GetAllSubscriptions(limit, offset int) ([]models.Subscription, int64, error)
	GetSubscriptionsByUserId(userUUID uuid.UUID, limit, offset int) ([]models.Subscription, int64, error)
	GetSubscriptionBySubObject(subObjUUID uuid.UUID, typeId int8, limit, offset int) ([]models.Subscription, int64, error)
	GetSubscriptionById(subId uuid.UUID) (*models.Subscription, error)
	CreateSubscription(sub models.Subscription) error
	DeleteSubscription(subUUID uuid.UUID) (bool, error)
	TaskExists(taskID uuid.UUID) (bool, error)
	ProblemExists(problemID uuid.UUID) (bool, error)
}
