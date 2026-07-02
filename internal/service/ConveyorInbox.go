package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	models "emplacc-api/internal/domain"

	"github.com/google/uuid"
)

type AgentInboxItemResponse struct {
	ID             uuid.UUID  `json:"id"`
	WorkItemID     *uuid.UUID `json:"work_item_id,omitempty"`
	Kind           string     `json:"kind"`
	Source         string     `json:"source"`
	Title          string     `json:"title"`
	Summary        string     `json:"summary"`
	Priority       int16      `json:"priority"`
	ActionRequired bool       `json:"action_required"`
	AckState       string     `json:"ack_state"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type AckAgentInboxItemRequest struct {
	ItemID         uuid.UUID `json:"item_id"`
	State          string    `json:"state"`
	IdempotencyKey string    `json:"idempotency_key"`
}

const inboxAckOperation = "inbox.ack"

type conveyorInboxRepository interface {
	ListAgentInboxItems(ctx context.Context, recipientID uuid.UUID) ([]models.AgentInboxItem, error)
	GetAgentInboxItem(ctx context.Context, id uuid.UUID) (*models.AgentInboxItem, error)
	UpdateAgentInboxItemAck(ctx context.Context, itemID uuid.UUID, recipientID uuid.UUID, state string, at time.Time) error
	GetIdempotencyRecord(ctx context.Context, actorID uuid.UUID, operation string, key string) (*models.IdempotencyRecord, error)
	CreateIdempotencyRecord(ctx context.Context, record *models.IdempotencyRecord) error
}

func (s *conveyorService) ListAgentInbox(ctx context.Context, actor ConveyorActor) ([]AgentInboxItemResponse, error) {
	if actor.ActorID == uuid.Nil {
		return nil, ErrPermissionDenied
	}
	repo, err := s.inboxRepository()
	if err != nil {
		return nil, err
	}
	items, err := repo.ListAgentInboxItems(ctx, actor.ActorID)
	if err != nil {
		return nil, err
	}
	visible := make([]AgentInboxItemResponse, 0, len(items))
	for _, item := range items {
		if item.WorkItemID != nil {
			allowed, err := s.repo.ActorCanAccessTask(ctx, actor.ActorID, *item.WorkItemID)
			if err != nil {
				return nil, err
			}
			if !allowed {
				continue
			}
		}
		visible = append(visible, agentInboxItemResponse(item))
	}
	sort.SliceStable(visible, func(left int, right int) bool {
		if visible[left].Priority != visible[right].Priority {
			return visible[left].Priority > visible[right].Priority
		}
		return visible[left].CreatedAt.Before(visible[right].CreatedAt)
	})
	return visible, nil
}

func (s *conveyorService) AckAgentInboxItem(ctx context.Context, actor ConveyorActor, req AckAgentInboxItemRequest) (*AgentInboxItemResponse, error) {
	if actor.ActorID == uuid.Nil {
		return nil, ErrPermissionDenied
	}
	if req.ItemID == uuid.Nil || !validAgentInboxAckState(req.State) {
		return nil, fmt.Errorf("%w: invalid inbox ack", ErrValidation)
	}
	hash, err := requestHash(req)
	if err != nil {
		return nil, err
	}
	var response *AgentInboxItemResponse
	err = s.repo.WithTransaction(ctx, func(tx ConveyorRepository) error {
		repo, ok := tx.(conveyorInboxRepository)
		if !ok {
			return fmt.Errorf("%w: inbox repository is not configured", ErrValidation)
		}
		if req.IdempotencyKey != "" {
			record, err := repo.GetIdempotencyRecord(ctx, actor.ActorID, inboxAckOperation, req.IdempotencyKey)
			if err == nil {
				if record.RequestHash != hash {
					return ErrConflict
				}
				var previous AgentInboxItemResponse
				if err := json.Unmarshal(record.Result, &previous); err != nil {
					return fmt.Errorf("decode inbox ack idempotency result: %w", err)
				}
				response = &previous
				return nil
			}
			if !errors.Is(err, ErrNotFound) {
				return err
			}
		}
		item, err := repo.GetAgentInboxItem(ctx, req.ItemID)
		if err != nil {
			return err
		}
		if item.RecipientID != actor.ActorID {
			return ErrPermissionDenied
		}
		if item.WorkItemID != nil {
			allowed, err := tx.ActorCanAccessTask(ctx, actor.ActorID, *item.WorkItemID)
			if err != nil {
				return err
			}
			if !allowed {
				return ErrPermissionDenied
			}
		}
		now := time.Now()
		if err := repo.UpdateAgentInboxItemAck(ctx, item.ID, actor.ActorID, req.State, now); err != nil {
			return err
		}
		item.AckState = req.State
		item.UpdatedAt = now
		itemResponse := agentInboxItemResponse(*item)
		if req.IdempotencyKey != "" {
			encoded, err := json.Marshal(itemResponse)
			if err != nil {
				return err
			}
			record := &models.IdempotencyRecord{ID: uuid.New(), ActorID: actor.ActorID, Operation: inboxAckOperation, IdempotencyKey: req.IdempotencyKey, RequestHash: hash, Result: encoded, CreatedAt: now}
			if err := repo.CreateIdempotencyRecord(ctx, record); err != nil {
				return err
			}
		}
		response = &itemResponse
		return nil
	})
	if err != nil {
		if req.IdempotencyKey != "" && isUniqueConstraintError(err) {
			return s.replayInboxAckIdempotency(ctx, actor, req.IdempotencyKey, hash)
		}
		return nil, err
	}
	return response, nil
}

func (s *conveyorService) replayInboxAckIdempotency(ctx context.Context, actor ConveyorActor, key string, hash string) (*AgentInboxItemResponse, error) {
	repo, err := s.inboxRepository()
	if err != nil {
		return nil, err
	}
	record, err := repo.GetIdempotencyRecord(ctx, actor.ActorID, inboxAckOperation, key)
	if err != nil {
		return nil, err
	}
	if record.RequestHash != hash {
		return nil, ErrConflict
	}
	var previous AgentInboxItemResponse
	if err := json.Unmarshal(record.Result, &previous); err != nil {
		return nil, fmt.Errorf("decode inbox ack idempotency result: %w", err)
	}
	return &previous, nil
}

func (s *conveyorService) inboxRepository() (conveyorInboxRepository, error) {
	repo, ok := s.repo.(conveyorInboxRepository)
	if !ok {
		return nil, fmt.Errorf("%w: inbox repository is not configured", ErrValidation)
	}
	return repo, nil
}

func validAgentInboxAckState(state string) bool {
	switch state {
	case models.AgentInboxAckStateDelivered, models.AgentInboxAckStateRead, models.AgentInboxAckStateHandled:
		return true
	default:
		return false
	}
}

func agentInboxItemResponse(item models.AgentInboxItem) AgentInboxItemResponse {
	return AgentInboxItemResponse{
		ID:             item.ID,
		WorkItemID:     item.WorkItemID,
		Kind:           item.Kind,
		Source:         item.Source,
		Title:          item.Title,
		Summary:        item.Summary,
		Priority:       item.Priority,
		ActionRequired: item.ActionRequired,
		AckState:       item.AckState,
		CreatedAt:      item.CreatedAt,
		UpdatedAt:      item.UpdatedAt,
	}
}
