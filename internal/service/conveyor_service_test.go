package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	models "emplacc-api/internal/domain"
	"emplacc-api/internal/ports"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestConveyorServiceCreatesCriterionAndUpdatesStateWithEvents(t *testing.T) {
	repo := newFakeConveyorRepository()
	taskID := repo.addTask(true)
	actor := testActor()
	svc := NewConveyorService(repo)

	created, err := svc.CreateAcceptanceCriterion(context.Background(), actor, CreateAcceptanceCriterionRequest{
		TaskID:         taskID,
		Title:          "Implement backend core",
		Required:       true,
		IdempotencyKey: "criterion-create-1",
	})
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, created.EntityID)
	require.NotEqual(t, uuid.Nil, created.EventID)
	require.Len(t, repo.events, 1)
	require.Equal(t, "criterion.created", repo.events[0].Type)

	updated, err := svc.UpdateAcceptanceCriterionState(context.Background(), actor, taskID, created.EntityID, UpdateCriterionStateRequest{
		State:          models.AcceptanceCriterionStatePassed,
		IdempotencyKey: "criterion-update-1",
	})
	require.NoError(t, err)
	require.Equal(t, created.EntityID, updated.EntityID)
	require.NotEqual(t, uuid.Nil, updated.EventID)
	require.Len(t, repo.events, 2)
	require.Equal(t, "criterion.state_changed", repo.events[1].Type)
	require.JSONEq(t, `{"previous_state":"unchecked","new_state":"passed"}`, string(repo.events[1].Payload))
}

func TestConveyorServiceAppendsAndRevokesEvidence(t *testing.T) {
	repo := newFakeConveyorRepository()
	taskID := repo.addTask(true)
	actor := testActor()
	svc := NewConveyorService(repo)

	attached, err := svc.AttachEvidence(context.Background(), actor, AttachEvidenceRequest{
		TaskID:         taskID,
		Type:           models.EvidenceTypeLink,
		Verdict:        models.EvidenceVerdictSupports,
		URI:            "https://example.test/evidence",
		IdempotencyKey: "evidence-attach-1",
	})
	require.NoError(t, err)
	require.Len(t, repo.evidence, 1)
	require.False(t, repo.evidence[attached.EntityID].Revoked)

	revoked, err := svc.RevokeEvidence(context.Background(), actor, taskID, attached.EntityID, RevokeEvidenceRequest{Reason: "superseded"})
	require.NoError(t, err)
	require.Equal(t, attached.EntityID, revoked.EntityID)
	require.Len(t, repo.evidence, 1, "revocation must not delete evidence")
	require.True(t, repo.evidence[attached.EntityID].Revoked)
	require.Equal(t, "evidence.revoked", repo.events[len(repo.events)-1].Type)
}

func TestConveyorServiceAuthorizeWorkItemAccessUsesProjectScope(t *testing.T) {
	repo := newFakeConveyorRepository()
	taskID := repo.addTask(true)
	actor := testActor()
	repo.setTaskAccess(taskID, actor.ActorID, false)
	svc := NewConveyorService(repo)

	err := svc.AuthorizeWorkItemAccess(context.Background(), actor, taskID)
	require.ErrorIs(t, err, ErrPermissionDenied)

	repo.setTaskAccess(taskID, actor.ActorID, true)
	err = svc.AuthorizeWorkItemAccess(context.Background(), actor, taskID)
	require.NoError(t, err)
}

func TestConveyorServiceRejectsSecretLikeEvidenceBeforeStorage(t *testing.T) {
	repo := newFakeConveyorRepository()
	taskID := repo.addTask(true)
	svc := NewConveyorService(repo)

	_, err := svc.AttachEvidence(context.Background(), testActor(), AttachEvidenceRequest{
		TaskID:   taskID,
		Type:     models.EvidenceTypeLink,
		Verdict:  models.EvidenceVerdictSupports,
		URI:      "https://example.test/evidence?token=emplacc_fake_secret_token",
		Title:    "Bearer emplacc_fake_title_token",
		Metadata: json.RawMessage(`{"api_token":"emplacc_fake_metadata_token"}`),
	})

	require.ErrorIs(t, err, ErrValidation)
	require.Empty(t, repo.evidence)
	require.Empty(t, repo.events)
}

func TestConveyorServiceDeploymentLogEvidenceRequiresApprovalToken(t *testing.T) {
	repo := newFakeConveyorRepository()
	taskID := repo.addTask(true)
	svc := NewConveyorService(repo)

	_, err := svc.AttachEvidence(context.Background(), testActor(), AttachEvidenceRequest{
		TaskID:          taskID,
		Type:            models.EvidenceTypeDeploymentLog,
		Verdict:         models.EvidenceVerdictSupports,
		URI:             "https://deploy.example.test/log/1",
		ApprovalGranted: true,
	})

	require.ErrorIs(t, err, ErrApprovalRequired)
	require.Empty(t, repo.evidence)
	require.Empty(t, repo.events)

	result, err := svc.AttachEvidence(context.Background(), testActor(), AttachEvidenceRequest{
		TaskID:          taskID,
		Type:            models.EvidenceTypeDeploymentLog,
		Verdict:         models.EvidenceVerdictSupports,
		URI:             "https://deploy.example.test/log/1",
		ApprovalGranted: true,
		ApprovalToken:   "approval-1",
	})

	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, result.EntityID)
	require.Len(t, repo.evidence, 1)
}

func TestConveyorServiceRejectsCriterionPathMismatch(t *testing.T) {
	repo := newFakeConveyorRepository()
	actualTaskID := repo.addTask(true)
	pathTaskID := repo.addTask(true)
	actor := testActor()
	svc := NewConveyorService(repo)

	criterionID := repo.addCriterion(actualTaskID, true, models.AcceptanceCriterionStateUnchecked)

	_, err := svc.UpdateAcceptanceCriterionState(context.Background(), actor, pathTaskID, criterionID, UpdateCriterionStateRequest{
		State: models.AcceptanceCriterionStatePassed,
	})

	require.ErrorIs(t, err, ErrNotFound)
	require.Equal(t, models.AcceptanceCriterionStateUnchecked, repo.criteria[criterionID].State)
	require.Empty(t, repo.events)
}

func TestConveyorServiceRejectsEvidencePathMismatch(t *testing.T) {
	repo := newFakeConveyorRepository()
	actualTaskID := repo.addTask(true)
	pathTaskID := repo.addTask(true)
	actor := testActor()
	svc := NewConveyorService(repo)

	evidenceID := repo.addEvidence(actualTaskID, models.EvidenceVerdictSupports, false)

	_, err := svc.RevokeEvidence(context.Background(), actor, pathTaskID, evidenceID, RevokeEvidenceRequest{Reason: "wrong work item"})

	require.ErrorIs(t, err, ErrNotFound)
	require.False(t, repo.evidence[evidenceID].Revoked)
	require.Empty(t, repo.events)
}

func TestConveyorServiceCloseGate(t *testing.T) {
	actor := testActor()

	t.Run("denies close without supporting evidence", func(t *testing.T) {
		repo := newFakeConveyorRepository()
		taskID := repo.addTask(true)
		closedStatusID := repo.addStatus(false)
		svc := NewConveyorService(repo)

		_, err := svc.CloseTask(context.Background(), actor, taskID, CloseTaskRequest{ToStatusID: closedStatusID})
		require.ErrorIs(t, err, ErrValidation)
		require.Contains(t, err.Error(), "supporting evidence")
		require.Empty(t, repo.events)
	})

	t.Run("denies close with active contradicting evidence", func(t *testing.T) {
		repo := newFakeConveyorRepository()
		taskID := repo.addTask(true)
		closedStatusID := repo.addStatus(false)
		repo.addEvidence(taskID, models.EvidenceVerdictSupports, false)
		repo.addEvidence(taskID, models.EvidenceVerdictContradicts, false)
		svc := NewConveyorService(repo)

		_, err := svc.CloseTask(context.Background(), actor, taskID, CloseTaskRequest{ToStatusID: closedStatusID})
		require.ErrorIs(t, err, ErrValidation)
		require.Contains(t, err.Error(), "contradicting evidence")
		require.Empty(t, repo.events)
	})

	t.Run("denies close with failed required criterion", func(t *testing.T) {
		repo := newFakeConveyorRepository()
		taskID := repo.addTask(true)
		closedStatusID := repo.addStatus(false)
		repo.addCriterion(taskID, true, models.AcceptanceCriterionStateFailed)
		repo.addEvidence(taskID, models.EvidenceVerdictSupports, false)
		svc := NewConveyorService(repo)

		_, err := svc.CloseTask(context.Background(), actor, taskID, CloseTaskRequest{ToStatusID: closedStatusID})
		require.ErrorIs(t, err, ErrValidation)
		require.Contains(t, err.Error(), "required criterion")
		require.Empty(t, repo.events)
	})

	t.Run("closes with passed criteria and supporting evidence", func(t *testing.T) {
		repo := newFakeConveyorRepository()
		taskID := repo.addTask(true)
		closedStatusID := repo.addStatus(false)
		repo.addCriterion(taskID, true, models.AcceptanceCriterionStatePassed)
		repo.addEvidence(taskID, models.EvidenceVerdictSupports, false)
		svc := NewConveyorService(repo)

		result, err := svc.CloseTask(context.Background(), actor, taskID, CloseTaskRequest{ToStatusID: closedStatusID})
		require.NoError(t, err)
		require.Equal(t, taskID, result.EntityID)
		require.Equal(t, closedStatusID, repo.tasks[taskID].StatusID)
		require.Equal(t, "task.completed", repo.events[len(repo.events)-1].Type)
	})

	t.Run("requires approval for medium risk waiver", func(t *testing.T) {
		repo := newFakeConveyorRepository()
		taskID := repo.addTask(true)
		closedStatusID := repo.addStatus(false)
		svc := NewConveyorService(repo)

		_, err := svc.CloseTask(context.Background(), actor, taskID, CloseTaskRequest{
			ToStatusID:   closedStatusID,
			TaskWaiverID: ptrUUID(uuid.New()),
			RiskLevel:    "medium",
		})
		require.ErrorIs(t, err, ErrApprovalRequired)
		require.Empty(t, repo.events)
	})

	t.Run("bare approval boolean is not enough for risky waiver", func(t *testing.T) {
		repo := newFakeConveyorRepository()
		taskID := repo.addTask(true)
		closedStatusID := repo.addStatus(false)
		svc := NewConveyorService(repo)

		_, err := svc.CloseTask(context.Background(), actor, taskID, CloseTaskRequest{
			ToStatusID:      closedStatusID,
			TaskWaiverID:    ptrUUID(uuid.New()),
			RiskLevel:       "medium",
			ApprovalGranted: true,
		})
		require.ErrorIs(t, err, ErrApprovalRequired)
		require.Empty(t, repo.events)
	})

	t.Run("approval token allows risky waiver", func(t *testing.T) {
		repo := newFakeConveyorRepository()
		taskID := repo.addTask(true)
		closedStatusID := repo.addStatus(false)
		waiverID := repo.seedWaiver(taskID, models.WaiverScopeTask)
		svc := NewConveyorService(repo)

		result, err := svc.CloseTask(context.Background(), actor, taskID, CloseTaskRequest{
			ToStatusID:      closedStatusID,
			TaskWaiverID:    ptrUUID(waiverID),
			RiskLevel:       "medium",
			ApprovalGranted: true,
			ApprovalToken:   "approval-1",
		})
		require.NoError(t, err)
		require.Equal(t, taskID, result.EntityID)
	})
}

