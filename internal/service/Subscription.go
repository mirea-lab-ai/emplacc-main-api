package service

import (
	models "emplacc-api/internal/domain"
	"emplacc-api/internal/dto/request"
	"emplacc-api/internal/ports"
	"errors"
	"time"

	"github.com/google/uuid"
)

type SubscriptionService interface {
	GetAllSubscriptions(page, pageSize int) ([]models.Subscription, int64, error)
	GetSubscriptionsByUserId(userUUID uuid.UUID, page, pageSize int) ([]models.Subscription, int64, error)
	GetSubscriptionBySubObject(subObjUUID uuid.UUID, typeId int8, page, pageSize int) ([]models.Subscription, int64, error)
	GetSubscriptionById(subId uuid.UUID) (*models.Subscription, error)
	CreateSubscription(req request.SubscriptionCreateRequest) (uuid.UUID, error)
	DeleteSubscription(subUUID uuid.UUID) error
}

type subscriptionService struct {
	repo ports.SubscriptionRepository
}

func NewSubscriptionService(repo ports.SubscriptionRepository) SubscriptionService {
	return &subscriptionService{
		repo: repo,
	}
}

func (s *subscriptionService) GetAllSubscriptions(page, pageSize int) ([]models.Subscription, int64, error) {
	offset := (page - 1) * pageSize
	return s.repo.GetAllSubscriptions(pageSize, offset)
}

func (s *subscriptionService) GetSubscriptionsByUserId(userUUID uuid.UUID, page, pageSize int) ([]models.Subscription, int64, error) {
	offset := (page - 1) * pageSize
	return s.repo.GetSubscriptionsByUserId(userUUID, pageSize, offset)
}

func (s *subscriptionService) GetSubscriptionBySubObject(subObjUUID uuid.UUID, typeId int8, page, pageSize int) ([]models.Subscription, int64, error) {
	offset := (page - 1) * pageSize
	return s.repo.GetSubscriptionBySubObject(subObjUUID, typeId, pageSize, offset)
}

func (s *subscriptionService) GetSubscriptionById(subId uuid.UUID) (*models.Subscription, error) {
	return s.repo.GetSubscriptionById(subId)
}

func (s *subscriptionService) CreateSubscription(req request.SubscriptionCreateRequest) (uuid.UUID, error) {
	newUUID := uuid.New()
	now := time.Now()
	del := false

	userId, err := uuid.Parse(req.UserId)
	if err != nil {
		return uuid.Nil, errors.New("invalid user id")
	}
	subId, err := uuid.Parse(req.SubscriptionId)
	if err != nil {
		return uuid.Nil, errors.New("invalid subscription id")
	}
	typeId := *req.TypeId

	switch typeId {
	case 0:
		exists, err := s.repo.TaskExists(subId)
		if err != nil {
			return uuid.Nil, err
		}
		if !exists {
			return uuid.Nil, errors.New("task not found")
		}
	case 1:
		exists, err := s.repo.ProblemExists(subId)
		if err != nil {
			return uuid.Nil, err
		}
		if !exists {
			return uuid.Nil, errors.New("problem not found")
		}
	default:
		return uuid.Nil, errors.New("invalid type object")
	}

	sub := models.Subscription{
		ID:             newUUID,
		UserID:         userId,
		SubscriptionId: &subId,
		TypeID:         req.TypeId,
		CreatedAt:      &now,
		Deleted:        &del,
	}

	err = s.repo.CreateSubscription(sub)
	if err != nil {
		return uuid.Nil, err
	}

	return sub.ID, nil
}

func (s *subscriptionService) DeleteSubscription(subUUID uuid.UUID) error {
	deleted, err := s.repo.DeleteSubscription(subUUID)
	if err != nil {
		return err
	}

	if !deleted {
		return errors.New("subscription not found")
	}

	return nil
}
