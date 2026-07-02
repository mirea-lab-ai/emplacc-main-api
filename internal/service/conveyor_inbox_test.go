package service

import (
	"context"
	"testing"
	"time"

	models "emplacc-api/internal/domain"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestConveyorServiceListsAccessibleInboxItemsForCurrentActor(t *testing.T) {
	repo := newFakeConveyorRepository()
	actor := testActor()
	accessibleWorkItemID := uuid.New()
	inaccessibleWorkItemID := uuid.New()
	otherActorID := uuid.New()
	now := time.Date(2026, 6, 22, 10, 0, 0, 0, time.UTC)
	repo.setTaskAccess(accessibleWorkItemID, actor.ActorID, true)
	repo.setTaskAccess(inaccessibleWorkItemID, actor.ActorID, false)
	accessibleItemID := uuid.New()
	repo.inboxItems[accessibleItemID] = &models.AgentInboxItem{
		ID:             accessibleItemID,
		RecipientID:    actor.ActorID,
		WorkItemID:     &accessibleWorkItemID,
		Kind:           "approval_requested",
		Source:         "approval_request",
		Title:          "Approval required",
		Summary:        "Review the pending approval",
		Priority:       5,
		ActionRequired: true,
		AckState:       models.AgentInboxAckStateDelivered,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	repo.inboxItems[uuid.New()] = &models.AgentInboxItem{ID: uuid.New(), RecipientID: actor.ActorID, WorkItemID: &inaccessibleWorkItemID, Kind: "hidden", Source: "test", Title: "Hidden", AckState: models.AgentInboxAckStateDelivered, CreatedAt: now, UpdatedAt: now}
	repo.inboxItems[uuid.New()] = &models.AgentInboxItem{ID: uuid.New(), RecipientID: otherActorID, WorkItemID: &accessibleWorkItemID, Kind: "other", Source: "test", Title: "Other", AckState: models.AgentInboxAckStateDelivered, CreatedAt: now, UpdatedAt: now}
	svc := NewConveyorService(repo)

	items, err := svc.(*conveyorService).ListAgentInbox(context.Background(), actor)

	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, accessibleItemID, items[0].ID)
	require.Equal(t, accessibleWorkItemID, *items[0].WorkItemID)
	require.Equal(t, "approval_requested", items[0].Kind)
	require.Equal(t, "approval_request", items[0].Source)
	require.Equal(t, "Approval required", items[0].Title)
	require.Equal(t, "Review the pending approval", items[0].Summary)
	require.Equal(t, int16(5), items[0].Priority)
	require.True(t, items[0].ActionRequired)
	require.Equal(t, models.AgentInboxAckStateDelivered, items[0].AckState)
}

func TestConveyorServiceAckAgentInboxItemValidatesStateAndOwnership(t *testing.T) {
	repo := newFakeConveyorRepository()
	actor := testActor()
	workItemID := uuid.New()
	itemID := uuid.New()
	now := time.Date(2026, 6, 22, 10, 0, 0, 0, time.UTC)
	repo.setTaskAccess(workItemID, actor.ActorID, true)
	repo.inboxItems[itemID] = &models.AgentInboxItem{ID: itemID, RecipientID: actor.ActorID, WorkItemID: &workItemID, Kind: "question", Source: "forum", Title: "Question", AckState: models.AgentInboxAckStateDelivered, CreatedAt: now, UpdatedAt: now}
	svc := NewConveyorService(repo)

	_, err := svc.(*conveyorService).AckAgentInboxItem(context.Background(), actor, AckAgentInboxItemRequest{ItemID: uuid.Nil, State: models.AgentInboxAckStateRead})
	require.ErrorIs(t, err, ErrValidation)
	_, err = svc.(*conveyorService).AckAgentInboxItem(context.Background(), actor, AckAgentInboxItemRequest{ItemID: itemID, State: "ignored"})
	require.ErrorIs(t, err, ErrValidation)
	_, err = svc.(*conveyorService).AckAgentInboxItem(context.Background(), actor, AckAgentInboxItemRequest{ItemID: itemID, State: " read "})
	require.ErrorIs(t, err, ErrValidation)

	readItem, err := svc.(*conveyorService).AckAgentInboxItem(context.Background(), actor, AckAgentInboxItemRequest{ItemID: itemID, State: models.AgentInboxAckStateRead})
	require.NoError(t, err)
	require.Equal(t, models.AgentInboxAckStateRead, readItem.AckState)
	handledItem, err := svc.(*conveyorService).AckAgentInboxItem(context.Background(), actor, AckAgentInboxItemRequest{ItemID: itemID, State: models.AgentInboxAckStateHandled})
	require.NoError(t, err)
	require.Equal(t, models.AgentInboxAckStateHandled, handledItem.AckState)
	retryItem, err := svc.(*conveyorService).AckAgentInboxItem(context.Background(), actor, AckAgentInboxItemRequest{ItemID: itemID, State: models.AgentInboxAckStateHandled})
	require.NoError(t, err)
	require.Equal(t, models.AgentInboxAckStateHandled, retryItem.AckState)
	idempotentItem, err := svc.(*conveyorService).AckAgentInboxItem(context.Background(), actor, AckAgentInboxItemRequest{ItemID: itemID, State: models.AgentInboxAckStateRead, IdempotencyKey: "inbox-ack-1"})
	require.NoError(t, err)
	replayedItem, err := svc.(*conveyorService).AckAgentInboxItem(context.Background(), actor, AckAgentInboxItemRequest{ItemID: itemID, State: models.AgentInboxAckStateRead, IdempotencyKey: "inbox-ack-1"})
	require.NoError(t, err)
	require.True(t, idempotentItem.UpdatedAt.Equal(replayedItem.UpdatedAt))
	_, err = svc.(*conveyorService).AckAgentInboxItem(context.Background(), actor, AckAgentInboxItemRequest{ItemID: itemID, State: models.AgentInboxAckStateHandled, IdempotencyKey: "inbox-ack-1"})
	require.ErrorIs(t, err, ErrConflict)
}

func TestConveyorServiceAckAgentInboxItemRejectsUnauthorizedRecipientAndWorkItem(t *testing.T) {
	repo := newFakeConveyorRepository()
	actor := testActor()
	otherActorID := uuid.New()
	workItemID := uuid.New()
	inaccessibleWorkItemID := uuid.New()
	now := time.Date(2026, 6, 22, 10, 0, 0, 0, time.UTC)
	repo.setTaskAccess(workItemID, actor.ActorID, true)
	repo.setTaskAccess(inaccessibleWorkItemID, actor.ActorID, false)
	otherRecipientItemID := uuid.New()
	inaccessibleItemID := uuid.New()
	repo.inboxItems[otherRecipientItemID] = &models.AgentInboxItem{ID: otherRecipientItemID, RecipientID: otherActorID, WorkItemID: &workItemID, Kind: "other", Source: "test", Title: "Other", AckState: models.AgentInboxAckStateDelivered, CreatedAt: now, UpdatedAt: now}
	repo.inboxItems[inaccessibleItemID] = &models.AgentInboxItem{ID: inaccessibleItemID, RecipientID: actor.ActorID, WorkItemID: &inaccessibleWorkItemID, Kind: "hidden", Source: "test", Title: "Hidden", AckState: models.AgentInboxAckStateDelivered, CreatedAt: now, UpdatedAt: now}
	svc := NewConveyorService(repo)

	_, err := svc.(*conveyorService).AckAgentInboxItem(context.Background(), actor, AckAgentInboxItemRequest{ItemID: otherRecipientItemID, State: models.AgentInboxAckStateRead})
	require.ErrorIs(t, err, ErrPermissionDenied)
	_, err = svc.(*conveyorService).AckAgentInboxItem(context.Background(), actor, AckAgentInboxItemRequest{ItemID: inaccessibleItemID, State: models.AgentInboxAckStateRead})
	require.ErrorIs(t, err, ErrPermissionDenied)
}