func TestConveyorServiceAuditableApprovalAndWaiverCloseFlow(t *testing.T) {
	repo := newFakeConveyorRepository()
	actor := testActor()
	taskID := repo.addTask(true)
	closedStatusID := repo.addStatus(false)
	repo.addEvidence(taskID, models.EvidenceVerdictContradicts, false)
	svc := NewConveyorService(repo)
	ctx := context.Background()

	approval, err := svc.RequestApproval(ctx, actor, RequestApprovalRequest{WorkItemID: ptrUUID(taskID), Action: models.ApprovalActionCloseWithWaiver, RiskLevel: "high", Reason: "close despite contradicting evidence"})
	require.NoError(t, err)

	if _, err := svc.GrantApproval(ctx, actor, approval.EntityID, DecideApprovalRequest{Reason: "owner approved"}); err != nil {
		t.Fatalf("grant approval: %v", err)
	}

	waiver, err := svc.CreateWaiver(ctx, actor, CreateWaiverRequest{Scope: models.WaiverScopeTask, WorkItemID: taskID, Reason: "tracked in follow-up", ApprovalRequestID: ptrUUID(approval.EntityID)})
	require.NoError(t, err)

	// The granted ApprovalRequest (referenced by ID as the token) authorizes the
	// risky close without any inline granted=true flag.
	result, err := svc.CloseTask(ctx, actor, taskID, CloseTaskRequest{
		ToStatusID:    closedStatusID,
		TaskWaiverID:  ptrUUID(waiver.EntityID),
		RiskLevel:     "high",
		ApprovalToken: approval.EntityID.String(),
	})
	require.NoError(t, err)
	require.Equal(t, taskID, result.EntityID)
	require.Equal(t, closedStatusID, repo.tasks[taskID].StatusID)

	types := map[string]bool{}
	for _, ev := range repo.events {
		types[ev.Type] = true
	}
	for _, want := range []string{"approval.requested", "approval.granted", "waiver.created", "task.completed"} {
		require.Truef(t, types[want], "expected event %q in audit log", want)
	}

	stored, err := svc.GetApprovalRequest(ctx, approval.EntityID)
	require.NoError(t, err)
	require.Equal(t, models.ApprovalStatusGranted, stored.Status)
	require.NotNil(t, stored.DecidedBy)
}

func TestConveyorServiceIdempotencyReplayAndConflict(t *testing.T) {
	repo := newFakeConveyorRepository()
	taskID := repo.addTask(true)
	actor := testActor()
	svc := NewConveyorService(repo)

	first, err := svc.AttachEvidence(context.Background(), actor, AttachEvidenceRequest{
		TaskID:         taskID,
		Type:           models.EvidenceTypeLink,
		Verdict:        models.EvidenceVerdictSupports,
		URI:            "https://example.test/one",
		IdempotencyKey: "same-key",
	})
	require.NoError(t, err)

	replay, err := svc.AttachEvidence(context.Background(), actor, AttachEvidenceRequest{
		TaskID:         taskID,
		Type:           models.EvidenceTypeLink,
		Verdict:        models.EvidenceVerdictSupports,
		URI:            "https://example.test/one",
		IdempotencyKey: "same-key",
	})
	require.NoError(t, err)
	require.True(t, replay.Replayed)
	require.Equal(t, first.EntityID, replay.EntityID)
	require.Len(t, repo.evidence, 1)
	require.Len(t, repo.events, 1)

	_, err = svc.AttachEvidence(context.Background(), actor, AttachEvidenceRequest{
		TaskID:         taskID,
		Type:           models.EvidenceTypeLink,
		Verdict:        models.EvidenceVerdictSupports,
		URI:            "https://example.test/two",
		IdempotencyKey: "same-key",
	})
	require.ErrorIs(t, err, ErrConflict)
	require.Len(t, repo.evidence, 1)
	require.Len(t, repo.events, 1)
}

func TestConveyorServiceRejectsSelfAndDuplicateLinks(t *testing.T) {
	repo := newFakeConveyorRepository()
	sourceID := repo.addTask(true)
	targetID := repo.addTask(true)
	actor := testActor()
	svc := NewConveyorService(repo)

	_, err := svc.LinkTasks(context.Background(), actor, LinkTasksRequest{SourceTaskID: sourceID, TargetTaskID: sourceID, LinkType: models.TaskLinkTypeRelatesTo})
	require.ErrorIs(t, err, ErrValidation)

	created, err := svc.LinkTasks(context.Background(), actor, LinkTasksRequest{SourceTaskID: sourceID, TargetTaskID: targetID, LinkType: models.TaskLinkTypeRelatesTo})
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, created.EntityID)

	_, err = svc.LinkTasks(context.Background(), actor, LinkTasksRequest{SourceTaskID: sourceID, TargetTaskID: targetID, LinkType: models.TaskLinkTypeRelatesTo})
	require.ErrorIs(t, err, ErrConflict)
	require.Len(t, repo.links, 1)
}

func TestConveyorServiceListsLinksForWorkItemAsSourceOrTarget(t *testing.T) {
	repo := newFakeConveyorRepository()
	workItemID := repo.addTask(true)
	targetID := repo.addTask(true)
	sourceID := repo.addTask(true)
	otherID := repo.addTask(true)
	actor := testActor()
	svc := NewConveyorService(repo)

	createdAsSource, err := svc.LinkTasks(context.Background(), actor, LinkTasksRequest{SourceTaskID: workItemID, TargetTaskID: targetID, LinkType: models.TaskLinkTypeRelatesTo})
	require.NoError(t, err)
	createdAsTarget, err := svc.LinkTasks(context.Background(), actor, LinkTasksRequest{SourceTaskID: sourceID, TargetTaskID: workItemID, LinkType: models.TaskLinkTypeBlocks})
	require.NoError(t, err)
	_, err = svc.LinkTasks(context.Background(), actor, LinkTasksRequest{SourceTaskID: sourceID, TargetTaskID: otherID, LinkType: models.TaskLinkTypeParent})
	require.NoError(t, err)

	links, err := svc.ListTaskLinks(context.Background(), workItemID)

	require.NoError(t, err)
	require.Len(t, links, 2)
	require.ElementsMatch(t, []uuid.UUID{createdAsSource.EntityID, createdAsTarget.EntityID}, []uuid.UUID{links[0].ID, links[1].ID})
}

func TestConveyorServiceRegistersAndListsAgentRuns(t *testing.T) {
	repo := newFakeConveyorRepository()
	workItemID := repo.addTask(true)
	actor := testActor()
	svc := NewConveyorService(repo)

	first, err := svc.RegisterAgentRun(context.Background(), actor, RegisterAgentRunRequest{
		WorkItemID:     workItemID,
		Source:         "rest",
		Harness:        "external-harness",
		Status:         models.AgentRunStatusQueued,
		Summary:        "queued run",
		LogURI:         "https://example.test/logs/one",
		WorkspaceURI:   "file:///workspace/one",
		Metadata:       json.RawMessage(`{"task_id":"task-1","event":"register"}`),
		IdempotencyKey: "agent-run-register-1",
	})
	require.NoError(t, err)

	second, err := svc.RegisterAgentRun(context.Background(), actor, RegisterAgentRunRequest{
		WorkItemID: workItemID,
		Source:     "mcp",
		Harness:    "external-harness",
		Status:     models.AgentRunStatusQueued,
		Summary:    "second queued run",
	})
	require.NoError(t, err)
	require.NotEqual(t, first.EntityID, second.EntityID)

	runs, err := svc.ListAgentRuns(context.Background(), workItemID)
	require.NoError(t, err)
	require.Len(t, runs, 2)
	require.ElementsMatch(t, []uuid.UUID{first.EntityID, second.EntityID}, []uuid.UUID{runs[0].ID, runs[1].ID})
	require.Equal(t, "agent_run.registered", repo.events[0].Type)
	require.JSONEq(t, `{"agent_run_id":"`+first.EntityID.String()+`","harness":"external-harness","new_status":"queued","source":"rest"}`, string(repo.events[0].Payload))
}

