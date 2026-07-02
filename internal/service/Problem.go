package service

import (
	models "emplacc-api/internal/domain"
	"emplacc-api/internal/dto/request"
	"emplacc-api/internal/ports"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

type ProblemService interface {
	GetAllProblems(page, pageSize int) ([]models.Problem, int64, error)
	GetProblemsByUserId(creatorUUID uuid.UUID, page, pageSize int) ([]models.Problem, int64, error)
	GetProblemByID(problemId uuid.UUID) (*models.Problem, error)
	SearchProblems(query string, page, pageSize int) ([]models.Problem, int64, error)
	CreateProblem(req request.ProblemCreateRequest) (uuid.UUID, error)
	UpdateProblem(problemId uuid.UUID, req request.ProblemUpdateRequest) error
	DeleteProblem(problemId uuid.UUID) error
	CreateForumMessage(problemId uuid.UUID, description []string) error
}

type problemService struct {
	repo         ports.ProblemRepository
	forumRepo    ports.ForumMessageRepository
	systemUserId uuid.UUID
}

func NewProblemService(repo ports.ProblemRepository, forumRepo ports.ForumMessageRepository, systemUserId uuid.UUID) ProblemService {
	return &problemService{
		repo:         repo,
		forumRepo:    forumRepo,
		systemUserId: systemUserId,
	}
}

func (s *problemService) GetAllProblems(page, pageSize int) ([]models.Problem, int64, error) {
	offset := (page - 1) * pageSize
	return s.repo.GetAllProblems(pageSize, offset)
}

func (s *problemService) GetProblemsByUserId(creatorUUID uuid.UUID, page, pageSize int) ([]models.Problem, int64, error) {
	offset := (page - 1) * pageSize
	return s.repo.GetProblemsByUserId(creatorUUID, pageSize, offset)
}

func (s *problemService) GetProblemByID(problemId uuid.UUID) (*models.Problem, error) {
	return s.repo.GetProblemByID(problemId)
}

func (s *problemService) SearchProblems(query string, page, pageSize int) ([]models.Problem, int64, error) {
	offset := (page - 1) * pageSize
	return s.repo.SearchProblems(query, pageSize, offset)
}

func (s *problemService) CreateForumMessage(problemId uuid.UUID, description []string) error {
	now := time.Now()
	del := false

	var desc pq.StringArray
	if description != nil {
		desc = pq.StringArray(description)
	} else {
		desc = pq.StringArray{}
	}

	fm := models.ForumMessage{
		ID:          uuid.New(),
		ProblemID:   problemId,
		Description: desc,
		CreatorID:   &s.systemUserId,
		CreatedAt:   &now,
		Deleted:     &del,
	}

	err := s.forumRepo.CreateForumMessage(fm)
	if err != nil {
		return err
	}

	return nil
}

func (s *problemService) CreateProblem(req request.ProblemCreateRequest) (uuid.UUID, error) {
	creatorId, err := uuid.Parse(req.CreatorID)
	if err != nil {
		return uuid.Nil, errors.New("invalid creator id")
	}

	now := time.Now()
	del := false

	var description pq.StringArray
	if req.Description != nil {
		description = pq.StringArray(*req.Description)
	} else {
		description = pq.StringArray{}
	}

	problemId := uuid.New()

	p := models.Problem{
		ID:          problemId,
		Description: description,
		CreatorID:   &creatorId,
		Name:        req.Name,
		CreatedAt:   &now,
		Deleted:     &del,
	}

	err = s.repo.CreateProblem(p)
	if err != nil {
		return uuid.Nil, err
	}

	description = pq.StringArray([]string{"Создана новая проблема"})

	fm := models.ForumMessage{
		ID:          uuid.New(),
		ProblemID:   problemId,
		Description: description,
		CreatorID:   &s.systemUserId,
		CreatedAt:   &now,
		Deleted:     &del,
	}

	err = s.forumRepo.CreateForumMessage(fm)
	if err != nil {
		return p.ID, err
	}

	return p.ID, nil
}

func (s *problemService) UpdateProblem(problemId uuid.UUID, req request.ProblemUpdateRequest) error {
	updateData := make(map[string]interface{})
	if req.Name != nil && *req.Name != "" {
		updateData["name"] = *req.Name
	}
	if req.Description != nil {
		updateData["description"] = pq.StringArray(*req.Description)
	}

	if len(updateData) == 0 {
		return errors.New("no fields to update")
	}

	now := time.Now()
	updateData["updated_at"] = &now

	updated, err := s.repo.UpdateProblem(problemId, updateData)
	if err != nil {
		return err
	}

	if !updated {
		return errors.New("problem not found")
	}

	del := false

	description := pq.StringArray([]string{"Данная проблема обновлена"})

	fm := models.ForumMessage{
		ID:          uuid.New(),
		ProblemID:   problemId,
		Description: description,
		CreatorID:   &s.systemUserId,
		CreatedAt:   &now,
		Deleted:     &del,
	}

	err = s.forumRepo.CreateForumMessage(fm)
	if err != nil {
		return err
	}

	return nil
}

func (s *problemService) DeleteProblem(problemId uuid.UUID) error {
	deleted, err := s.repo.DeleteProblem(problemId)
	if err != nil {
		return err
	}

	if !deleted {
		return errors.New("problem not found")
	}

	return nil
}
