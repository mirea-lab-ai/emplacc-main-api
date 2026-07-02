package ports

import (
	"context"
	"encoding/json"
	"time"

	models "emplacc-api/internal/domain"

	"github.com/google/uuid"
)

// ProjectReportSource is an aggregate row returned by ListProjectReportSources.
// It is part of the ConveyorRepository port contract.
type ProjectReportSource struct {
	Task     models.Task
	Events   []models.ConveyorEvent
	Evidence []models.Evidence
}

type ConveyorRepository interface {
	WithTransaction(ctx context.Context, fn func(ConveyorRepository) error) error
	GetTask(ctx context.Context, id uuid.UUID) (*models.Task, error)
	ActorCanAccessTask(ctx context.Context, actorID uuid.UUID, taskID uuid.UUID) (bool, error)
	CreateTask(ctx context.Context, task *models.Task) error
	GetStatus(ctx context.Context, id uuid.UUID) (*models.Status, error)
	FindStatusByBoardName(ctx context.Context, boardID uuid.UUID, name string) (*models.Status, error)
	UpdateTaskStatus(ctx context.Context, taskID uuid.UUID, statusID uuid.UUID, at time.Time) error
	CreateAcceptanceCriterion(ctx context.Context, criterion *models.AcceptanceCriterion) error
	GetAcceptanceCriterion(ctx context.Context, id uuid.UUID) (*models.AcceptanceCriterion, error)
	UpdateAcceptanceCriterionState(ctx context.Context, id uuid.UUID, state string, at time.Time) error
	ListAcceptanceCriteria(ctx context.Context, taskID uuid.UUID) ([]models.AcceptanceCriterion, error)
	CreateEvidence(ctx context.Context, evidence *models.Evidence) error
	GetEvidence(ctx context.Context, id uuid.UUID) (*models.Evidence, error)
	RevokeEvidence(ctx context.Context, id uuid.UUID, actorID uuid.UUID, reason string, at time.Time) error
	ListEvidence(ctx context.Context, taskID uuid.UUID) ([]models.Evidence, error)
	CreateEvent(ctx context.Context, event *models.ConveyorEvent) error
	ListEvents(ctx context.Context, taskID uuid.UUID) ([]models.ConveyorEvent, error)
	CreateTaskLink(ctx context.Context, link *models.TaskLink) error
	TaskLinkExists(ctx context.Context, sourceID uuid.UUID, targetID uuid.UUID, linkType string) (bool, error)
	ListTaskLinks(ctx context.Context, taskID uuid.UUID) ([]models.TaskLink, error)
	ListDownstreamDependencies(ctx context.Context, upstreamID uuid.UUID) ([]models.TaskLink, error)
	ListUpstreamDependencies(ctx context.Context, downstreamID uuid.UUID) ([]models.TaskLink, error)
	CreateWorkOrder(ctx context.Context, workOrder *models.WorkOrder) error
	GetWorkOrder(ctx context.Context, id uuid.UUID) (*models.WorkOrder, error)
	ListWorkOrdersByTask(ctx context.Context, taskID uuid.UUID) ([]models.WorkOrder, error)
	UpdateWorkOrder(ctx context.Context, workOrder *models.WorkOrder) error
	CreateAgentRun(ctx context.Context, run *models.AgentRun) error
	GetAgentRun(ctx context.Context, id uuid.UUID) (*models.AgentRun, error)
	ListAgentRuns(ctx context.Context, workItemID uuid.UUID) ([]models.AgentRun, error)
	ListStaleAgentRuns(ctx context.Context, cutoff time.Time) ([]models.AgentRun, error)
	UpdateAgentRun(ctx context.Context, run *models.AgentRun) error
	CreateAgentInboxItem(ctx context.Context, item *models.AgentInboxItem) error
	ListAgentInboxItems(ctx context.Context, recipientID uuid.UUID) ([]models.AgentInboxItem, error)
	GetAgentInboxItem(ctx context.Context, id uuid.UUID) (*models.AgentInboxItem, error)
	UpdateAgentInboxItemAck(ctx context.Context, itemID uuid.UUID, recipientID uuid.UUID, state string, at time.Time) error
	GetIdempotencyRecord(ctx context.Context, actorID uuid.UUID, operation string, key string) (*models.IdempotencyRecord, error)
	CreateIdempotencyRecord(ctx context.Context, record *models.IdempotencyRecord) error
	ListProjectReportSources(ctx context.Context, projectID uuid.UUID, start time.Time, end time.Time) ([]ProjectReportSource, error)
	CreateGeneratedReport(ctx context.Context, report *models.GeneratedReport) error
	GetGeneratedReport(ctx context.Context, id uuid.UUID) (*models.GeneratedReport, error)
	UpdateGeneratedReportLLM(ctx context.Context, id uuid.UUID, draft json.RawMessage, llmError string, at time.Time) error
	CreateForumDigest(ctx context.Context, digest *models.ForumDigest) error
	GetForumDigest(ctx context.Context, id uuid.UUID) (*models.ForumDigest, error)
	ListForumDigests(ctx context.Context, sourceType string, sourceID string) ([]models.ForumDigest, error)
	CreateForumActionCandidate(ctx context.Context, candidate *models.ForumActionCandidate) error
	GetForumActionCandidate(ctx context.Context, id uuid.UUID) (*models.ForumActionCandidate, error)
	ListForumActionCandidates(ctx context.Context, digestID uuid.UUID) ([]models.ForumActionCandidate, error)
	UpdateForumActionCandidate(ctx context.Context, candidate *models.ForumActionCandidate) error
	CreateWaiver(ctx context.Context, waiver *models.Waiver) error
	GetWaiver(ctx context.Context, id uuid.UUID) (*models.Waiver, error)
	CreateApprovalRequest(ctx context.Context, request *models.ApprovalRequest) error
	GetApprovalRequest(ctx context.Context, id uuid.UUID) (*models.ApprovalRequest, error)
	UpdateApprovalRequest(ctx context.Context, request *models.ApprovalRequest) error
	ListApprovalRequests(ctx context.Context, workItemID uuid.UUID) ([]models.ApprovalRequest, error)
	ListPendingApprovals(ctx context.Context, limit int) ([]models.ApprovalRequest, error)
}