func TestConveyorServiceUpdatesAgentRunStatusWithoutClosingWorkItem(t *testing.T) {
	repo := newFakeConveyorRepository()
	workItemID := repo.addTask(true)
	originalStatusID := repo.tasks[workItemID].StatusID
	actor := testActor()
	svc := NewConveyorService(repo)

	created, err := svc.RegisterAgentRun(context.Background(), actor, RegisterAgentRunRequest{
		WorkItemID: workItemID,
		Source:     "rest",
		Harness:    "external-harness",
		Status:     models.AgentRunStatusQueued,
	})
	require.NoError(t, err)

	running, err := svc.UpdateAgentRun(context.Background(), actor, workItemID, created.EntityID, UpdateAgentRunRequest{
		Status: models.AgentRunStatusRunning,
	})
	require.NoError(t, err)
	require.Equal(t, created.EntityID, running.EntityID)

	succeeded, err := svc.UpdateAgentRun(context.Background(), actor, workItemID, created.EntityID, UpdateAgentRunRequest{
		Status:       models.AgentRunStatusSucceeded,
		Summary:      "completed by harness",
		LogURI:       "https://example.test/logs/final",
		WorkspaceURI: "file:///workspace/final",
		Metadata:     json.RawMessage(`{"exit_code":0}`),
	})
	require.NoError(t, err)
	require.Equal(t, created.EntityID, succeeded.EntityID)
	require.Equal(t, originalStatusID, repo.tasks[workItemID].StatusID)
	require.Len(t, repo.events, 3)
	require.Equal(t, "agent_run.status_changed", repo.events[1].Type)
	require.JSONEq(t, `{"agent_run_id":"`+created.EntityID.String()+`","previous_status":"queued","new_status":"running"}`, string(repo.events[1].Payload))
	require.Equal(t, "agent_run.status_changed", repo.events[2].Type)
	require.NotContains(t, collectEventTypes(repo.events), "task.completed")
}

func TestConveyorServiceRejectsInvalidAgentRunTransitions(t *testing.T) {
	repo := newFakeConveyorRepository()
	workItemID := repo.addTask(true)
	actor := testActor()
	svc := NewConveyorService(repo)

	created, err := svc.RegisterAgentRun(context.Background(), actor, RegisterAgentRunRequest{
		WorkItemID: workItemID,
		Source:     "rest",
		Harness:    "external-harness",
		Status:     models.AgentRunStatusQueued,
	})
	require.NoError(t, err)

	_, err = svc.UpdateAgentRun(context.Background(), actor, workItemID, created.EntityID, UpdateAgentRunRequest{Status: models.AgentRunStatusSucceeded})
	require.ErrorIs(t, err, ErrValidation)
	require.Equal(t, models.AgentRunStatusQueued, repo.agentRuns[created.EntityID].Status)
	require.Len(t, repo.events, 1)
}

func TestConveyorServiceHeartbeatsAgentRun(t *testing.T) {
	repo := newFakeConveyorRepository()
	workItemID := repo.addTask(true)
	actor := testActor()
	svc := NewConveyorService(repo)

	created, err := svc.RegisterAgentRun(context.Background(), actor, RegisterAgentRunRequest{
		WorkItemID: workItemID,
		Source:     "rest",
		Harness:    "external-harness",
		Status:     models.AgentRunStatusQueued,
	})
	require.NoError(t, err)
	require.Nil(t, repo.agentRuns[created.EntityID].HeartbeatAt)

	result, err := svc.HeartbeatAgentRun(context.Background(), actor, workItemID, created.EntityID)
	require.NoError(t, err)
	require.Equal(t, created.EntityID, result.EntityID)
	require.NotNil(t, repo.agentRuns[created.EntityID].HeartbeatAt)
	require.Equal(t, "agent_run.heartbeat", repo.events[len(repo.events)-1].Type)
}

func TestConveyorServiceAgentRunIdempotencyReplayAndConflict(t *testing.T) {
	repo := newFakeConveyorRepository()
	workItemID := repo.addTask(true)
	actor := testActor()
	svc := NewConveyorService(repo)

	first, err := svc.RegisterAgentRun(context.Background(), actor, RegisterAgentRunRequest{
		WorkItemID:     workItemID,
		Source:         "rest",
		Harness:        "external-harness",
		Status:         models.AgentRunStatusQueued,
		Summary:        "first",
		IdempotencyKey: "same-run-key",
	})
	require.NoError(t, err)

	replay, err := svc.RegisterAgentRun(context.Background(), actor, RegisterAgentRunRequest{
		WorkItemID:     workItemID,
		Source:         "rest",
		Harness:        "external-harness",
		Status:         models.AgentRunStatusQueued,
		Summary:        "first",
		IdempotencyKey: "same-run-key",
	})
	require.NoError(t, err)
	require.True(t, replay.Replayed)
	require.Equal(t, first.EntityID, replay.EntityID)
	require.Len(t, repo.agentRuns, 1)

	_, err = svc.RegisterAgentRun(context.Background(), actor, RegisterAgentRunRequest{
		WorkItemID:     workItemID,
		Source:         "rest",
		Harness:        "external-harness",
		Status:         models.AgentRunStatusQueued,
		Summary:        "different",
		IdempotencyKey: "same-run-key",
	})
	require.ErrorIs(t, err, ErrConflict)
}

func TestConveyorServiceReturnsNotFoundForUnknownAgentRunInputs(t *testing.T) {
	repo := newFakeConveyorRepository()
	workItemID := repo.addTask(true)
	actor := testActor()
	svc := NewConveyorService(repo)

	_, err := svc.RegisterAgentRun(context.Background(), actor, RegisterAgentRunRequest{
		WorkItemID: uuid.New(),
		Source:     "rest",
		Harness:    "llm-task-report",
		Status:     models.AgentRunStatusQueued,
	})
	require.ErrorIs(t, err, ErrNotFound)

	_, err = svc.GetAgentRun(context.Background(), workItemID, uuid.New())
	require.ErrorIs(t, err, ErrNotFound)

	_, err = svc.ListAgentRuns(context.Background(), uuid.New())
	require.ErrorIs(t, err, ErrNotFound)

	_, err = svc.UpdateAgentRun(context.Background(), actor, workItemID, uuid.New(), UpdateAgentRunRequest{Status: models.AgentRunStatusRunning})
	require.ErrorIs(t, err, ErrNotFound)
}

func TestConveyorServiceMarksStaleAgentRunsFailed(t *testing.T) {
	repo := newFakeConveyorRepository()
	workItemID := repo.addTask(true)
	actor := testActor()
	cutoff := time.Now().Add(-10 * time.Minute)
	staleTime := cutoff.Add(-time.Minute)
	freshTime := cutoff.Add(time.Minute)
	staleQueued := repo.addAgentRun(workItemID, models.AgentRunStatusQueued, staleTime, nil)
	staleRunning := repo.addAgentRun(workItemID, models.AgentRunStatusRunning, staleTime, &staleTime)
	staleNeedsHuman := repo.addAgentRun(workItemID, models.AgentRunStatusNeedsHuman, staleTime, &staleTime)
	freshRunning := repo.addAgentRun(workItemID, models.AgentRunStatusRunning, freshTime, &freshTime)
	terminalSucceeded := repo.addAgentRun(workItemID, models.AgentRunStatusSucceeded, staleTime, &staleTime)
	svc := NewConveyorService(repo)

	marked, err := svc.MarkStaleAgentRunsFailed(context.Background(), actor, cutoff)

	require.NoError(t, err)
	require.ElementsMatch(t, []uuid.UUID{staleQueued, staleRunning, staleNeedsHuman}, marked)
	require.Equal(t, models.AgentRunStatusFailed, repo.agentRuns[staleQueued].Status)
	require.Equal(t, models.AgentRunStatusFailed, repo.agentRuns[staleRunning].Status)
	require.Equal(t, models.AgentRunStatusFailed, repo.agentRuns[staleNeedsHuman].Status)
	require.Equal(t, models.AgentRunStatusRunning, repo.agentRuns[freshRunning].Status)
	require.Equal(t, models.AgentRunStatusSucceeded, repo.agentRuns[terminalSucceeded].Status)
	require.Len(t, repo.events, 3)
	queuedEvent := findAgentRunEvent(t, repo.events, staleQueued)
	require.Equal(t, "agent_run.status_changed", queuedEvent.Type)
	require.JSONEq(t, `{"agent_run_id":"`+staleQueued.String()+`","previous_status":"queued","new_status":"failed","reason":"stale"}`, string(queuedEvent.Payload))
}

func TestConveyorServiceGeneratesProjectReportSnapshot(t *testing.T) {
	repo := newFakeConveyorRepository()
	projectID := uuid.New()
	workItemID := repo.addProjectTask(projectID, "Ship report API", true)
	actor := testActor()
	evidenceID := repo.addEvidenceWithTitle(workItemID, models.EvidenceVerdictSupports, false, "go test evidence")
	completedEventID := repo.addEvent(workItemID, "task.completed", time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC), actor.ActorID, map[string]any{"evidence_id": evidenceID})
	repo.addEvent(workItemID, "criterion.state_changed", time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC), actor.ActorID, map[string]any{"new_status": "passed"})
	svc := NewConveyorService(repo)

	report, err := svc.GenerateProjectReport(context.Background(), actor, GenerateProjectReportRequest{
		ProjectID:   projectID,
		PeriodStart: time.Date(2026, 6, 9, 0, 0, 0, 0, time.UTC),
		PeriodEnd:   time.Date(2026, 6, 16, 0, 0, 0, 0, time.UTC),
	})

	require.NoError(t, err)
	require.Equal(t, projectID, report.ProjectID)
	require.Contains(t, report.SourceEventIDs, completedEventID)
	require.Contains(t, report.SourceEvidenceIDs, evidenceID)
	require.Len(t, report.Facts, 1)
	require.Equal(t, "completed_work", report.Facts[0].Kind)
	require.Equal(t, workItemID, report.Facts[0].WorkItemID)
	require.Contains(t, report.Facts[0].SourceEventIDs, completedEventID)
	require.Contains(t, report.Facts[0].SourceEvidenceIDs, evidenceID)
	require.Empty(t, report.LLMDraft)
	require.Len(t, repo.generatedReports, 1)
	require.Contains(t, collectEventTypes(repo.events), "generated_report.created")
}

func TestConveyorServiceWeeklyNonLLMReportPerformanceBudgetWith500Cards(t *testing.T) {
	repo := newFakeConveyorRepository()
	projectID := uuid.New()
	actor := testActor()
	periodStart := time.Date(2026, 6, 9, 0, 0, 0, 0, time.UTC)
	periodEnd := time.Date(2026, 6, 16, 0, 0, 0, 0, time.UTC)
	for index := 0; index < 500; index++ {
		workItemID := repo.addProjectTask(projectID, fmt.Sprintf("Completed work item %03d", index), false)
		evidenceID := repo.addEvidenceWithTitle(workItemID, models.EvidenceVerdictSupports, false, fmt.Sprintf("Evidence %03d", index))
		repo.addEvent(workItemID, "task.completed", periodStart.Add(time.Duration(index)*time.Minute), actor.ActorID, map[string]any{"evidence_id": evidenceID})
	}
	svc := NewConveyorService(repo)

	startedAt := time.Now()
	report, err := svc.GenerateProjectReport(context.Background(), actor, GenerateProjectReportRequest{
		ProjectID:   projectID,
		PeriodStart: periodStart,
		PeriodEnd:   periodEnd,
		IncludeLLM:  false,
	})
	elapsed := time.Since(startedAt)

	require.NoError(t, err)
	require.Len(t, report.Facts, 500)
	require.Empty(t, report.Risks)
	require.Empty(t, report.LLMDraft)
	require.Less(t, elapsed, 10*time.Second)
	t.Logf("conveyor_weekly_non_llm_report_500_cards_ms=%.2f facts=%d risks=%d", float64(elapsed.Microseconds())/1000, len(report.Facts), len(report.Risks))
}

func TestConveyorServiceDoesNotClaimCompletedWorkWithoutValidEvidence(t *testing.T) {
	actor := testActor()
	periodStart := time.Date(2026, 6, 9, 0, 0, 0, 0, time.UTC)
	periodEnd := time.Date(2026, 6, 16, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name         string
		prepare      func(*fakeConveyorRepository, uuid.UUID)
		expectedRisk string
	}{
		{
			name: "missing evidence",
			prepare: func(repo *fakeConveyorRepository, workItemID uuid.UUID) {
				repo.addEvent(workItemID, "task.completed", periodStart.Add(time.Hour), actor.ActorID, map[string]any{})
			},
			expectedRisk: "supporting evidence is missing",
		},
		{
			name: "revoked evidence",
			prepare: func(repo *fakeConveyorRepository, workItemID uuid.UUID) {
				evidenceID := repo.addEvidenceWithTitle(workItemID, models.EvidenceVerdictSupports, true, "revoked evidence")
				repo.addEvent(workItemID, "task.completed", periodStart.Add(time.Hour), actor.ActorID, map[string]any{"evidence_id": evidenceID})
			},
			expectedRisk: "supporting evidence is missing",
		},
		{
			name: "contradicting evidence",
			prepare: func(repo *fakeConveyorRepository, workItemID uuid.UUID) {
				supportID := repo.addEvidenceWithTitle(workItemID, models.EvidenceVerdictSupports, false, "support")
				repo.addEvidenceWithTitle(workItemID, models.EvidenceVerdictContradicts, false, "contradiction")
				repo.addEvent(workItemID, "task.completed", periodStart.Add(time.Hour), actor.ActorID, map[string]any{"evidence_id": supportID})
			},
			expectedRisk: "contradicting evidence exists",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeConveyorRepository()
			projectID := uuid.New()
			workItemID := repo.addProjectTask(projectID, "Risky work", true)
			tt.prepare(repo, workItemID)
			svc := NewConveyorService(repo)

			report, err := svc.GenerateProjectReport(context.Background(), actor, GenerateProjectReportRequest{ProjectID: projectID, PeriodStart: periodStart, PeriodEnd: periodEnd})

			require.NoError(t, err)
			require.Empty(t, report.Facts)
			require.NotEmpty(t, report.Risks)
			require.Contains(t, report.Risks[0].Text, tt.expectedRisk)
		})
	}
}

func TestConveyorServiceSeparatesReportSectionsAndExportsMarkdownReferences(t *testing.T) {
	repo := newFakeConveyorRepository()
	projectID := uuid.New()
	workItemID := repo.addProjectTask(projectID, "Escape *markdown* [chars]", true)
	actor := testActor()
	evidenceID := repo.addEvidenceWithTitle(workItemID, models.EvidenceVerdictSupports, false, "Evidence with [link] and *star*")
	eventID := repo.addEvent(workItemID, "task.completed", time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC), actor.ActorID, map[string]any{"evidence_id": evidenceID})
	svc := NewConveyorService(repo)

	report, err := svc.GenerateProjectReport(context.Background(), actor, GenerateProjectReportRequest{ProjectID: projectID, PeriodStart: time.Date(2026, 6, 9, 0, 0, 0, 0, time.UTC), PeriodEnd: time.Date(2026, 6, 16, 0, 0, 0, 0, time.UTC)})
	require.NoError(t, err)
	require.NotEmpty(t, report.Facts)
	require.NotEmpty(t, report.Conclusions)
	require.Empty(t, report.Risks)

	markdown, err := svc.ExportGeneratedReportMarkdown(context.Background(), report.ID)

	require.NoError(t, err)
	require.Contains(t, markdown, "## Facts")
	require.Contains(t, markdown, "## Conclusions")
	require.Contains(t, markdown, "## Risks")
	require.Contains(t, markdown, eventID.String())
	require.Contains(t, markdown, evidenceID.String())
	require.Contains(t, markdown, "Escape \\*markdown\\* \\[chars\\]")
}

func TestConveyorServiceLLMSuccessCreatesAgentRunDraftAndDoesNotMutateDomainState(t *testing.T) {
	repo := newFakeConveyorRepository()
	projectID := uuid.New()
	workItemID := repo.addProjectTask(projectID, "LLM report", true)
	actor := testActor()
	evidenceID := repo.addEvidenceWithTitle(workItemID, models.EvidenceVerdictSupports, false, "support")
	repo.addEvent(workItemID, "task.completed", time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC), actor.ActorID, map[string]any{"evidence_id": evidenceID})
	llm := &fakeGeneratedReportLLM{result: "Draft wording from LLM"}
	svc := NewConveyorServiceWithReportLLM(repo, llm)

	report, err := svc.GenerateProjectReport(context.Background(), actor, GenerateProjectReportRequest{ProjectID: projectID, PeriodStart: time.Date(2026, 6, 9, 0, 0, 0, 0, time.UTC), PeriodEnd: time.Date(2026, 6, 16, 0, 0, 0, 0, time.UTC), IncludeLLM: true})

	require.NoError(t, err)
	require.True(t, report.LLMDraft.Auxiliary)
	require.Equal(t, "Draft wording from LLM", report.LLMDraft.Text)
	require.Len(t, repo.agentRuns, 1)
	require.Equal(t, models.AgentRunStatusSucceeded, firstAgentRun(repo).Status)
	require.Len(t, report.Facts, 1)
	require.Equal(t, "completed_work", report.Facts[0].Kind)
	require.Len(t, llm.requests, 1)
	require.Equal(t, projectID, llm.requests[0].ProjectID)
	require.Contains(t, string(repo.generatedReports[report.ID].LLMDraft), "Draft wording from LLM")
	require.Equal(t, 1, len(repo.evidence), "LLM must not mutate evidence")
	require.NotContains(t, collectEventTypes(repo.events), "task.completed.llm")
}

func TestConveyorServiceLLMFailureKeepsBaseReportWithErrorNoteAndFailedAgentRun(t *testing.T) {
	repo := newFakeConveyorRepository()
	projectID := uuid.New()
	workItemID := repo.addProjectTask(projectID, "LLM failure report", true)
	actor := testActor()
	evidenceID := repo.addEvidenceWithTitle(workItemID, models.EvidenceVerdictSupports, false, "support")
	repo.addEvent(workItemID, "task.completed", time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC), actor.ActorID, map[string]any{"evidence_id": evidenceID})
	svc := NewConveyorServiceWithReportLLM(repo, &fakeGeneratedReportLLM{err: errors.New("llm unavailable")})

	report, err := svc.GenerateProjectReport(context.Background(), actor, GenerateProjectReportRequest{ProjectID: projectID, PeriodStart: time.Date(2026, 6, 9, 0, 0, 0, 0, time.UTC), PeriodEnd: time.Date(2026, 6, 16, 0, 0, 0, 0, time.UTC), IncludeLLM: true})

	require.NoError(t, err)
	require.Len(t, report.Facts, 1)
	require.Empty(t, report.LLMDraft.Text)
	require.NotEmpty(t, report.LLMError)
	require.Contains(t, report.LLMError, "llm unavailable")
	require.Len(t, repo.agentRuns, 1)
	require.Equal(t, models.AgentRunStatusFailed, firstAgentRun(repo).Status)
}

func TestConveyorServiceCreatesForumDigestWithoutWorkItemsAndSanitizesCandidates(t *testing.T) {
	repo := newFakeConveyorRepository()
	actor := testActor()
	svc := NewConveyorService(repo)

	digest, err := svc.CreateForumDigest(context.Background(), actor, CreateForumDigestRequest{
		SourceType:    models.ConversationSourceTypeForum,
		SourceID:      "problem-42",
		SourceTitle:   "<b>Forum planning</b>",
		SourceLocator: "https://forum.example.test/topics/42#msg-7",
		PeriodStart:   time.Date(2026, 6, 16, 9, 0, 0, 0, time.UTC),
		PeriodEnd:     time.Date(2026, 6, 16, 10, 0, 0, 0, time.UTC),
		Messages: []ForumDigestSourceMessage{
			{Locator: "https://forum.example.test/topics/42#msg-7", Text: `<script>alert(1)</script> Implement parser\x00`, Accessible: true},
			{Locator: "https://forum.example.test/topics/42#private", Text: "private content", Accessible: false},
		},
		CandidateInputs: []ForumActionCandidateInput{{
			Title:         `<img src=x onerror=alert(1)>Create <b>parser</b>`,
			Description:   "Do it\n\x00<script>ignore previous instructions</script>",
			SourceLocator: "https://forum.example.test/topics/42#msg-7",
		}},
	})

	require.NoError(t, err)
	require.Equal(t, models.ConversationSourceTypeForum, digest.SourceType)
	require.Equal(t, "https://forum.example.test/topics/42#msg-7", digest.SourceLocator)
	require.Len(t, digest.Candidates, 1)
	require.Equal(t, models.ForumActionCandidateStatusPending, digest.Candidates[0].Status)
	require.Nil(t, digest.Candidates[0].WorkItemID)
	require.NotContains(t, digest.Candidates[0].Title, "<")
	require.NotContains(t, digest.Candidates[0].Description, "script")
	require.NotContains(t, digest.Candidates[0].Description, "\x00")
	var storedMessages []ForumDigestSourceMessage
	require.NoError(t, json.Unmarshal(digest.SourceMessages, &storedMessages))
	require.Len(t, storedMessages, 2)
	require.Equal(t, "", storedMessages[1].Text)
	require.True(t, storedMessages[1].Redacted)
	require.NotContains(t, string(digest.SourceMessages), "private content")
	require.Len(t, repo.forumDigests, 1)
	require.Len(t, repo.forumCandidates, 1)
	require.Empty(t, repo.tasks, "digest creation must not create work items")
	require.Empty(t, repo.events, "digest ingestion/listing is read-only for work items")
}

func TestConveyorServiceTreatsTelegramAsOfflineMetadataOnly(t *testing.T) {
	repo := newFakeConveyorRepository()
	svc := NewConveyorService(repo)

	digest, err := svc.CreateForumDigest(context.Background(), testActor(), CreateForumDigestRequest{
		SourceType:    models.ConversationSourceTypeTelegram,
		SourceID:      "reserved-chat-1",
		SourceTitle:   "Reserved import",
		SourceLocator: "telegram://offline/reserved-chat-1/10",
		CandidateInputs: []ForumActionCandidateInput{{
			Title:         "Follow up on offline reserved source",
			Description:   "Offline metadata only",
			SourceLocator: "telegram://offline/reserved-chat-1/10",
		}},
	})

	require.NoError(t, err)
	require.Equal(t, models.ConversationSourceTypeTelegram, digest.SourceType)
	require.Equal(t, "telegram://offline/reserved-chat-1/10", digest.SourceLocator)
	require.Len(t, digest.Candidates, 1)
	require.Empty(t, repo.tasks, "reserved Telegram metadata must not require live Telegram ingest")
}

func TestConveyorServiceListsAndReadsForumDigestsWithoutSideEffects(t *testing.T) {
	repo := newFakeConveyorRepository()
	svc := NewConveyorService(repo)
	digest, err := svc.CreateForumDigest(context.Background(), testActor(), CreateForumDigestRequest{
		SourceType:    models.ConversationSourceTypeForum,
		SourceID:      "topic-list",
		SourceTitle:   "List topic",
		SourceLocator: "https://forum.example.test/topics/list",
		CandidateInputs: []ForumActionCandidateInput{{
			Title:         "Listed candidate",
			Description:   "Visible as candidate only",
			SourceLocator: "https://forum.example.test/topics/list#msg-1",
		}},
	})
	require.NoError(t, err)

	listed, err := svc.ListForumDigests(context.Background(), models.ConversationSourceTypeForum, "topic-list")
	require.NoError(t, err)
	require.Len(t, listed, 1)
	require.Equal(t, digest.ID, listed[0].ID)
	require.Len(t, listed[0].Candidates, 1)
	read, err := svc.GetForumDigest(context.Background(), digest.ID)
	require.NoError(t, err)
	require.Equal(t, digest.Candidates[0].ID, read.Candidates[0].ID)
	require.Nil(t, read.Candidates[0].WorkItemID)
	require.Empty(t, repo.tasks)
	require.Empty(t, repo.events)
}

func TestConveyorServiceRejectsInvalidForumDigestInput(t *testing.T) {
	repo := newFakeConveyorRepository()
	svc := NewConveyorService(repo)
	actor := testActor()

	_, err := svc.CreateForumDigest(context.Background(), actor, CreateForumDigestRequest{SourceType: "email", SourceID: "topic-invalid"})
	require.ErrorIs(t, err, ErrValidation)
	require.Empty(t, repo.forumDigests)
	require.Empty(t, repo.forumCandidates)

	_, err = svc.CreateForumDigest(context.Background(), actor, CreateForumDigestRequest{
		SourceType:  models.ConversationSourceTypeForum,
		SourceID:    "topic-invalid-period",
		PeriodStart: time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC),
		PeriodEnd:   time.Date(2026, 6, 17, 9, 0, 0, 0, time.UTC),
	})
	require.ErrorIs(t, err, ErrValidation)
	require.Empty(t, repo.forumDigests)
	require.Empty(t, repo.forumCandidates)

	_, err = svc.ListForumDigests(context.Background(), "email", "topic-invalid")
	require.ErrorIs(t, err, ErrValidation)
}

func TestConveyorServiceConfirmsForumCandidateOnceAndLinksSourceEvidence(t *testing.T) {
	repo := newFakeConveyorRepository()
	actor := testActor()
	statusID := repo.addStatus(true)
	svc := NewConveyorService(repo)
	digest, err := svc.CreateForumDigest(context.Background(), actor, CreateForumDigestRequest{
		SourceType:    models.ConversationSourceTypeForum,
		SourceID:      "topic-77",
		SourceTitle:   "Planning",
		SourceLocator: "https://forum.example.test/topics/77",
		CandidateInputs: []ForumActionCandidateInput{{
			Title:         "Create digest endpoint",
			Description:   "Add backend endpoint",
			SourceLocator: "https://forum.example.test/topics/77#msg-2",
		}},
	})
	require.NoError(t, err)
	candidateID := digest.Candidates[0].ID

	confirmed, err := svc.ConfirmForumActionCandidate(context.Background(), actor, candidateID, ConfirmForumActionCandidateRequest{StatusID: statusID, IdempotencyKey: "confirm-candidate-1"})
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, confirmed.EntityID)
	require.Len(t, repo.tasks, 1)
	require.Equal(t, confirmed.EntityID, *repo.forumCandidates[candidateID].WorkItemID)
	require.Equal(t, models.ForumActionCandidateStatusConfirmed, repo.forumCandidates[candidateID].Status)
	require.Len(t, repo.evidence, 1)
	for _, evidence := range repo.evidence {
		require.Equal(t, confirmed.EntityID, evidence.TaskID)
		require.Equal(t, models.EvidenceTypeLink, evidence.Type)
		require.Equal(t, "https://forum.example.test/topics/77#msg-2", evidence.URI)
	}
	require.Equal(t, "forum_candidate.confirmed", repo.events[len(repo.events)-1].Type)

	replay, err := svc.ConfirmForumActionCandidate(context.Background(), actor, candidateID, ConfirmForumActionCandidateRequest{StatusID: statusID, IdempotencyKey: "confirm-candidate-1"})
	require.NoError(t, err)
	require.True(t, replay.Replayed)
	require.Equal(t, confirmed.EntityID, replay.EntityID)
	require.Len(t, repo.tasks, 1, "idempotent retry must not create duplicate cards")
}

func TestConveyorServiceRejectsForumCandidateConfirmationWhenStatusMissing(t *testing.T) {
	repo := newFakeConveyorRepository()
	actor := testActor()
	svc := NewConveyorService(repo)
	digest, err := svc.CreateForumDigest(context.Background(), actor, CreateForumDigestRequest{
		SourceType: models.ConversationSourceTypeForum,
		SourceID:   "topic-missing-status",
		CandidateInputs: []ForumActionCandidateInput{{
			Title:         "Needs status",
			Description:   "Confirmation should require an existing status",
			SourceLocator: "https://forum.example.test/topics/missing-status#msg-1",
		}},
	})
	require.NoError(t, err)
	candidateID := digest.Candidates[0].ID

	_, err = svc.ConfirmForumActionCandidate(context.Background(), actor, candidateID, ConfirmForumActionCandidateRequest{StatusID: uuid.New()})

	require.ErrorIs(t, err, ErrNotFound)
	require.Nil(t, repo.forumCandidates[candidateID].WorkItemID)
	require.Equal(t, models.ForumActionCandidateStatusPending, repo.forumCandidates[candidateID].Status)
	require.Empty(t, repo.tasks)
}

func TestConveyorServiceRejectsForumCandidateAndDoesNotConfirmRejectedCandidate(t *testing.T) {
	repo := newFakeConveyorRepository()
	actor := testActor()
	statusID := repo.addStatus(true)
	svc := NewConveyorService(repo)
	digest, err := svc.CreateForumDigest(context.Background(), actor, CreateForumDigestRequest{
		SourceType: models.ConversationSourceTypeForum,
		SourceID:   "topic-reject",
		CandidateInputs: []ForumActionCandidateInput{{
			Title:         "Rejected work",
			Description:   "Should not become a card",
			SourceLocator: "https://forum.example.test/topics/reject#msg-1",
		}},
	})
	require.NoError(t, err)
	candidateID := digest.Candidates[0].ID

	_, err = svc.RejectForumActionCandidate(context.Background(), actor, candidateID, RejectForumActionCandidateRequest{Reason: "out of scope"})
	require.NoError(t, err)
	_, err = svc.ConfirmForumActionCandidate(context.Background(), actor, candidateID, ConfirmForumActionCandidateRequest{StatusID: statusID})

	require.ErrorIs(t, err, ErrConflict)
	require.Equal(t, models.ForumActionCandidateStatusRejected, repo.forumCandidates[candidateID].Status)
	require.Nil(t, repo.forumCandidates[candidateID].WorkItemID)
	require.Empty(t, repo.tasks)
}

type fakeConveyorRepository struct {
	tasks            map[uuid.UUID]*models.Task
	statuses         map[uuid.UUID]*models.Status
	statusBoard      map[uuid.UUID]uuid.UUID
	boardProject     map[uuid.UUID]uuid.UUID
	criteria         map[uuid.UUID]*models.AcceptanceCriterion
	evidence         map[uuid.UUID]*models.Evidence
	events           []models.ConveyorEvent
	links            map[string]models.TaskLink
	workOrders       map[uuid.UUID]*models.WorkOrder
	agentRuns        map[uuid.UUID]*models.AgentRun
	inboxItems       map[uuid.UUID]*models.AgentInboxItem
	generatedReports map[uuid.UUID]*models.GeneratedReport
	forumDigests     map[uuid.UUID]*models.ForumDigest
	forumCandidates  map[uuid.UUID]*models.ForumActionCandidate
	idempotency      map[string]models.IdempotencyRecord
	taskAccess       map[uuid.UUID]map[uuid.UUID]bool
	waivers          map[uuid.UUID]*models.Waiver
	approvals        map[uuid.UUID]*models.ApprovalRequest
}

func newFakeConveyorRepository() *fakeConveyorRepository {
	return &fakeConveyorRepository{
		tasks:            map[uuid.UUID]*models.Task{},
		statuses:         map[uuid.UUID]*models.Status{},
		statusBoard:      map[uuid.UUID]uuid.UUID{},
		boardProject:     map[uuid.UUID]uuid.UUID{},
		criteria:         map[uuid.UUID]*models.AcceptanceCriterion{},
		evidence:         map[uuid.UUID]*models.Evidence{},
		links:            map[string]models.TaskLink{},
		workOrders:       map[uuid.UUID]*models.WorkOrder{},
		agentRuns:        map[uuid.UUID]*models.AgentRun{},
		inboxItems:       map[uuid.UUID]*models.AgentInboxItem{},
		generatedReports: map[uuid.UUID]*models.GeneratedReport{},
		forumDigests:     map[uuid.UUID]*models.ForumDigest{},
		forumCandidates:  map[uuid.UUID]*models.ForumActionCandidate{},
		idempotency:      map[string]models.IdempotencyRecord{},
		taskAccess:       map[uuid.UUID]map[uuid.UUID]bool{},
		waivers:          map[uuid.UUID]*models.Waiver{},
		approvals:        map[uuid.UUID]*models.ApprovalRequest{},
	}
}

func (r *fakeConveyorRepository) seedWaiver(taskID uuid.UUID, scope string) uuid.UUID {
	id := uuid.New()
	r.waivers[id] = &models.Waiver{ID: id, Scope: scope, WorkItemID: taskID, Reason: "seeded", CreatedBy: uuid.New(), CreatedAt: time.Now()}
	return id
}

func (r *fakeConveyorRepository) CreateWaiver(ctx context.Context, waiver *models.Waiver) error {
	r.waivers[waiver.ID] = waiver
	return nil
}

func (r *fakeConveyorRepository) GetWaiver(ctx context.Context, id uuid.UUID) (*models.Waiver, error) {
	if waiver, ok := r.waivers[id]; ok {
		return waiver, nil
	}
	return nil, models.ErrConveyorNotFound
}

func (r *fakeConveyorRepository) CreateApprovalRequest(ctx context.Context, request *models.ApprovalRequest) error {
	r.approvals[request.ID] = request
	return nil
}

func (r *fakeConveyorRepository) GetApprovalRequest(ctx context.Context, id uuid.UUID) (*models.ApprovalRequest, error) {
	if request, ok := r.approvals[id]; ok {
		return request, nil
	}
	return nil, models.ErrConveyorNotFound
}

func (r *fakeConveyorRepository) UpdateApprovalRequest(ctx context.Context, request *models.ApprovalRequest) error {
	if _, ok := r.approvals[request.ID]; !ok {
		return models.ErrConveyorNotFound
	}
	r.approvals[request.ID] = request
	return nil
}

func (r *fakeConveyorRepository) ListPendingApprovals(ctx context.Context, limit int) ([]models.ApprovalRequest, error) {
	var out []models.ApprovalRequest
	for _, request := range r.approvals {
		if request.Status == models.ApprovalStatusPending {
			out = append(out, *request)
		}
	}
	return out, nil
}

func (r *fakeConveyorRepository) ListApprovalRequests(ctx context.Context, workItemID uuid.UUID) ([]models.ApprovalRequest, error) {
	var out []models.ApprovalRequest
	for _, request := range r.approvals {
		if request.WorkItemID != nil && *request.WorkItemID == workItemID {
			out = append(out, *request)
		}
	}
	return out, nil
}

func (r *fakeConveyorRepository) setTaskAccess(taskID uuid.UUID, actorID uuid.UUID, allowed bool) {
	if r.taskAccess[taskID] == nil {
		r.taskAccess[taskID] = map[uuid.UUID]bool{}
	}
	r.taskAccess[taskID][actorID] = allowed
}

func (r *fakeConveyorRepository) WithTransaction(ctx context.Context, fn func(ConveyorRepository) error) error {
	return fn(r)
}
func (r *fakeConveyorRepository) GetTask(ctx context.Context, id uuid.UUID) (*models.Task, error) {
	v, ok := r.tasks[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *v
	return &cp, nil
}

func (r *fakeConveyorRepository) ActorCanAccessTask(ctx context.Context, actorID uuid.UUID, taskID uuid.UUID) (bool, error) {
	if byActor, ok := r.taskAccess[taskID]; ok {
		allowed, exists := byActor[actorID]
		return exists && allowed, nil
	}
	if _, ok := r.tasks[taskID]; !ok {
		return false, ErrNotFound
	}
	return true, nil
}

func (r *fakeConveyorRepository) CreateTask(ctx context.Context, task *models.Task) error {
	cp := *task
	r.tasks[cp.ID] = &cp
	return nil
}
func (r *fakeConveyorRepository) GetStatus(ctx context.Context, id uuid.UUID) (*models.Status, error) {
	v, ok := r.statuses[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *v
	return &cp, nil
}
func (r *fakeConveyorRepository) FindStatusByBoardName(ctx context.Context, boardID uuid.UUID, name string) (*models.Status, error) {
	for _, status := range r.statuses {
		if status.BoardID == boardID && status.Name != nil && *status.Name == name {
			cp := *status
			return &cp, nil
		}
	}
	return nil, ErrNotFound
}
func (r *fakeConveyorRepository) UpdateTaskStatus(ctx context.Context, taskID uuid.UUID, statusID uuid.UUID, at time.Time) error {
	r.tasks[taskID].StatusID = statusID
	r.tasks[taskID].UpdatedAt = &at
	return nil
}
func (r *fakeConveyorRepository) CreateAcceptanceCriterion(ctx context.Context, criterion *models.AcceptanceCriterion) error {
	cp := *criterion
	r.criteria[cp.ID] = &cp
	return nil
}
func (r *fakeConveyorRepository) GetAcceptanceCriterion(ctx context.Context, id uuid.UUID) (*models.AcceptanceCriterion, error) {
	v, ok := r.criteria[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *v
	return &cp, nil
}
func (r *fakeConveyorRepository) UpdateAcceptanceCriterionState(ctx context.Context, id uuid.UUID, state string, at time.Time) error {
	r.criteria[id].State = state
	r.criteria[id].UpdatedAt = at
	return nil
}
func (r *fakeConveyorRepository) ListAcceptanceCriteria(ctx context.Context, taskID uuid.UUID) ([]models.AcceptanceCriterion, error) {
	var out []models.AcceptanceCriterion
	for _, v := range r.criteria {
		if v.TaskID == taskID {
			out = append(out, *v)
		}
	}
	return out, nil
}
func (r *fakeConveyorRepository) CreateEvidence(ctx context.Context, evidence *models.Evidence) error {
	cp := *evidence
	r.evidence[cp.ID] = &cp
	return nil
}
func (r *fakeConveyorRepository) GetEvidence(ctx context.Context, id uuid.UUID) (*models.Evidence, error) {
	v, ok := r.evidence[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *v
	return &cp, nil
}
func (r *fakeConveyorRepository) RevokeEvidence(ctx context.Context, id uuid.UUID, actorID uuid.UUID, reason string, at time.Time) error {
	v := r.evidence[id]
	v.Revoked = true
	v.RevokedBy = &actorID
	v.RevokedReason = &reason
	v.RevokedAt = &at
	return nil
}
func (r *fakeConveyorRepository) ListEvidence(ctx context.Context, taskID uuid.UUID) ([]models.Evidence, error) {
	var out []models.Evidence
	for _, v := range r.evidence {
		if v.TaskID == taskID {
			out = append(out, *v)
		}
	}
	return out, nil
}
func (r *fakeConveyorRepository) CreateEvent(ctx context.Context, event *models.ConveyorEvent) error {
	cp := *event
	r.events = append(r.events, cp)
	return nil
}
func (r *fakeConveyorRepository) ListEvents(ctx context.Context, taskID uuid.UUID) ([]models.ConveyorEvent, error) {
	var out []models.ConveyorEvent
	for _, v := range r.events {
		if v.WorkItemID == taskID {
			out = append(out, v)
		}
	}
	return out, nil
}
func (r *fakeConveyorRepository) ListTaskLinks(ctx context.Context, taskID uuid.UUID) ([]models.TaskLink, error) {
	var out []models.TaskLink
	for _, v := range r.links {
		if v.SourceTaskID == taskID || v.TargetTaskID == taskID {
			out = append(out, v)
		}
	}
	return out, nil
}
func (r *fakeConveyorRepository) CreateTaskLink(ctx context.Context, link *models.TaskLink) error {
	key := linkKey(link.SourceTaskID, link.TargetTaskID, link.LinkType)
	if _, ok := r.links[key]; ok {
		return ErrConflict
	}
	r.links[key] = *link
	return nil
}
func (r *fakeConveyorRepository) TaskLinkExists(ctx context.Context, sourceID uuid.UUID, targetID uuid.UUID, linkType string) (bool, error) {
	_, ok := r.links[linkKey(sourceID, targetID, linkType)]
	return ok, nil
}
func (r *fakeConveyorRepository) ListDownstreamDependencies(ctx context.Context, upstreamID uuid.UUID) ([]models.TaskLink, error) {
	var out []models.TaskLink
	for _, link := range r.links {
		if (link.SourceTaskID == upstreamID && link.LinkType == models.TaskLinkTypeBlocks) || (link.TargetTaskID == upstreamID && link.LinkType == models.TaskLinkTypeBlockedBy) {
			out = append(out, link)
		}
	}
	return out, nil
}
func (r *fakeConveyorRepository) ListUpstreamDependencies(ctx context.Context, downstreamID uuid.UUID) ([]models.TaskLink, error) {
	var out []models.TaskLink
	for _, link := range r.links {
		if (link.TargetTaskID == downstreamID && link.LinkType == models.TaskLinkTypeBlocks) || (link.SourceTaskID == downstreamID && link.LinkType == models.TaskLinkTypeBlockedBy) {
			out = append(out, link)
		}
	}
	return out, nil
}
func (r *fakeConveyorRepository) CreateWorkOrder(ctx context.Context, workOrder *models.WorkOrder) error {
	cp := *workOrder
	r.workOrders[cp.ID] = &cp
	return nil
}
func (r *fakeConveyorRepository) GetWorkOrder(ctx context.Context, id uuid.UUID) (*models.WorkOrder, error) {
	workOrder, ok := r.workOrders[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *workOrder
	return &cp, nil
}
func (r *fakeConveyorRepository) UpdateWorkOrder(ctx context.Context, workOrder *models.WorkOrder) error {
	if _, ok := r.workOrders[workOrder.ID]; !ok {
		return ErrNotFound
	}
	cp := *workOrder
	r.workOrders[cp.ID] = &cp
	return nil
}
func (r *fakeConveyorRepository) CreateAgentRun(ctx context.Context, run *models.AgentRun) error {
	cp := *run
	r.agentRuns[cp.ID] = &cp
	return nil
}
func (r *fakeConveyorRepository) GetAgentRun(ctx context.Context, id uuid.UUID) (*models.AgentRun, error) {
	v, ok := r.agentRuns[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *v
	return &cp, nil
}
func (r *fakeConveyorRepository) ListAgentRuns(ctx context.Context, workItemID uuid.UUID) ([]models.AgentRun, error) {
	var out []models.AgentRun
	for _, v := range r.agentRuns {
		if v.WorkItemID == workItemID {
			out = append(out, *v)
		}
	}
	return out, nil
}
func (r *fakeConveyorRepository) CreateAgentInboxItem(ctx context.Context, item *models.AgentInboxItem) error {
	cp := *item
	r.inboxItems[cp.ID] = &cp
	return nil
}
func (r *fakeConveyorRepository) ListAgentInboxItems(ctx context.Context, recipientID uuid.UUID) ([]models.AgentInboxItem, error) {
	var out []models.AgentInboxItem
	for _, item := range r.inboxItems {
		if item.RecipientID == recipientID {
			out = append(out, *item)
		}
	}
	return out, nil
}
func (r *fakeConveyorRepository) GetAgentInboxItem(ctx context.Context, id uuid.UUID) (*models.AgentInboxItem, error) {
	item, ok := r.inboxItems[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *item
	return &cp, nil
}
func (r *fakeConveyorRepository) UpdateAgentInboxItemAck(ctx context.Context, itemID uuid.UUID, recipientID uuid.UUID, state string, at time.Time) error {
	item, ok := r.inboxItems[itemID]
	if !ok || item.RecipientID != recipientID {
		return ErrNotFound
	}
	item.AckState = state
	item.UpdatedAt = at
	return nil
}
func (r *fakeConveyorRepository) UpdateAgentRun(ctx context.Context, run *models.AgentRun) error {
	if _, ok := r.agentRuns[run.ID]; !ok {
		return ErrNotFound
	}
	cp := *run
	r.agentRuns[cp.ID] = &cp
	return nil
}
func (r *fakeConveyorRepository) ListStaleAgentRuns(ctx context.Context, cutoff time.Time) ([]models.AgentRun, error) {
	var out []models.AgentRun
	for _, v := range r.agentRuns {
		if isTerminalAgentRunStatus(v.Status) {
			continue
		}
		lastSeen := v.UpdatedAt
		if v.HeartbeatAt != nil {
			lastSeen = *v.HeartbeatAt
		}
		if lastSeen.Before(cutoff) {
			out = append(out, *v)
		}
	}
	return out, nil
}
func (r *fakeConveyorRepository) GetIdempotencyRecord(ctx context.Context, actorID uuid.UUID, operation string, key string) (*models.IdempotencyRecord, error) {
	v, ok := r.idempotency[actorID.String()+":"+operation+":"+key]
	if !ok {
		return nil, ErrNotFound
	}
	return &v, nil
}
func (r *fakeConveyorRepository) CreateIdempotencyRecord(ctx context.Context, record *models.IdempotencyRecord) error {
	k := record.ActorID.String() + ":" + record.Operation + ":" + record.IdempotencyKey
	if _, ok := r.idempotency[k]; ok {
		return ErrConflict
	}
	r.idempotency[k] = *record
	return nil
}
func (r *fakeConveyorRepository) ListProjectReportSources(ctx context.Context, projectID uuid.UUID, start time.Time, end time.Time) ([]ports.ProjectReportSource, error) {
	var out []ports.ProjectReportSource
	for _, task := range r.tasks {
		boardID, ok := r.statusBoard[task.StatusID]
		if !ok || r.boardProject[boardID] != projectID {
			continue
		}
		var taskEvents []models.ConveyorEvent
		for _, event := range r.events {
			if event.WorkItemID == task.ID && !event.Timestamp.Before(start) && !event.Timestamp.After(end) {
				taskEvents = append(taskEvents, event)
			}
		}
		var taskEvidence []models.Evidence
		for _, evidence := range r.evidence {
			if evidence.TaskID == task.ID {
				taskEvidence = append(taskEvidence, *evidence)
			}
		}
		out = append(out, ports.ProjectReportSource{Task: *task, Events: taskEvents, Evidence: taskEvidence})
	}
	return out, nil
}
func (r *fakeConveyorRepository) CreateGeneratedReport(ctx context.Context, report *models.GeneratedReport) error {
	cp := *report
	r.generatedReports[cp.ID] = &cp
	return nil
}
func (r *fakeConveyorRepository) GetGeneratedReport(ctx context.Context, id uuid.UUID) (*models.GeneratedReport, error) {
	v, ok := r.generatedReports[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *v
	return &cp, nil
}
func (r *fakeConveyorRepository) UpdateGeneratedReportLLM(ctx context.Context, id uuid.UUID, draft json.RawMessage, llmError string, at time.Time) error {
	report, ok := r.generatedReports[id]
	if !ok {
		return ErrNotFound
	}
	report.LLMDraft = draft
	report.LLMError = llmError
	report.UpdatedAt = at
	return nil
}
func (r *fakeConveyorRepository) CreateForumDigest(ctx context.Context, digest *models.ForumDigest) error {
	cp := *digest
	r.forumDigests[cp.ID] = &cp
	return nil
}
func (r *fakeConveyorRepository) GetForumDigest(ctx context.Context, id uuid.UUID) (*models.ForumDigest, error) {
	v, ok := r.forumDigests[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *v
	return &cp, nil
}
func (r *fakeConveyorRepository) ListForumDigests(ctx context.Context, sourceType string, sourceID string) ([]models.ForumDigest, error) {
	var out []models.ForumDigest
	for _, digest := range r.forumDigests {
		if sourceType != "" && digest.SourceType != sourceType {
			continue
		}
		if sourceID != "" && digest.SourceID != sourceID {
			continue
		}
		out = append(out, *digest)
	}
	return out, nil
}
func (r *fakeConveyorRepository) CreateForumActionCandidate(ctx context.Context, candidate *models.ForumActionCandidate) error {
	cp := *candidate
	r.forumCandidates[cp.ID] = &cp
	return nil
}
func (r *fakeConveyorRepository) GetForumActionCandidate(ctx context.Context, id uuid.UUID) (*models.ForumActionCandidate, error) {
	v, ok := r.forumCandidates[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *v
	return &cp, nil
}
func (r *fakeConveyorRepository) ListForumActionCandidates(ctx context.Context, digestID uuid.UUID) ([]models.ForumActionCandidate, error) {
	var out []models.ForumActionCandidate
	for _, candidate := range r.forumCandidates {
		if candidate.DigestID == digestID {
			out = append(out, *candidate)
		}
	}
	return out, nil
}
func (r *fakeConveyorRepository) UpdateForumActionCandidate(ctx context.Context, candidate *models.ForumActionCandidate) error {
	if _, ok := r.forumCandidates[candidate.ID]; !ok {
		return ErrNotFound
	}
	cp := *candidate
	r.forumCandidates[cp.ID] = &cp
	return nil
}

func (r *fakeConveyorRepository) addTask(open bool) uuid.UUID {
	statusID := r.addStatus(open)
	id := uuid.New()
	now := time.Now()
	deleted := false
	r.tasks[id] = &models.Task{ID: id, StatusID: statusID, Deleted: &deleted, CreatedAt: &now, UpdatedAt: &now}
	return id
}

func (r *fakeConveyorRepository) addProjectTask(projectID uuid.UUID, name string, open bool) uuid.UUID {
	boardID := uuid.New()
	statusID := r.addStatus(open)
	r.statusBoard[statusID] = boardID
	r.boardProject[boardID] = projectID
	id := uuid.New()
	now := time.Now()
	deleted := false
	r.tasks[id] = &models.Task{ID: id, Name: &name, StatusID: statusID, Deleted: &deleted, CreatedAt: &now, UpdatedAt: &now}
	return id
}

func (r *fakeConveyorRepository) addStatus(open bool) uuid.UUID {
	id := uuid.New()
	r.statuses[id] = &models.Status{ID: id, IsOpen: &open}
	return id
}

func (r *fakeConveyorRepository) addCriterion(taskID uuid.UUID, required bool, state string) uuid.UUID {
	id := uuid.New()
	now := time.Now()
	r.criteria[id] = &models.AcceptanceCriterion{ID: id, TaskID: taskID, Title: "criterion", Required: required, State: state, CreatedAt: now, UpdatedAt: now}
	return id
}

func (r *fakeConveyorRepository) addEvidence(taskID uuid.UUID, verdict string, revoked bool) uuid.UUID {
	id := uuid.New()
	now := time.Now()
	r.evidence[id] = &models.Evidence{ID: id, TaskID: taskID, Type: models.EvidenceTypeLink, Verdict: verdict, URI: "https://example.test/evidence", Revoked: revoked, CreatedAt: now}
	return id
}

func (r *fakeConveyorRepository) addEvidenceWithTitle(taskID uuid.UUID, verdict string, revoked bool, title string) uuid.UUID {
	id := uuid.New()
	now := time.Now()
	r.evidence[id] = &models.Evidence{ID: id, TaskID: taskID, Type: models.EvidenceTypeLink, Verdict: verdict, URI: "https://example.test/evidence", Title: title, Revoked: revoked, CreatedAt: now}
	return id
}

func (r *fakeConveyorRepository) addEvent(workItemID uuid.UUID, eventType string, at time.Time, actorID uuid.UUID, payload any) uuid.UUID {
	id := uuid.New()
	encoded, err := json.Marshal(payload)
	if err != nil {
		panic(fmt.Sprintf("encode event payload: %v", err))
	}
	r.events = append(r.events, models.ConveyorEvent{ID: id, WorkItemID: workItemID, Type: eventType, Timestamp: at, ActorType: "user", ActorID: actorID, Payload: encoded, SchemaVersion: 1, Source: "test"})
	return id
}

func (r *fakeConveyorRepository) addAgentRun(workItemID uuid.UUID, status string, updatedAt time.Time, heartbeatAt *time.Time) uuid.UUID {
	id := uuid.New()
	r.agentRuns[id] = &models.AgentRun{ID: id, WorkItemID: workItemID, Source: "test", Harness: "test-harness", Status: status, CreatedBy: uuid.New(), CreatedAt: updatedAt, UpdatedAt: updatedAt, HeartbeatAt: heartbeatAt}
	return id
}

func testActor() ConveyorActor {
	return ConveyorActor{ActorType: "user", ActorID: uuid.New(), Source: "test"}
}

func linkKey(sourceID uuid.UUID, targetID uuid.UUID, linkType string) string {
	return sourceID.String() + ":" + targetID.String() + ":" + linkType
}

func ptrUUID(v uuid.UUID) *uuid.UUID { return &v }

func collectEventTypes(events []models.ConveyorEvent) []string {
	types := make([]string, 0, len(events))
	for _, event := range events {
		types = append(types, event.Type)
	}
	return types
}

func findAgentRunEvent(t *testing.T, events []models.ConveyorEvent, agentRunID uuid.UUID) models.ConveyorEvent {
	t.Helper()
	for _, event := range events {
		var payload struct {
			AgentRunID uuid.UUID `json:"agent_run_id"`
		}
		require.NoError(t, json.Unmarshal(event.Payload, &payload))
		if payload.AgentRunID == agentRunID {
			return event
		}
	}
	t.Fatalf("agent run event not found: %s", agentRunID)
	return models.ConveyorEvent{}
}

func firstAgentRun(repo *fakeConveyorRepository) *models.AgentRun {
	for _, run := range repo.agentRuns {
		return run
	}
	return nil
}

type fakeModeLLM struct {
	modeResult string
	modeErr    error
	modes      []string
	inputs     []string
}

func (f *fakeModeLLM) EnrichGeneratedReport(ctx context.Context, req GeneratedReportLLMRequest) (string, error) {
	return f.modeResult, f.modeErr
}

func (f *fakeModeLLM) GenerateWithMode(ctx context.Context, mode string, description string, input string, refID string) (string, error) {
	f.modes = append(f.modes, mode)
	f.inputs = append(f.inputs, input)
	if f.modeErr != nil {
		return "", f.modeErr
	}
	return f.modeResult, nil
}

func TestConveyorServiceLLMModeWiring(t *testing.T) {
	repo := newFakeConveyorRepository()
	actor := testActor()
	llm := &fakeModeLLM{modeResult: "AI-сгенерированное резюме"}
	svc := NewConveyorServiceWithReportLLM(repo, llm)
	ctx := context.Background()

	digest, err := svc.CreateForumDigest(ctx, actor, CreateForumDigestRequest{
		SourceType: models.ConversationSourceTypeForum,
		SourceID:   "problem-1",
		Summary:    "caller summary",
		Messages:   []ForumDigestSourceMessage{{Locator: "m1", Author: "alice", Text: "we should add a healthcheck", Accessible: true}},
		UseLLM:     true,
	})
	require.NoError(t, err)
	require.Equal(t, "AI-сгенерированное резюме", digest.Summary)
	require.Contains(t, llm.modes, "forum_digest")
	require.Contains(t, llm.inputs[len(llm.inputs)-1], "healthcheck")

	taskID := repo.addTask(true)
	name := "Add healthcheck endpoint"
	repo.tasks[taskID].Name = &name
	suggestion, err := svc.SuggestCriteria(ctx, actor, taskID)
	require.NoError(t, err)
	require.Equal(t, "AI-сгенерированное резюме", suggestion)
	require.Contains(t, llm.modes, "criteria_suggestion")

	repo.addEvidence(taskID, models.EvidenceVerdictSupports, false)
	summary, err := svc.SummarizeEvidence(ctx, actor, taskID)
	require.NoError(t, err)
	require.Equal(t, "AI-сгенерированное резюме", summary)
	require.Contains(t, llm.modes, "evidence_summary")

	// Without an LLM configured the draft-only modes report a clear error.
	if _, err := NewConveyorService(repo).SuggestCriteria(ctx, actor, taskID); err == nil {
		t.Fatal("expected SuggestCriteria to fail without an LLM client")
	}
}

type fakeGeneratedReportLLM struct {
	result   string
	err      error
	requests []GeneratedReportLLMRequest
}

func (f *fakeGeneratedReportLLM) EnrichGeneratedReport(ctx context.Context, req GeneratedReportLLMRequest) (string, error) {
	f.requests = append(f.requests, req)
	if f.err != nil {
		return "", f.err
	}
	return f.result, nil
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	return data
}

var _ = errors.Is
var _ = mustJSON
