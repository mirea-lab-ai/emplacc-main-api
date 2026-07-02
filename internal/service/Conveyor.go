package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"regexp"
	"sort"
	"strings"
	"time"

	models "emplacc-api/internal/domain"
	llmclient "emplacc-api/internal/grpc/client"
	"emplacc-api/internal/ports"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

var (
	ErrValidation       = models.ErrConveyorValidation
	ErrNotFound         = models.ErrConveyorNotFound
	ErrConflict         = models.ErrConveyorConflict
	ErrApprovalRequired = models.ErrConveyorApprovalRequired
	ErrPermissionDenied = models.ErrConveyorPermissionDenied
	forumMarkupPattern  = regexp.MustCompile(`<[^>]*>`)
	secretKeyPattern    = regexp.MustCompile(`(?i)(secret|token|password|passwd|api[_-]?key|authorization|credential|private[_-]?key)`)
	secretValuePattern  = regexp.MustCompile(`(?i)(emplacc_[A-Za-z0-9._-]+|sess_[A-Za-z0-9._-]+|Bearer\s+[A-Za-z0-9._-]+|(?:sk|pk|rk|ghp|github_pat|xox[baprs])-[-a-z0-9_]{12,}|eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+)`)
)

type ConveyorRepository = ports.ConveyorRepository

type ConveyorService interface {
	AuthorizeWorkItemAccess(ctx context.Context, actor ConveyorActor, taskID uuid.UUID) error
	CreateAcceptanceCriterion(ctx context.Context, actor ConveyorActor, req CreateAcceptanceCriterionRequest) (*ConveyorMutationResult, error)
	UpdateAcceptanceCriterionState(ctx context.Context, actor ConveyorActor, taskID uuid.UUID, criterionID uuid.UUID, req UpdateCriterionStateRequest) (*ConveyorMutationResult, error)
	AttachEvidence(ctx context.Context, actor ConveyorActor, req AttachEvidenceRequest) (*ConveyorMutationResult, error)
	RevokeEvidence(ctx context.Context, actor ConveyorActor, taskID uuid.UUID, evidenceID uuid.UUID, req RevokeEvidenceRequest) (*ConveyorMutationResult, error)
	LinkTasks(ctx context.Context, actor ConveyorActor, req LinkTasksRequest) (*ConveyorMutationResult, error)
	CloseTask(ctx context.Context, actor ConveyorActor, taskID uuid.UUID, req CloseTaskRequest) (*ConveyorMutationResult, error)
	CreateWorkOrder(ctx context.Context, actor ConveyorActor, req CreateWorkOrderRequest) (*ConveyorMutationResult, error)
	GetWorkOrder(ctx context.Context, workOrderID uuid.UUID) (*models.WorkOrder, error)
	ListWorkOrders(ctx context.Context, taskID uuid.UUID) ([]models.WorkOrder, error)
	AcceptWorkOrder(ctx context.Context, actor ConveyorActor, workOrderID uuid.UUID, req AcceptWorkOrderRequest) (*ConveyorMutationResult, error)
	RejectWorkOrder(ctx context.Context, actor ConveyorActor, workOrderID uuid.UUID, req RejectWorkOrderRequest) (*ConveyorMutationResult, error)
	CompleteWorkOrder(ctx context.Context, actor ConveyorActor, workOrderID uuid.UUID, req CompleteWorkOrderRequest) (*ConveyorMutationResult, error)
	CancelWorkOrder(ctx context.Context, actor ConveyorActor, workOrderID uuid.UUID, req CancelWorkOrderRequest) (*ConveyorMutationResult, error)
	FailWorkOrder(ctx context.Context, actor ConveyorActor, workOrderID uuid.UUID, req FailWorkOrderRequest) (*ConveyorMutationResult, error)
	RegisterAgentRun(ctx context.Context, actor ConveyorActor, req RegisterAgentRunRequest) (*ConveyorMutationResult, error)
	UpdateAgentRun(ctx context.Context, actor ConveyorActor, workItemID uuid.UUID, agentRunID uuid.UUID, req UpdateAgentRunRequest) (*ConveyorMutationResult, error)
	HeartbeatAgentRun(ctx context.Context, actor ConveyorActor, workItemID uuid.UUID, agentRunID uuid.UUID) (*ConveyorMutationResult, error)
	MarkStaleAgentRunsFailed(ctx context.Context, actor ConveyorActor, cutoff time.Time) ([]uuid.UUID, error)
	GetAgentRun(ctx context.Context, workItemID uuid.UUID, agentRunID uuid.UUID) (*models.AgentRun, error)
	ListAgentRuns(ctx context.Context, workItemID uuid.UUID) ([]models.AgentRun, error)
	ListAgentInbox(ctx context.Context, actor ConveyorActor) ([]AgentInboxItemResponse, error)
	AckAgentInboxItem(ctx context.Context, actor ConveyorActor, req AckAgentInboxItemRequest) (*AgentInboxItemResponse, error)
	GenerateProjectReport(ctx context.Context, actor ConveyorActor, req GenerateProjectReportRequest) (*GeneratedReportResponse, error)
	GetGeneratedReport(ctx context.Context, reportID uuid.UUID) (*GeneratedReportResponse, error)
	ExportGeneratedReportMarkdown(ctx context.Context, reportID uuid.UUID) (string, error)
	SuggestCriteria(ctx context.Context, actor ConveyorActor, taskID uuid.UUID) (string, error)
	SummarizeEvidence(ctx context.Context, actor ConveyorActor, taskID uuid.UUID) (string, error)
	CreateForumDigest(ctx context.Context, actor ConveyorActor, req CreateForumDigestRequest) (*ForumDigestResponse, error)
	GetForumDigest(ctx context.Context, digestID uuid.UUID) (*ForumDigestResponse, error)
	ListForumDigests(ctx context.Context, sourceType string, sourceID string) ([]ForumDigestResponse, error)
	ConfirmForumActionCandidate(ctx context.Context, actor ConveyorActor, candidateID uuid.UUID, req ConfirmForumActionCandidateRequest) (*ConveyorMutationResult, error)
	RejectForumActionCandidate(ctx context.Context, actor ConveyorActor, candidateID uuid.UUID, req RejectForumActionCandidateRequest) (*ConveyorMutationResult, error)
	ListAcceptanceCriteria(ctx context.Context, taskID uuid.UUID) ([]models.AcceptanceCriterion, error)
	ListEvidence(ctx context.Context, taskID uuid.UUID) ([]models.Evidence, error)
	ListEvents(ctx context.Context, taskID uuid.UUID) ([]models.ConveyorEvent, error)
	ListTaskLinks(ctx context.Context, taskID uuid.UUID) ([]models.TaskLink, error)
	CreateWaiver(ctx context.Context, actor ConveyorActor, req CreateWaiverRequest) (*ConveyorMutationResult, error)
	GetWaiver(ctx context.Context, id uuid.UUID) (*models.Waiver, error)
	RequestApproval(ctx context.Context, actor ConveyorActor, req RequestApprovalRequest) (*ConveyorMutationResult, error)
	GrantApproval(ctx context.Context, actor ConveyorActor, approvalID uuid.UUID, req DecideApprovalRequest) (*ConveyorMutationResult, error)
	DenyApproval(ctx context.Context, actor ConveyorActor, approvalID uuid.UUID, req DecideApprovalRequest) (*ConveyorMutationResult, error)
	GetApprovalRequest(ctx context.Context, id uuid.UUID) (*models.ApprovalRequest, error)
	ListApprovalRequests(ctx context.Context, workItemID uuid.UUID) ([]models.ApprovalRequest, error)
	ListPendingApprovals(ctx context.Context, limit int) ([]models.ApprovalRequest, error)
}

type conveyorService struct {
	repo      ConveyorRepository
	reportLLM GeneratedReportLLMClient
	llmMode   ConveyorLLMModeClient
}

// ConveyorLLMModeClient runs a mode-scoped LLM prompt through emplacc-main-api ->
// reports_llm_ms (RUN-08). mode is the reports_llm_ms meta["mode"] value
// (forum_digest | evidence_summary | criteria_suggestion).
type ConveyorLLMModeClient interface {
	GenerateWithMode(ctx context.Context, mode string, description string, input string, refID string) (string, error)
}

type ConveyorActor struct {
	ActorType        string
	ActorID          uuid.UUID
	AgentID          *string
	OnBehalfOfUserID *uuid.UUID
	APITokenID       *uuid.UUID
	CorrelationID    *string
	RequestID        *string
	Source           string
}

type ConveyorMutationResult struct {
	EntityID uuid.UUID `json:"entity_id"`
	EventID  uuid.UUID `json:"event_id"`
	Replayed bool      `json:"replayed,omitempty"`
}

type CreateAcceptanceCriterionRequest struct {
	TaskID         uuid.UUID `json:"task_id"`
	ACID           string    `json:"ac_id,omitempty"`
	Title          string    `json:"title"`
	SpecIDs        []string  `json:"spec_ids,omitempty"`
	Required       bool      `json:"required"`
	IdempotencyKey string    `json:"idempotency_key,omitempty"`
}

type UpdateCriterionStateRequest struct {
	State          string `json:"state"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
}

type AttachEvidenceRequest struct {
	TaskID          uuid.UUID       `json:"task_id"`
	CriterionID     *uuid.UUID      `json:"criterion_id,omitempty"`
	Type            string          `json:"type"`
	Verdict         string          `json:"verdict"`
	URI             string          `json:"uri"`
	Title           string          `json:"title"`
	SHA256          string          `json:"sha256,omitempty"`
	Metadata        json.RawMessage `json:"metadata,omitempty"`
	ApprovalGranted bool            `json:"approval_granted,omitempty"`
	ApprovalToken   string          `json:"approval_token,omitempty"`
	IdempotencyKey  string          `json:"idempotency_key,omitempty"`
}

type RevokeEvidenceRequest struct {
	Reason         string `json:"reason"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
}

type LinkTasksRequest struct {
	SourceTaskID   uuid.UUID `json:"source_task_id"`
	TargetTaskID   uuid.UUID `json:"target_task_id"`
	LinkType       string    `json:"link_type"`
	IdempotencyKey string    `json:"idempotency_key,omitempty"`
}

type CloseTaskRequest struct {
	ToStatusID               uuid.UUID  `json:"to_status_id"`
	TaskWaiverID             *uuid.UUID `json:"task_waiver_id,omitempty"`
	RiskLevel                string     `json:"risk_level,omitempty"`
	ApprovalGranted          bool       `json:"approval_granted,omitempty"`
	ApprovalToken            string     `json:"approval_token,omitempty"`
	AllowDependencyAutoReady bool       `json:"allow_dependency_auto_ready,omitempty"`
	IdempotencyKey           string     `json:"idempotency_key,omitempty"`
}

type CreateWaiverRequest struct {
	Scope             string     `json:"scope"`
	WorkItemID        uuid.UUID  `json:"work_item_id"`
	CriterionID       *uuid.UUID `json:"criterion_id,omitempty"`
	Reason            string     `json:"reason"`
	ApprovalRequestID *uuid.UUID `json:"approval_request_id,omitempty"`
	ExpiresAt         *time.Time `json:"expires_at,omitempty"`
	IdempotencyKey    string     `json:"idempotency_key,omitempty"`
}

type RequestApprovalRequest struct {
	WorkItemID     *uuid.UUID      `json:"work_item_id,omitempty"`
	Action         string          `json:"action"`
	RiskLevel      string          `json:"risk_level,omitempty"`
	Reason         string          `json:"reason,omitempty"`
	Resource       json.RawMessage `json:"resource,omitempty"`
	ExpiresAt      *time.Time      `json:"expires_at,omitempty"`
	IdempotencyKey string          `json:"idempotency_key,omitempty"`
}

type DecideApprovalRequest struct {
	Reason         string `json:"reason,omitempty"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
}

type CreateWorkOrderRequest struct {
	SourceTaskID             uuid.UUID       `json:"source_task_id"`
	ProviderBoardID          uuid.UUID       `json:"provider_board_id"`
	ProviderStatusID         uuid.UUID       `json:"provider_status_id"`
	Goal                     string          `json:"goal"`
	Inputs                   json.RawMessage `json:"inputs,omitempty"`
	AcceptanceCriteria       json.RawMessage `json:"acceptance_criteria,omitempty"`
	RequiredEvidenceMetadata json.RawMessage `json:"required_evidence_metadata,omitempty"`
	RequesterContext         json.RawMessage `json:"requester_context,omitempty"`
	ProviderContext          json.RawMessage `json:"provider_context,omitempty"`
	IdempotencyKey           string          `json:"idempotency_key,omitempty"`
}

type AcceptWorkOrderRequest struct {
	TargetName     string `json:"target_name,omitempty"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
}

type RejectWorkOrderRequest struct {
	Reason         string `json:"reason"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
}

type CompleteWorkOrderRequest struct {
	ResultEvidenceID *uuid.UUID `json:"result_evidence_id,omitempty"`
	EvidenceWaiver   string     `json:"evidence_waiver,omitempty"`
	IdempotencyKey   string     `json:"idempotency_key,omitempty"`
}

type CancelWorkOrderRequest struct {
	Reason         string `json:"reason"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
}

type FailWorkOrderRequest struct {
	Reason         string `json:"reason"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
}

type RegisterAgentRunRequest struct {
	WorkItemID     uuid.UUID       `json:"work_item_id"`
	Source         string          `json:"source"`
	Harness        string          `json:"harness"`
	Status         string          `json:"status"`
	Summary        string          `json:"summary"`
	LogURI         string          `json:"log_uri"`
	WorkspaceURI   string          `json:"workspace_uri"`
	Metadata       json.RawMessage `json:"metadata,omitempty"`
	IdempotencyKey string          `json:"idempotency_key,omitempty"`
}

type UpdateAgentRunRequest struct {
	Status         string          `json:"status"`
	Summary        string          `json:"summary"`
	LogURI         string          `json:"log_uri"`
	WorkspaceURI   string          `json:"workspace_uri"`
	Metadata       json.RawMessage `json:"metadata,omitempty"`
	IdempotencyKey string          `json:"idempotency_key,omitempty"`
}

type GenerateProjectReportRequest struct {
	ProjectID      uuid.UUID `json:"project_id"`
	PeriodStart    time.Time `json:"period_start"`
	PeriodEnd      time.Time `json:"period_end"`
	IncludeLLM     bool      `json:"include_llm"`
	IdempotencyKey string    `json:"idempotency_key,omitempty"`
}

type ForumDigestSourceMessage struct {
	Locator    string `json:"locator"`
	Author     string `json:"author,omitempty"`
	Text       string `json:"text"`
	Accessible bool   `json:"accessible"`
	Redacted   bool   `json:"redacted,omitempty"`
}

type ForumActionCandidateInput struct {
	Title          string          `json:"title"`
	Description    string          `json:"description"`
	SourceLocator  string          `json:"source_locator"`
	SourceMetadata json.RawMessage `json:"source_metadata,omitempty"`
}

type CreateForumDigestRequest struct {
	SourceType      string                      `json:"source_type"`
	SourceID        string                      `json:"source_id"`
	SourceTitle     string                      `json:"source_title"`
	SourceLocator   string                      `json:"source_locator"`
	PeriodStart     time.Time                   `json:"period_start,omitempty"`
	PeriodEnd       time.Time                   `json:"period_end,omitempty"`
	Summary         string                      `json:"summary"`
	Decisions       json.RawMessage             `json:"decisions,omitempty"`
	SourceMetadata  json.RawMessage             `json:"source_metadata,omitempty"`
	Messages        []ForumDigestSourceMessage  `json:"messages,omitempty"`
	CandidateInputs []ForumActionCandidateInput `json:"candidate_inputs,omitempty"`
	// UseLLM asks the backend to generate the digest summary from Messages via
	// reports_llm_ms (mode=forum_digest) instead of trusting a caller-supplied Summary.
	UseLLM bool `json:"use_llm,omitempty"`
}

type ForumActionCandidateResponse struct {
	ID             uuid.UUID       `json:"id"`
	DigestID       uuid.UUID       `json:"digest_id"`
	Title          string          `json:"title"`
	Description    string          `json:"description"`
	SourceLocator  string          `json:"source_locator"`
	SourceMetadata json.RawMessage `json:"source_metadata,omitempty"`
	Status         string          `json:"status"`
	WorkItemID     *uuid.UUID      `json:"work_item_id,omitempty"`
	CreatedBy      uuid.UUID       `json:"created_by"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

type ForumDigestResponse struct {
	ID             uuid.UUID                      `json:"id"`
	SourceType     string                         `json:"source_type"`
	SourceID       string                         `json:"source_id"`
	SourceTitle    string                         `json:"source_title"`
	SourceLocator  string                         `json:"source_locator"`
	PeriodStart    *time.Time                     `json:"period_start,omitempty"`
	PeriodEnd      *time.Time                     `json:"period_end,omitempty"`
	Summary        string                         `json:"summary"`
	Decisions      json.RawMessage                `json:"decisions,omitempty"`
	SourceMessages json.RawMessage                `json:"source_messages"`
	SourceMetadata json.RawMessage                `json:"source_metadata,omitempty"`
	Candidates     []ForumActionCandidateResponse `json:"candidates"`
	CreatedBy      uuid.UUID                      `json:"created_by"`
	ActorType      string                         `json:"actor_type"`
	CreatedAt      time.Time                      `json:"created_at"`
	UpdatedAt      time.Time                      `json:"updated_at"`
}

type ConfirmForumActionCandidateRequest struct {
	StatusID       uuid.UUID  `json:"status_id"`
	AssignedTo     *uuid.UUID `json:"assigned_to,omitempty"`
	Priority       *int16     `json:"priority,omitempty"`
	IdempotencyKey string     `json:"idempotency_key,omitempty"`
}

type RejectForumActionCandidateRequest struct {
	Reason         string `json:"reason"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
}

type GeneratedReportItem struct {
	Kind              string      `json:"kind"`
	Text              string      `json:"text"`
	WorkItemID        uuid.UUID   `json:"work_item_id,omitempty"`
	SourceEventIDs    []uuid.UUID `json:"source_event_ids,omitempty"`
	SourceEvidenceIDs []uuid.UUID `json:"source_evidence_ids,omitempty"`
}

type GeneratedReportDraft struct {
	Text      string `json:"text"`
	Auxiliary bool   `json:"auxiliary"`
}

type GeneratedReportResponse struct {
	ID                uuid.UUID             `json:"id"`
	ProjectID         uuid.UUID             `json:"project_id"`
	PeriodStart       time.Time             `json:"period_start"`
	PeriodEnd         time.Time             `json:"period_end"`
	SourceEventIDs    []uuid.UUID           `json:"source_event_ids"`
	SourceEvidenceIDs []uuid.UUID           `json:"source_evidence_ids"`
	Facts             []GeneratedReportItem `json:"facts"`
	Conclusions       []GeneratedReportItem `json:"conclusions"`
	Risks             []GeneratedReportItem `json:"risks"`
	LLMDraft          GeneratedReportDraft  `json:"llm_draft,omitempty"`
	LLMError          string                `json:"llm_error,omitempty"`
	CreatedBy         uuid.UUID             `json:"created_by"`
	ActorType         string                `json:"actor_type"`
	CreatedAt         time.Time             `json:"created_at"`
	UpdatedAt         time.Time             `json:"updated_at"`
}

type GeneratedReportLLMRequest struct {
	ReportID          uuid.UUID
	ProjectID         uuid.UUID
	PeriodStart       time.Time
	PeriodEnd         time.Time
	Facts             []GeneratedReportItem
	Conclusions       []GeneratedReportItem
	Risks             []GeneratedReportItem
	SourceEventIDs    []uuid.UUID
	SourceEvidenceIDs []uuid.UUID
}

type GeneratedReportLLMClient interface {
	EnrichGeneratedReport(ctx context.Context, req GeneratedReportLLMRequest) (string, error)
}

type BackendGeneratedReportLLMClient struct {
	client interface {
		ProcessTaskWithLLM(ctx context.Context, taskDescription string, userText string, taskID string, overrides *llmclient.LLMOverrides) (string, error)
	}
}

func NewBackendGeneratedReportLLMClient(client interface {
	ProcessTaskWithLLM(ctx context.Context, taskDescription string, userText string, taskID string, overrides *llmclient.LLMOverrides) (string, error)
}) *BackendGeneratedReportLLMClient {
	return &BackendGeneratedReportLLMClient{client: client}
}

func (c *BackendGeneratedReportLLMClient) EnrichGeneratedReport(ctx context.Context, req GeneratedReportLLMRequest) (string, error) {
	if c == nil || c.client == nil {
		return "", fmt.Errorf("report LLM client is not configured")
	}
	text := generatedReportMarkdown(&GeneratedReportResponse{ID: req.ReportID, ProjectID: req.ProjectID, PeriodStart: req.PeriodStart, PeriodEnd: req.PeriodEnd, SourceEventIDs: req.SourceEventIDs, SourceEvidenceIDs: req.SourceEvidenceIDs, Facts: req.Facts, Conclusions: req.Conclusions, Risks: req.Risks})
	return c.client.ProcessTaskWithLLM(ctx, "Generate an auxiliary draft summary for the factual generated report. Do not add unsupported facts.", text, req.ReportID.String(), &llmclient.LLMOverrides{Mode: "report_enrichment", SystemPrompt: "Return only auxiliary draft prose. Do not modify facts, statuses, criteria, or evidence."})
}

// GenerateWithMode runs a mode-scoped prompt (forum_digest / evidence_summary /
// criteria_suggestion) through the same gRPC LLM path as report enrichment.
func (c *BackendGeneratedReportLLMClient) GenerateWithMode(ctx context.Context, mode string, description string, input string, refID string) (string, error) {
	if c == nil || c.client == nil {
		return "", fmt.Errorf("LLM client is not configured")
	}
	return c.client.ProcessTaskWithLLM(ctx, description, input, refID, &llmclient.LLMOverrides{Mode: mode})
}

func NewConveyorService(repo ConveyorRepository) ConveyorService {
	return &conveyorService{repo: repo}
}

func NewConveyorServiceWithReportLLM(repo ConveyorRepository, llm GeneratedReportLLMClient) ConveyorService {
	s := &conveyorService{repo: repo, reportLLM: llm}
	if mode, ok := llm.(ConveyorLLMModeClient); ok {
		s.llmMode = mode
	}
	return s
}

func (s *conveyorService) AuthorizeWorkItemAccess(ctx context.Context, actor ConveyorActor, taskID uuid.UUID) error {
	if actor.ActorID == uuid.Nil || taskID == uuid.Nil {
		return ErrPermissionDenied
	}
	allowed, err := s.repo.ActorCanAccessTask(ctx, actor.ActorID, taskID)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrPermissionDenied
	}
	return nil
}

func (s *conveyorService) ListAcceptanceCriteria(ctx context.Context, taskID uuid.UUID) ([]models.AcceptanceCriterion, error) {
	return s.repo.ListAcceptanceCriteria(ctx, taskID)
}

func (s *conveyorService) ListEvidence(ctx context.Context, taskID uuid.UUID) ([]models.Evidence, error) {
	return s.repo.ListEvidence(ctx, taskID)
}

func (s *conveyorService) ListEvents(ctx context.Context, taskID uuid.UUID) ([]models.ConveyorEvent, error) {
	return s.repo.ListEvents(ctx, taskID)
}

func (s *conveyorService) ListTaskLinks(ctx context.Context, taskID uuid.UUID) ([]models.TaskLink, error) {
	return s.repo.ListTaskLinks(ctx, taskID)
}

func (s *conveyorService) GenerateProjectReport(ctx context.Context, actor ConveyorActor, req GenerateProjectReportRequest) (*GeneratedReportResponse, error) {
	if req.ProjectID == uuid.Nil || req.PeriodStart.IsZero() || req.PeriodEnd.IsZero() || req.PeriodEnd.Before(req.PeriodStart) {
		return nil, fmt.Errorf("%w: invalid report period", ErrValidation)
	}
	effectiveEnd := reportPeriodEnd(req.PeriodEnd)
	var stored *models.GeneratedReport
	err := s.repo.WithTransaction(ctx, func(repo ConveyorRepository) error {
		sources, err := repo.ListProjectReportSources(ctx, req.ProjectID, req.PeriodStart, effectiveEnd)
		if err != nil {
			return err
		}
		report := s.buildGeneratedReport(actor, req, sources)
		if err := repo.CreateGeneratedReport(ctx, report); err != nil {
			return err
		}
		anchorID := firstReportAnchor(sources)
		if anchorID != uuid.Nil {
			if _, err := s.createEvent(ctx, repo, actor, anchorID, "generated_report.created", req.IdempotencyKey, map[string]any{"generated_report_id": report.ID, "project_id": req.ProjectID}); err != nil {
				return err
			}
		}
		stored = report
		return nil
	})
	if err != nil {
		return nil, err
	}
	response, err := GeneratedReportResponseFromModel(*stored)
	if err != nil {
		return nil, err
	}
	if req.IncludeLLM {
		return s.enrichGeneratedReport(ctx, actor, response)
	}
	return response, nil
}

func (s *conveyorService) GetGeneratedReport(ctx context.Context, reportID uuid.UUID) (*GeneratedReportResponse, error) {
	if reportID == uuid.Nil {
		return nil, fmt.Errorf("%w: report_id is required", ErrValidation)
	}
	report, err := s.repo.GetGeneratedReport(ctx, reportID)
	if err != nil {
		return nil, err
	}
	return GeneratedReportResponseFromModel(*report)
}

func (s *conveyorService) ExportGeneratedReportMarkdown(ctx context.Context, reportID uuid.UUID) (string, error) {
	report, err := s.GetGeneratedReport(ctx, reportID)
	if err != nil {
		return "", err
	}
	return generatedReportMarkdown(report), nil
}

func forumMessagesToText(messages []ForumDigestSourceMessage) string {
	var b strings.Builder
	for _, m := range messages {
		if !m.Accessible {
			continue
		}
		text := strings.TrimSpace(m.Text)
		if text == "" {
			continue
		}
		if strings.TrimSpace(m.Author) != "" {
			b.WriteString(strings.TrimSpace(m.Author))
			b.WriteString(": ")
		}
		b.WriteString(text)
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func taskInputText(task *models.Task) string {
	var b strings.Builder
	if task.Name != nil && strings.TrimSpace(*task.Name) != "" {
		b.WriteString(strings.TrimSpace(*task.Name))
		b.WriteString("\n")
	}
	if task.Description != nil && strings.TrimSpace(*task.Description) != "" {
		b.WriteString(strings.TrimSpace(*task.Description))
	}
	return strings.TrimSpace(b.String())
}

func evidenceToText(items []models.Evidence) string {
	var b strings.Builder
	for _, item := range items {
		if item.Revoked {
			continue
		}
		b.WriteString("- [")
		b.WriteString(item.Verdict)
		b.WriteString("/")
		b.WriteString(item.Type)
		b.WriteString("] ")
		if strings.TrimSpace(item.Title) != "" {
			b.WriteString(strings.TrimSpace(item.Title))
		}
		if strings.TrimSpace(item.URI) != "" {
			b.WriteString(" (")
			b.WriteString(strings.TrimSpace(item.URI))
			b.WriteString(")")
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

// SuggestCriteria returns LLM-proposed acceptance criteria for a task (CRIT-05:
// draft only, requires human confirmation — nothing is persisted here).
func (s *conveyorService) SuggestCriteria(ctx context.Context, actor ConveyorActor, taskID uuid.UUID) (string, error) {
	if taskID == uuid.Nil {
		return "", fmt.Errorf("%w: task_id is required", ErrValidation)
	}
	if s.llmMode == nil {
		return "", fmt.Errorf("%w: LLM is not configured", ErrValidation)
	}
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return "", err
	}
	input := taskInputText(task)
	if input == "" {
		return "", fmt.Errorf("%w: task has no description to suggest criteria from", ErrValidation)
	}
	return s.llmMode.GenerateWithMode(ctx, "criteria_suggestion", "Предложи проверяемые критерии приёмки для задачи.", input, taskID.String())
}

// SummarizeEvidence returns an LLM summary of a task's active evidence
// (mode=evidence_summary). Auxiliary draft text, not a source of truth.
func (s *conveyorService) SummarizeEvidence(ctx context.Context, actor ConveyorActor, taskID uuid.UUID) (string, error) {
	if taskID == uuid.Nil {
		return "", fmt.Errorf("%w: task_id is required", ErrValidation)
	}
	if s.llmMode == nil {
		return "", fmt.Errorf("%w: LLM is not configured", ErrValidation)
	}
	if _, err := s.repo.GetTask(ctx, taskID); err != nil {
		return "", err
	}
	evidence, err := s.repo.ListEvidence(ctx, taskID)
	if err != nil {
		return "", err
	}
	input := evidenceToText(evidence)
	if input == "" {
		return "", fmt.Errorf("%w: task has no active evidence to summarize", ErrValidation)
	}
	return s.llmMode.GenerateWithMode(ctx, "evidence_summary", "Сделай краткое резюме приложенных evidence по задаче.", input, taskID.String())
}

func (s *conveyorService) CreateForumDigest(ctx context.Context, actor ConveyorActor, req CreateForumDigestRequest) (*ForumDigestResponse, error) {
	if !validConversationSourceType(req.SourceType) || strings.TrimSpace(req.SourceID) == "" {
		return nil, fmt.Errorf("%w: source_type and source_id are required", ErrValidation)
	}
	if !req.PeriodStart.IsZero() && !req.PeriodEnd.IsZero() && req.PeriodEnd.Before(req.PeriodStart) {
		return nil, fmt.Errorf("%w: invalid digest period", ErrValidation)
	}
	// LLM-generated summary (mode=forum_digest) is computed before the DB
	// transaction so the network call never holds a transaction open. On error we
	// fall back to the caller-supplied Summary rather than failing the digest.
	summaryText := sanitizeForumText(req.Summary)
	if req.UseLLM && s.llmMode != nil {
		input := forumMessagesToText(sanitizedForumMessages(req.Messages))
		if strings.TrimSpace(input) != "" {
			if generated, err := s.llmMode.GenerateWithMode(ctx, "forum_digest", "Составь дайджест обсуждения на форуме.", input, strings.TrimSpace(req.SourceID)); err == nil {
				summaryText = sanitizeForumText(generated)
			}
		}
	}
	var response *ForumDigestResponse
	err := s.repo.WithTransaction(ctx, func(repo ConveyorRepository) error {
		now := time.Now()
		messages := sanitizedForumMessages(req.Messages)
		var periodStart *time.Time
		var periodEnd *time.Time
		if !req.PeriodStart.IsZero() {
			v := req.PeriodStart
			periodStart = &v
		}
		if !req.PeriodEnd.IsZero() {
			v := req.PeriodEnd
			periodEnd = &v
		}
		digest := &models.ForumDigest{
			ID:             uuid.New(),
			SourceType:     req.SourceType,
			SourceID:       strings.TrimSpace(req.SourceID),
			SourceTitle:    sanitizeForumText(req.SourceTitle),
			SourceLocator:  strings.TrimSpace(req.SourceLocator),
			PeriodStart:    periodStart,
			PeriodEnd:      periodEnd,
			Summary:        summaryText,
			Decisions:      normalizedJSON(req.Decisions),
			SourceMessages: mustMarshalJSON(messages),
			SourceMetadata: normalizedJSON(req.SourceMetadata),
			CreatedBy:      actor.ActorID,
			ActorType:      actorTypeOrDefault(actor.ActorType),
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		if err := repo.CreateForumDigest(ctx, digest); err != nil {
			return err
		}
		candidates := make([]models.ForumActionCandidate, 0, len(req.CandidateInputs))
		for _, input := range req.CandidateInputs {
			title := sanitizeForumText(input.Title)
			if strings.TrimSpace(title) == "" {
				continue
			}
			candidate := &models.ForumActionCandidate{ID: uuid.New(), DigestID: digest.ID, Title: title, Description: sanitizeForumText(input.Description), SourceLocator: strings.TrimSpace(input.SourceLocator), SourceMetadata: normalizedJSON(input.SourceMetadata), Status: models.ForumActionCandidateStatusPending, CreatedBy: actor.ActorID, CreatedAt: now, UpdatedAt: now}
			if candidate.SourceLocator == "" {
				candidate.SourceLocator = digest.SourceLocator
			}
			if err := repo.CreateForumActionCandidate(ctx, candidate); err != nil {
				return err
			}
			candidates = append(candidates, *candidate)
		}
		response = forumDigestResponse(*digest, candidates)
		return nil
	})
	if err != nil {
		return nil, err
	}
	// Realtime: панель дайджеста у других клиентов перезагрузится по SSE.
	publishGlobal(StreamEvent{Type: "forum.digest.created", WorkItemID: strings.TrimSpace(req.SourceID)})
	return response, nil
}

func (s *conveyorService) GetForumDigest(ctx context.Context, digestID uuid.UUID) (*ForumDigestResponse, error) {
	if digestID == uuid.Nil {
		return nil, fmt.Errorf("%w: digest_id is required", ErrValidation)
	}
	digest, err := s.repo.GetForumDigest(ctx, digestID)
	if err != nil {
		return nil, err
	}
	candidates, err := s.repo.ListForumActionCandidates(ctx, digest.ID)
	if err != nil {
		return nil, err
	}
	return forumDigestResponse(*digest, candidates), nil
}

func (s *conveyorService) ListForumDigests(ctx context.Context, sourceType string, sourceID string) ([]ForumDigestResponse, error) {
	if sourceType != "" && !validConversationSourceType(sourceType) {
		return nil, fmt.Errorf("%w: invalid source_type", ErrValidation)
	}
	digests, err := s.repo.ListForumDigests(ctx, sourceType, sourceID)
	if err != nil {
		return nil, err
	}
	out := make([]ForumDigestResponse, 0, len(digests))
	for _, digest := range digests {
		candidates, err := s.repo.ListForumActionCandidates(ctx, digest.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, *forumDigestResponse(digest, candidates))
	}
	return out, nil
}

func (s *conveyorService) ConfirmForumActionCandidate(ctx context.Context, actor ConveyorActor, candidateID uuid.UUID, req ConfirmForumActionCandidateRequest) (*ConveyorMutationResult, error) {
	if candidateID == uuid.Nil || req.StatusID == uuid.Nil {
		return nil, fmt.Errorf("%w: candidate_id and status_id are required", ErrValidation)
	}
	request := struct {
		CandidateID uuid.UUID                          `json:"candidate_id"`
		Request     ConfirmForumActionCandidateRequest `json:"request"`
	}{candidateID, req}
	return s.runIdempotent(ctx, actor, "forum_candidate.confirm", req.IdempotencyKey, request, func(repo ConveyorRepository) (*ConveyorMutationResult, error) {
		candidate, err := repo.GetForumActionCandidate(ctx, candidateID)
		if err != nil {
			return nil, err
		}
		if candidate.Status != models.ForumActionCandidateStatusPending || candidate.WorkItemID != nil {
			return nil, ErrConflict
		}
		if _, err := repo.GetStatus(ctx, req.StatusID); err != nil {
			return nil, err
		}
		assignee := actor.ActorID
		if req.AssignedTo != nil {
			assignee = *req.AssignedTo
		}
		now := time.Now()
		name := candidate.Title
		description := candidate.Description
		if candidate.SourceLocator != "" {
			description = strings.TrimSpace(description + "\n\nSource: " + candidate.SourceLocator)
		}
		task := &models.Task{ID: uuid.New(), StatusID: req.StatusID, Priority: req.Priority, Name: &name, Description: &description, CreatedBy: &actor.ActorID, AssignedTo: &assignee, Deleted: boolPtr(false), CreatedAt: &now, UpdatedAt: &now}
		if err := repo.CreateTask(ctx, task); err != nil {
			return nil, err
		}
		if candidate.SourceLocator != "" {
			evidence := &models.Evidence{ID: uuid.New(), TaskID: task.ID, Type: models.EvidenceTypeLink, Verdict: models.EvidenceVerdictInformational, URI: candidate.SourceLocator, Title: "Forum digest source", Metadata: candidate.SourceMetadata, CreatedBy: actor.ActorID, CreatedAt: now}
			if err := repo.CreateEvidence(ctx, evidence); err != nil {
				return nil, err
			}
		}
		candidate.Status = models.ForumActionCandidateStatusConfirmed
		candidate.WorkItemID = &task.ID
		candidate.ConfirmedBy = &actor.ActorID
		candidate.ConfirmedAt = &now
		candidate.UpdatedAt = now
		if err := repo.UpdateForumActionCandidate(ctx, candidate); err != nil {
			return nil, err
		}
		event, err := s.createEvent(ctx, repo, actor, task.ID, "forum_candidate.confirmed", req.IdempotencyKey, map[string]any{"candidate_id": candidate.ID, "digest_id": candidate.DigestID, "source_locator": candidate.SourceLocator})
		if err != nil {
			return nil, err
		}
		return &ConveyorMutationResult{EntityID: task.ID, EventID: event.ID}, nil
	})
}

func (s *conveyorService) RejectForumActionCandidate(ctx context.Context, actor ConveyorActor, candidateID uuid.UUID, req RejectForumActionCandidateRequest) (*ConveyorMutationResult, error) {
	if candidateID == uuid.Nil {
		return nil, fmt.Errorf("%w: candidate_id is required", ErrValidation)
	}
	var result *ConveyorMutationResult
	err := s.repo.WithTransaction(ctx, func(repo ConveyorRepository) error {
		candidate, err := repo.GetForumActionCandidate(ctx, candidateID)
		if err != nil {
			return err
		}
		if candidate.Status != models.ForumActionCandidateStatusPending {
			return ErrConflict
		}
		now := time.Now()
		candidate.Status = models.ForumActionCandidateStatusRejected
		candidate.RejectedBy = &actor.ActorID
		candidate.RejectedAt = &now
		candidate.RejectReason = sanitizeForumText(req.Reason)
		candidate.UpdatedAt = now
		if err := repo.UpdateForumActionCandidate(ctx, candidate); err != nil {
			return err
		}
		result = &ConveyorMutationResult{EntityID: candidate.ID}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *conveyorService) buildGeneratedReport(actor ConveyorActor, req GenerateProjectReportRequest, sources []ports.ProjectReportSource) *models.GeneratedReport {
	facts := []GeneratedReportItem{}
	risks := []GeneratedReportItem{}
	allEventIDs := []uuid.UUID{}
	allEvidenceIDs := []uuid.UUID{}
	for _, source := range sources {
		supporting := activeEvidenceByVerdict(source.Evidence, models.EvidenceVerdictSupports)
		contradicting := activeEvidenceByVerdict(source.Evidence, models.EvidenceVerdictContradicts)
		for _, event := range source.Events {
			allEventIDs = append(allEventIDs, event.ID)
		}
		for _, evidence := range source.Evidence {
			allEvidenceIDs = append(allEvidenceIDs, evidence.ID)
		}
		completedEvents := eventsByType(source.Events, "task.completed")
		for _, event := range completedEvents {
			sourceEventIDs := []uuid.UUID{event.ID}
			sourceEvidenceIDs := evidenceIDs(supporting)
			if len(supporting) == 0 {
				risks = append(risks, GeneratedReportItem{Kind: "completed_work_unverified", Text: "Completed work source exists but supporting evidence is missing or inactive", WorkItemID: source.Task.ID, SourceEventIDs: sourceEventIDs})
				continue
			}
			if len(contradicting) > 0 {
				risks = append(risks, GeneratedReportItem{Kind: "completed_work_contradicted", Text: "Completed work source has active contradicting evidence exists", WorkItemID: source.Task.ID, SourceEventIDs: sourceEventIDs, SourceEvidenceIDs: append(sourceEvidenceIDs, evidenceIDs(contradicting)...)})
				continue
			}
			facts = append(facts, GeneratedReportItem{Kind: "completed_work", Text: completedWorkText(source.Task), WorkItemID: source.Task.ID, SourceEventIDs: sourceEventIDs, SourceEvidenceIDs: sourceEvidenceIDs})
		}
	}
	if len(sources) == 0 {
		risks = append(risks, GeneratedReportItem{Kind: "empty_period", Text: "No report sources were found for the selected project and period"})
	}
	conclusions := []GeneratedReportItem{{Kind: "summary", Text: fmt.Sprintf("%d completed work facts, %d risks", len(facts), len(risks))}}
	now := time.Now()
	report := &models.GeneratedReport{ID: uuid.New(), ProjectID: req.ProjectID, PeriodStart: req.PeriodStart, PeriodEnd: req.PeriodEnd, CreatedBy: actor.ActorID, ActorType: actorTypeOrDefault(actor.ActorType), AgentID: actor.AgentID, CreatedAt: now, UpdatedAt: now}
	report.SourceEventIDs = mustMarshalJSON(uniqueUUIDs(allEventIDs))
	report.SourceEvidenceIDs = mustMarshalJSON(uniqueUUIDs(allEvidenceIDs))
	report.Facts = mustMarshalJSON(facts)
	report.Conclusions = mustMarshalJSON(conclusions)
	report.Risks = mustMarshalJSON(risks)
	return report
}

func (s *conveyorService) enrichGeneratedReport(ctx context.Context, actor ConveyorActor, report *GeneratedReportResponse) (*GeneratedReportResponse, error) {
	if s.reportLLM == nil {
		report.LLMError = "report LLM seam is not configured"
		return report, nil
	}
	anchorID := firstReportWorkItem(report)
	if anchorID == uuid.Nil {
		report.LLMError = "report LLM requires at least one source work item"
		return report, nil
	}
	created, err := s.RegisterAgentRun(ctx, actor, RegisterAgentRunRequest{WorkItemID: anchorID, Source: "rest", Harness: "reports_llm_ms", Status: models.AgentRunStatusRunning, Summary: "Generated report LLM enrichment started"})
	if err != nil {
		report.LLMError = err.Error()
		return report, nil
	}
	draft, err := s.reportLLM.EnrichGeneratedReport(ctx, GeneratedReportLLMRequest{ReportID: report.ID, ProjectID: report.ProjectID, PeriodStart: report.PeriodStart, PeriodEnd: report.PeriodEnd, Facts: report.Facts, Conclusions: report.Conclusions, Risks: report.Risks, SourceEventIDs: report.SourceEventIDs, SourceEvidenceIDs: report.SourceEvidenceIDs})
	status := models.AgentRunStatusSucceeded
	summary := "Generated report LLM enrichment succeeded"
	if err != nil {
		status = models.AgentRunStatusFailed
		summary = err.Error()
		report.LLMError = err.Error()
	} else {
		report.LLMDraft = GeneratedReportDraft{Text: draft, Auxiliary: true}
	}
	if persistErr := s.repo.UpdateGeneratedReportLLM(ctx, report.ID, mustMarshalJSON(report.LLMDraft), report.LLMError, time.Now()); persistErr != nil && report.LLMError == "" {
		report.LLMError = persistErr.Error()
	}
	_, updateErr := s.UpdateAgentRun(ctx, actor, anchorID, created.EntityID, UpdateAgentRunRequest{Status: status, Summary: summary})
	if updateErr != nil && report.LLMError == "" {
		report.LLMError = updateErr.Error()
	}
	return report, nil
}

func (s *conveyorService) ListAgentRuns(ctx context.Context, workItemID uuid.UUID) ([]models.AgentRun, error) {
	if workItemID == uuid.Nil {
		return nil, fmt.Errorf("%w: work_item_id is required", ErrValidation)
	}
	if _, err := s.repo.GetTask(ctx, workItemID); err != nil {
		return nil, err
	}
	return s.repo.ListAgentRuns(ctx, workItemID)
}

func (s *conveyorService) GetAgentRun(ctx context.Context, workItemID uuid.UUID, agentRunID uuid.UUID) (*models.AgentRun, error) {
	if workItemID == uuid.Nil || agentRunID == uuid.Nil {
		return nil, fmt.Errorf("%w: work_item_id and agent_run_id are required", ErrValidation)
	}
	run, err := s.repo.GetAgentRun(ctx, agentRunID)
	if err != nil {
		return nil, err
	}
	if run.WorkItemID != workItemID {
		return nil, ErrNotFound
	}
	return run, nil
}

func (s *conveyorService) CreateAcceptanceCriterion(ctx context.Context, actor ConveyorActor, req CreateAcceptanceCriterionRequest) (*ConveyorMutationResult, error) {
	if req.TaskID == uuid.Nil || strings.TrimSpace(req.Title) == "" {
		return nil, fmt.Errorf("%w: task_id and title are required", ErrValidation)
	}
	return s.runIdempotent(ctx, actor, "criterion.create", req.IdempotencyKey, req, func(repo ConveyorRepository) (*ConveyorMutationResult, error) {
		if _, err := repo.GetTask(ctx, req.TaskID); err != nil {
			return nil, err
		}
		now := time.Now()
		var specIDs json.RawMessage
		if len(req.SpecIDs) > 0 {
			specIDs = mustMarshalJSON(req.SpecIDs)
		}
		criterion := &models.AcceptanceCriterion{ID: uuid.New(), TaskID: req.TaskID, ACID: strings.TrimSpace(req.ACID), Title: strings.TrimSpace(req.Title), SpecIDs: specIDs, Required: req.Required, State: models.AcceptanceCriterionStateUnchecked, CreatedBy: actor.ActorID, CreatedAt: now, UpdatedAt: now}
		if err := repo.CreateAcceptanceCriterion(ctx, criterion); err != nil {
			return nil, err
		}
		event, err := s.createEvent(ctx, repo, actor, req.TaskID, "criterion.created", req.IdempotencyKey, map[string]any{"criterion_id": criterion.ID, "ac_id": criterion.ACID, "state": criterion.State})
		if err != nil {
			return nil, err
		}
		return &ConveyorMutationResult{EntityID: criterion.ID, EventID: event.ID}, nil
	})
}

func (s *conveyorService) UpdateAcceptanceCriterionState(ctx context.Context, actor ConveyorActor, taskID uuid.UUID, criterionID uuid.UUID, req UpdateCriterionStateRequest) (*ConveyorMutationResult, error) {
	if taskID == uuid.Nil || criterionID == uuid.Nil || !validCriterionState(req.State) {
		return nil, fmt.Errorf("%w: invalid criterion state", ErrValidation)
	}
	return s.runIdempotent(ctx, actor, "criterion.update_state", req.IdempotencyKey, struct {
		TaskID      uuid.UUID `json:"task_id"`
		CriterionID uuid.UUID `json:"criterion_id"`
		State       string    `json:"state"`
	}{taskID, criterionID, req.State}, func(repo ConveyorRepository) (*ConveyorMutationResult, error) {
		criterion, err := repo.GetAcceptanceCriterion(ctx, criterionID)
		if err != nil {
			return nil, err
		}
		if criterion.TaskID != taskID {
			return nil, ErrNotFound
		}
		previous := criterion.State
		now := time.Now()
		if err := repo.UpdateAcceptanceCriterionState(ctx, criterionID, req.State, now); err != nil {
			return nil, err
		}
		event, err := s.createEvent(ctx, repo, actor, criterion.TaskID, "criterion.state_changed", req.IdempotencyKey, map[string]string{"previous_state": previous, "new_state": req.State})
		if err != nil {
			return nil, err
		}
		return &ConveyorMutationResult{EntityID: criterionID, EventID: event.ID}, nil
	})
}

func (s *conveyorService) AttachEvidence(ctx context.Context, actor ConveyorActor, req AttachEvidenceRequest) (*ConveyorMutationResult, error) {
	if req.TaskID == uuid.Nil || !validEvidenceType(req.Type) || !validEvidenceVerdict(req.Verdict) {
		return nil, fmt.Errorf("%w: invalid evidence", ErrValidation)
	}
	if containsSecretLikeEvidence(req.URI, req.Title, req.Metadata) {
		return nil, fmt.Errorf("%w: evidence contains secret-like data", ErrValidation)
	}
	if req.Type == models.EvidenceTypeDeploymentLog && !hasApprovalDecision(req.ApprovalGranted, req.ApprovalToken) {
		return nil, ErrApprovalRequired
	}
	return s.runIdempotent(ctx, actor, "evidence.attach", req.IdempotencyKey, req, func(repo ConveyorRepository) (*ConveyorMutationResult, error) {
		if _, err := repo.GetTask(ctx, req.TaskID); err != nil {
			return nil, err
		}
		if req.CriterionID != nil {
			criterion, err := repo.GetAcceptanceCriterion(ctx, *req.CriterionID)
			if err != nil {
				return nil, err
			}
			if criterion.TaskID != req.TaskID {
				return nil, fmt.Errorf("%w: criterion does not belong to task", ErrValidation)
			}
		}
		now := time.Now()
		evidence := &models.Evidence{ID: uuid.New(), TaskID: req.TaskID, CriterionID: req.CriterionID, Type: req.Type, Verdict: req.Verdict, URI: req.URI, Title: req.Title, SHA256: evidenceChecksum(req), Metadata: []byte(req.Metadata), CreatedBy: actor.ActorID, CreatedAt: now}
		if err := repo.CreateEvidence(ctx, evidence); err != nil {
			return nil, err
		}
		event, err := s.createEvent(ctx, repo, actor, req.TaskID, "evidence.attached", req.IdempotencyKey, map[string]any{"evidence_id": evidence.ID, "verdict": evidence.Verdict, "type": evidence.Type, "criterion_id": req.CriterionID, "sha256": evidence.SHA256})
		if err != nil {
			return nil, err
		}
		return &ConveyorMutationResult{EntityID: evidence.ID, EventID: event.ID}, nil
	})
}

func (s *conveyorService) RevokeEvidence(ctx context.Context, actor ConveyorActor, taskID uuid.UUID, evidenceID uuid.UUID, req RevokeEvidenceRequest) (*ConveyorMutationResult, error) {
	if taskID == uuid.Nil || evidenceID == uuid.Nil {
		return nil, fmt.Errorf("%w: invalid evidence id", ErrValidation)
	}
	return s.runIdempotent(ctx, actor, "evidence.revoke", req.IdempotencyKey, struct {
		TaskID     uuid.UUID `json:"task_id"`
		EvidenceID uuid.UUID `json:"evidence_id"`
		Reason     string    `json:"reason"`
	}{taskID, evidenceID, req.Reason}, func(repo ConveyorRepository) (*ConveyorMutationResult, error) {
		evidence, err := repo.GetEvidence(ctx, evidenceID)
		if err != nil {
			return nil, err
		}
		if evidence.TaskID != taskID {
			return nil, ErrNotFound
		}
		now := time.Now()
		if err := repo.RevokeEvidence(ctx, evidenceID, actor.ActorID, req.Reason, now); err != nil {
			return nil, err
		}
		event, err := s.createEvent(ctx, repo, actor, evidence.TaskID, "evidence.revoked", req.IdempotencyKey, map[string]any{"evidence_id": evidenceID, "reason": req.Reason})
		if err != nil {
			return nil, err
		}
		return &ConveyorMutationResult{EntityID: evidenceID, EventID: event.ID}, nil
	})
}

func (s *conveyorService) LinkTasks(ctx context.Context, actor ConveyorActor, req LinkTasksRequest) (*ConveyorMutationResult, error) {
	if req.SourceTaskID == uuid.Nil || req.TargetTaskID == uuid.Nil || !validTaskLinkType(req.LinkType) {
		return nil, fmt.Errorf("%w: invalid task link", ErrValidation)
	}
	if req.SourceTaskID == req.TargetTaskID {
		return nil, fmt.Errorf("%w: task cannot link to itself", ErrValidation)
	}
	return s.runIdempotent(ctx, actor, "task.link", req.IdempotencyKey, req, func(repo ConveyorRepository) (*ConveyorMutationResult, error) {
		if _, err := repo.GetTask(ctx, req.SourceTaskID); err != nil {
			return nil, err
		}
		if _, err := repo.GetTask(ctx, req.TargetTaskID); err != nil {
			return nil, err
		}
		exists, err := repo.TaskLinkExists(ctx, req.SourceTaskID, req.TargetTaskID, req.LinkType)
		if err != nil {
			return nil, err
		}
		if exists {
			return nil, ErrConflict
		}
		link := &models.TaskLink{ID: uuid.New(), SourceTaskID: req.SourceTaskID, TargetTaskID: req.TargetTaskID, LinkType: req.LinkType, CreatedBy: actor.ActorID, CreatedAt: time.Now()}
		if err := repo.CreateTaskLink(ctx, link); err != nil {
			return nil, err
		}
		event, err := s.createEvent(ctx, repo, actor, req.SourceTaskID, "task.linked", req.IdempotencyKey, map[string]any{"link_id": link.ID, "target_task_id": req.TargetTaskID, "link_type": req.LinkType})
		if err != nil {
			return nil, err
		}
		return &ConveyorMutationResult{EntityID: link.ID, EventID: event.ID}, nil
	})
}

func (s *conveyorService) CloseTask(ctx context.Context, actor ConveyorActor, taskID uuid.UUID, req CloseTaskRequest) (*ConveyorMutationResult, error) {
	if taskID == uuid.Nil || req.ToStatusID == uuid.Nil {
		return nil, fmt.Errorf("%w: task_id and to_status_id are required", ErrValidation)
	}
	return s.runIdempotent(ctx, actor, "task.close", req.IdempotencyKey, struct {
		TaskID uuid.UUID        `json:"task_id"`
		Req    CloseTaskRequest `json:"request"`
	}{taskID, req}, func(repo ConveyorRepository) (*ConveyorMutationResult, error) {
		status, err := repo.GetStatus(ctx, req.ToStatusID)
		if err != nil {
			return nil, err
		}
		if status.IsOpen != nil && *status.IsOpen {
			return nil, fmt.Errorf("%w: status is not closed", ErrValidation)
		}
		if _, err := repo.GetTask(ctx, taskID); err != nil {
			return nil, err
		}
		if err := s.checkCloseGate(ctx, repo, taskID, req); err != nil {
			return nil, err
		}
		now := time.Now()
		if err := repo.UpdateTaskStatus(ctx, taskID, req.ToStatusID, now); err != nil {
			return nil, err
		}
		event, err := s.createEvent(ctx, repo, actor, taskID, "task.completed", req.IdempotencyKey, map[string]any{"to_status_id": req.ToStatusID})
		if err != nil {
			return nil, err
		}
		if err := s.emitDependencySignals(ctx, repo, actor, taskID, event, req); err != nil {
			return nil, err
		}
		return &ConveyorMutationResult{EntityID: taskID, EventID: event.ID}, nil
	})
}

func (s *conveyorService) CreateWorkOrder(ctx context.Context, actor ConveyorActor, req CreateWorkOrderRequest) (*ConveyorMutationResult, error) {
	if req.SourceTaskID == uuid.Nil || req.ProviderBoardID == uuid.Nil || req.ProviderStatusID == uuid.Nil || strings.TrimSpace(req.Goal) == "" {
		return nil, fmt.Errorf("%w: source task, provider board, provider status, and goal are required", ErrValidation)
	}
	return s.runIdempotent(ctx, actor, "work_order.create", req.IdempotencyKey, req, func(repo ConveyorRepository) (*ConveyorMutationResult, error) {
		if _, err := repo.GetTask(ctx, req.SourceTaskID); err != nil {
			return nil, err
		}
		status, err := repo.GetStatus(ctx, req.ProviderStatusID)
		if err != nil {
			return nil, err
		}
		if status.BoardID != req.ProviderBoardID {
			return nil, fmt.Errorf("%w: provider status does not belong to provider board", ErrValidation)
		}
		now := time.Now()
		workOrder := &models.WorkOrder{ID: uuid.New(), SourceTaskID: req.SourceTaskID, ProviderBoardID: req.ProviderBoardID, ProviderStatusID: req.ProviderStatusID, Goal: strings.TrimSpace(req.Goal), Inputs: normalizedJSON(req.Inputs), AcceptanceCriteria: normalizedJSON(req.AcceptanceCriteria), RequiredEvidenceMetadata: normalizedJSON(req.RequiredEvidenceMetadata), RequesterContext: normalizedJSON(req.RequesterContext), ProviderContext: normalizedJSON(req.ProviderContext), Status: models.WorkOrderStatusRequested, RequestedBy: actor.ActorID, CreatedAt: now, UpdatedAt: now}
		if err := repo.CreateWorkOrder(ctx, workOrder); err != nil {
			return nil, err
		}
		event, err := s.createEvent(ctx, repo, actor, req.SourceTaskID, "work_order.created", req.IdempotencyKey, map[string]any{"work_order_id": workOrder.ID, "provider_board_id": req.ProviderBoardID, "provider_status_id": req.ProviderStatusID})
		if err != nil {
			return nil, err
		}
		return &ConveyorMutationResult{EntityID: workOrder.ID, EventID: event.ID}, nil
	})
}

func (s *conveyorService) GetWorkOrder(ctx context.Context, workOrderID uuid.UUID) (*models.WorkOrder, error) {
	if workOrderID == uuid.Nil {
		return nil, fmt.Errorf("%w: work_order_id is required", ErrValidation)
	}
	return s.repo.GetWorkOrder(ctx, workOrderID)
}

// ListWorkOrders — наряды, созданные из задачи (source_task_id). Доступ проверяет
// контроллер через AuthorizeWorkItemAccess, как и для прочих list-эндпоинтов.
func (s *conveyorService) ListWorkOrders(ctx context.Context, taskID uuid.UUID) ([]models.WorkOrder, error) {
	if taskID == uuid.Nil {
		return nil, fmt.Errorf("%w: task_id is required", ErrValidation)
	}
	return s.repo.ListWorkOrdersByTask(ctx, taskID)
}

func (s *conveyorService) AcceptWorkOrder(ctx context.Context, actor ConveyorActor, workOrderID uuid.UUID, req AcceptWorkOrderRequest) (*ConveyorMutationResult, error) {
	if workOrderID == uuid.Nil {
		return nil, fmt.Errorf("%w: work_order_id is required", ErrValidation)
	}
	return s.runIdempotent(ctx, actor, "work_order.accept", req.IdempotencyKey, struct {
		WorkOrderID uuid.UUID              `json:"work_order_id"`
		Request     AcceptWorkOrderRequest `json:"request"`
	}{workOrderID, req}, func(repo ConveyorRepository) (*ConveyorMutationResult, error) {
		workOrder, err := repo.GetWorkOrder(ctx, workOrderID)
		if err != nil {
			return nil, err
		}
		if workOrder.Status != models.WorkOrderStatusRequested {
			return nil, ErrConflict
		}
		name := strings.TrimSpace(req.TargetName)
		if name == "" {
			name = strings.TrimSpace(workOrder.Goal)
		}
		now := time.Now()
		closed := false
		task := &models.Task{ID: uuid.New(), Name: &name, CreatedBy: &actor.ActorID, StatusID: workOrder.ProviderStatusID, Deleted: &closed, CreatedAt: &now, UpdatedAt: &now}
		if err := repo.CreateTask(ctx, task); err != nil {
			return nil, err
		}
		link := &models.TaskLink{ID: uuid.New(), SourceTaskID: workOrder.SourceTaskID, TargetTaskID: task.ID, LinkType: models.TaskLinkTypeBlocks, CreatedBy: actor.ActorID, CreatedAt: now}
		if err := repo.CreateTaskLink(ctx, link); err != nil {
			return nil, err
		}
		workOrder.Status = models.WorkOrderStatusAccepted
		workOrder.TargetTaskID = &task.ID
		workOrder.ProviderActorID = &actor.ActorID
		workOrder.AcceptedAt = &now
		workOrder.UpdatedAt = now
		if err := repo.UpdateWorkOrder(ctx, workOrder); err != nil {
			return nil, err
		}
		event, err := s.createEvent(ctx, repo, actor, workOrder.SourceTaskID, "work_order.accepted", req.IdempotencyKey, map[string]any{"work_order_id": workOrder.ID, "target_task_id": task.ID, "link_id": link.ID})
		if err != nil {
			return nil, err
		}
		return &ConveyorMutationResult{EntityID: task.ID, EventID: event.ID}, nil
	})
}

func (s *conveyorService) RejectWorkOrder(ctx context.Context, actor ConveyorActor, workOrderID uuid.UUID, req RejectWorkOrderRequest) (*ConveyorMutationResult, error) {
	return s.transitionWorkOrder(ctx, actor, workOrderID, req.IdempotencyKey, models.WorkOrderStatusRejected, "work_order.rejected", strings.TrimSpace(req.Reason), func(workOrder *models.WorkOrder, reason string) { workOrder.RejectReason = reason })
}

func (s *conveyorService) CompleteWorkOrder(ctx context.Context, actor ConveyorActor, workOrderID uuid.UUID, req CompleteWorkOrderRequest) (*ConveyorMutationResult, error) {
	if workOrderID == uuid.Nil {
		return nil, fmt.Errorf("%w: work_order_id is required", ErrValidation)
	}
	if req.ResultEvidenceID == nil && strings.TrimSpace(req.EvidenceWaiver) == "" {
		return nil, fmt.Errorf("%w: result evidence or waiver is required", ErrValidation)
	}
	return s.runIdempotent(ctx, actor, "work_order.complete", req.IdempotencyKey, struct {
		WorkOrderID uuid.UUID                `json:"work_order_id"`
		Request     CompleteWorkOrderRequest `json:"request"`
	}{workOrderID, req}, func(repo ConveyorRepository) (*ConveyorMutationResult, error) {
		workOrder, err := repo.GetWorkOrder(ctx, workOrderID)
		if err != nil {
			return nil, err
		}
		if workOrder.Status == models.WorkOrderStatusCompleted || workOrder.Status == models.WorkOrderStatusRejected || workOrder.Status == models.WorkOrderStatusCanceled || workOrder.Status == models.WorkOrderStatusFailed {
			return nil, ErrConflict
		}
		if req.ResultEvidenceID != nil {
			evidence, err := repo.GetEvidence(ctx, *req.ResultEvidenceID)
			if err != nil {
				return nil, err
			}
			if workOrder.TargetTaskID != nil && evidence.TaskID != *workOrder.TargetTaskID {
				return nil, fmt.Errorf("%w: result evidence does not belong to target task", ErrValidation)
			}
		}
		now := time.Now()
		workOrder.Status = models.WorkOrderStatusCompleted
		workOrder.ResultEvidenceID = req.ResultEvidenceID
		workOrder.EvidenceWaiver = strings.TrimSpace(req.EvidenceWaiver)
		workOrder.CompletedAt = &now
		workOrder.UpdatedAt = now
		if err := repo.UpdateWorkOrder(ctx, workOrder); err != nil {
			return nil, err
		}
		payload := map[string]any{"work_order_id": workOrder.ID, "evidence_waiver": workOrder.EvidenceWaiver}
		if req.ResultEvidenceID != nil {
			payload["result_evidence_id"] = req.ResultEvidenceID.String()
		}
		event, err := s.createEvent(ctx, repo, actor, workOrder.SourceTaskID, "work_order.completed", req.IdempotencyKey, payload)
		if err != nil {
			return nil, err
		}
		return &ConveyorMutationResult{EntityID: workOrder.ID, EventID: event.ID}, nil
	})
}

func (s *conveyorService) CancelWorkOrder(ctx context.Context, actor ConveyorActor, workOrderID uuid.UUID, req CancelWorkOrderRequest) (*ConveyorMutationResult, error) {
	return s.transitionWorkOrder(ctx, actor, workOrderID, req.IdempotencyKey, models.WorkOrderStatusCanceled, "work_order.canceled", strings.TrimSpace(req.Reason), func(workOrder *models.WorkOrder, reason string) { workOrder.CancelReason = reason })
}

func (s *conveyorService) FailWorkOrder(ctx context.Context, actor ConveyorActor, workOrderID uuid.UUID, req FailWorkOrderRequest) (*ConveyorMutationResult, error) {
	return s.transitionWorkOrder(ctx, actor, workOrderID, req.IdempotencyKey, models.WorkOrderStatusFailed, "work_order.failed", strings.TrimSpace(req.Reason), func(workOrder *models.WorkOrder, reason string) { workOrder.FailureReason = reason })
}

func (s *conveyorService) transitionWorkOrder(ctx context.Context, actor ConveyorActor, workOrderID uuid.UUID, key string, next string, eventType string, reason string, apply func(*models.WorkOrder, string)) (*ConveyorMutationResult, error) {
	if workOrderID == uuid.Nil || reason == "" {
		return nil, fmt.Errorf("%w: work_order_id and reason are required", ErrValidation)
	}
	return s.runIdempotent(ctx, actor, eventType, key, struct {
		WorkOrderID uuid.UUID `json:"work_order_id"`
		Reason      string    `json:"reason"`
	}{workOrderID, reason}, func(repo ConveyorRepository) (*ConveyorMutationResult, error) {
		workOrder, err := repo.GetWorkOrder(ctx, workOrderID)
		if err != nil {
			return nil, err
		}
		if workOrder.Status == models.WorkOrderStatusCompleted || workOrder.Status == models.WorkOrderStatusRejected || workOrder.Status == models.WorkOrderStatusCanceled || workOrder.Status == models.WorkOrderStatusFailed {
			return nil, ErrConflict
		}
		workOrder.Status = next
		workOrder.UpdatedAt = time.Now()
		apply(workOrder, reason)
		if err := repo.UpdateWorkOrder(ctx, workOrder); err != nil {
			return nil, err
		}
		event, err := s.createEvent(ctx, repo, actor, workOrder.SourceTaskID, eventType, key, map[string]any{"work_order_id": workOrder.ID, "reason": reason})
		if err != nil {
			return nil, err
		}
		return &ConveyorMutationResult{EntityID: workOrder.ID, EventID: event.ID}, nil
	})
}

func (s *conveyorService) emitDependencySignals(ctx context.Context, repo ConveyorRepository, actor ConveyorActor, upstreamID uuid.UUID, completedEvent *models.ConveyorEvent, req CloseTaskRequest) error {
	links, err := repo.ListDownstreamDependencies(ctx, upstreamID)
	if err != nil {
		return err
	}
	if len(links) == 0 {
		return nil
	}
	upstream, err := repo.GetTask(ctx, upstreamID)
	if err != nil {
		return err
	}
	supporting, err := activeEvidenceIDs(ctx, repo, upstreamID, models.EvidenceVerdictSupports)
	if err != nil {
		return err
	}
	for _, link := range links {
		downstreamID := downstreamTaskID(upstreamID, link)
		if downstreamID == uuid.Nil {
			continue
		}
		downstream, err := repo.GetTask(ctx, downstreamID)
		if err != nil {
			return err
		}
		payload := map[string]any{
			"upstream_work_item_id":   upstream.ID.String(),
			"downstream_work_item_id": downstream.ID.String(),
			"link_id":                 link.ID.String(),
			"link_type":               link.LinkType,
			"completed_event_id":      completedEvent.ID.String(),
			"supporting_evidence_ids": uuidStrings(supporting),
			"auto_ready_requested":    req.AllowDependencyAutoReady,
		}
		if _, err := s.createEvent(ctx, repo, actor, downstreamID, "dependency.signal", "", payload); err != nil {
			return err
		}
		if ready, readyStatusID, reason, err := s.shouldAutoReady(ctx, repo, downstream); err != nil {
			return err
		} else if req.AllowDependencyAutoReady && ready {
			if err := repo.UpdateTaskStatus(ctx, downstreamID, readyStatusID, time.Now()); err != nil {
				return err
			}
			if _, err := s.createEvent(ctx, repo, actor, downstreamID, "dependency.auto_ready", "", map[string]any{"upstream_work_item_id": upstream.ID.String(), "ready_status_id": readyStatusID.String(), "supporting_evidence_ids": uuidStrings(supporting)}); err != nil {
				return err
			}
		} else if req.AllowDependencyAutoReady && reason != "" {
			if _, err := s.createEvent(ctx, repo, actor, downstreamID, "dependency.auto_ready_skipped", "", map[string]any{"upstream_work_item_id": upstream.ID.String(), "reason": reason}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *conveyorService) shouldAutoReady(ctx context.Context, repo ConveyorRepository, downstream *models.Task) (bool, uuid.UUID, string, error) {
	currentStatus, err := repo.GetStatus(ctx, downstream.StatusID)
	if err != nil {
		return false, uuid.Nil, "", err
	}
	if currentStatus.Name == nil || !strings.EqualFold(strings.TrimSpace(*currentStatus.Name), "blocked") {
		return false, uuid.Nil, "downstream_not_blocked", nil
	}
	readyStatus, err := repo.FindStatusByBoardName(ctx, currentStatus.BoardID, "ready")
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return false, uuid.Nil, "ready_status_missing", nil
		}
		return false, uuid.Nil, "", err
	}
	upstreamLinks, err := repo.ListUpstreamDependencies(ctx, downstream.ID)
	if err != nil {
		return false, uuid.Nil, "", err
	}
	for _, link := range upstreamLinks {
		upstreamID := upstreamTaskID(downstream.ID, link)
		if upstreamID == uuid.Nil {
			continue
		}
		upstream, err := repo.GetTask(ctx, upstreamID)
		if err != nil {
			return false, uuid.Nil, "", err
		}
		status, err := repo.GetStatus(ctx, upstream.StatusID)
		if err != nil {
			return false, uuid.Nil, "", err
		}
		if status.IsOpen == nil || *status.IsOpen {
			return false, uuid.Nil, "blocker_open", nil
		}
	}
	contradicts, err := activeEvidenceIDs(ctx, repo, downstream.ID, models.EvidenceVerdictContradicts)
	if err != nil {
		return false, uuid.Nil, "", err
	}
	if len(contradicts) > 0 {
		return false, uuid.Nil, "contradicting_evidence", nil
	}
	return true, readyStatus.ID, "", nil
}

func downstreamTaskID(upstreamID uuid.UUID, link models.TaskLink) uuid.UUID {
	if link.LinkType == models.TaskLinkTypeBlocks && link.SourceTaskID == upstreamID {
		return link.TargetTaskID
	}
	if link.LinkType == models.TaskLinkTypeBlockedBy && link.TargetTaskID == upstreamID {
		return link.SourceTaskID
	}
	return uuid.Nil
}

func upstreamTaskID(downstreamID uuid.UUID, link models.TaskLink) uuid.UUID {
	if link.LinkType == models.TaskLinkTypeBlocks && link.TargetTaskID == downstreamID {
		return link.SourceTaskID
	}
	if link.LinkType == models.TaskLinkTypeBlockedBy && link.SourceTaskID == downstreamID {
		return link.TargetTaskID
	}
	return uuid.Nil
}

func activeEvidenceIDs(ctx context.Context, repo ConveyorRepository, taskID uuid.UUID, verdict string) ([]uuid.UUID, error) {
	items, err := repo.ListEvidence(ctx, taskID)
	if err != nil {
		return nil, err
	}
	out := []uuid.UUID{}
	for _, item := range items {
		if !item.Revoked && item.Verdict == verdict {
			out = append(out, item.ID)
		}
	}
	return out, nil
}

func uuidStrings(values []uuid.UUID) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, value.String())
	}
	return out
}

func (s *conveyorService) RegisterAgentRun(ctx context.Context, actor ConveyorActor, req RegisterAgentRunRequest) (*ConveyorMutationResult, error) {
	if req.WorkItemID == uuid.Nil || strings.TrimSpace(req.Source) == "" || strings.TrimSpace(req.Harness) == "" {
		return nil, fmt.Errorf("%w: work_item_id, source, and harness are required", ErrValidation)
	}
	status := strings.TrimSpace(req.Status)
	if status == "" {
		status = models.AgentRunStatusQueued
	}
	if !validAgentRunStatus(status) {
		return nil, fmt.Errorf("%w: invalid agent run status", ErrValidation)
	}
	request := req
	request.Status = status
	return s.runIdempotent(ctx, actor, "agent_run.register", req.IdempotencyKey, request, func(repo ConveyorRepository) (*ConveyorMutationResult, error) {
		if _, err := repo.GetTask(ctx, req.WorkItemID); err != nil {
			return nil, err
		}
		now := time.Now()
		run := &models.AgentRun{
			ID:               uuid.New(),
			WorkItemID:       req.WorkItemID,
			Source:           strings.TrimSpace(req.Source),
			Harness:          strings.TrimSpace(req.Harness),
			Status:           status,
			Summary:          strings.TrimSpace(req.Summary),
			LogURI:           strings.TrimSpace(req.LogURI),
			WorkspaceURI:     strings.TrimSpace(req.WorkspaceURI),
			Metadata:         normalizedJSON(req.Metadata),
			CreatedBy:        actor.ActorID,
			ActorType:        actorTypeOrDefault(actor.ActorType),
			AgentID:          actor.AgentID,
			OnBehalfOfUserID: actor.OnBehalfOfUserID,
			APITokenID:       actor.APITokenID,
			CreatedAt:        now,
			UpdatedAt:        now,
		}
		if status == models.AgentRunStatusRunning {
			run.StartedAt = &now
			run.HeartbeatAt = &now
		}
		if isTerminalAgentRunStatus(status) {
			run.FinishedAt = &now
		}
		if err := repo.CreateAgentRun(ctx, run); err != nil {
			return nil, err
		}
		event, err := s.createEvent(ctx, repo, actor, req.WorkItemID, "agent_run.registered", req.IdempotencyKey, map[string]any{"agent_run_id": run.ID, "source": run.Source, "harness": run.Harness, "new_status": run.Status})
		if err != nil {
			return nil, err
		}
		return &ConveyorMutationResult{EntityID: run.ID, EventID: event.ID}, nil
	})
}

func (s *conveyorService) UpdateAgentRun(ctx context.Context, actor ConveyorActor, workItemID uuid.UUID, agentRunID uuid.UUID, req UpdateAgentRunRequest) (*ConveyorMutationResult, error) {
	if workItemID == uuid.Nil || agentRunID == uuid.Nil {
		return nil, fmt.Errorf("%w: work_item_id and agent_run_id are required", ErrValidation)
	}
	return s.runIdempotent(ctx, actor, "agent_run.update", req.IdempotencyKey, struct {
		WorkItemID uuid.UUID             `json:"work_item_id"`
		AgentRunID uuid.UUID             `json:"agent_run_id"`
		Request    UpdateAgentRunRequest `json:"request"`
	}{workItemID, agentRunID, req}, func(repo ConveyorRepository) (*ConveyorMutationResult, error) {
		run, err := repo.GetAgentRun(ctx, agentRunID)
		if err != nil {
			return nil, err
		}
		if run.WorkItemID != workItemID {
			return nil, ErrNotFound
		}
		previous := run.Status
		newStatus := strings.TrimSpace(req.Status)
		if newStatus == "" {
			newStatus = previous
		}
		if !validAgentRunStatus(newStatus) || !validAgentRunTransition(previous, newStatus) {
			return nil, fmt.Errorf("%w: invalid agent run transition", ErrValidation)
		}
		now := time.Now()
		run.Status = newStatus
		run.Summary = strings.TrimSpace(req.Summary)
		run.LogURI = strings.TrimSpace(req.LogURI)
		run.WorkspaceURI = strings.TrimSpace(req.WorkspaceURI)
		if req.Metadata != nil {
			run.Metadata = normalizedJSON(req.Metadata)
		}
		if newStatus == models.AgentRunStatusRunning && run.StartedAt == nil {
			run.StartedAt = &now
		}
		if newStatus == models.AgentRunStatusRunning {
			run.HeartbeatAt = &now
		}
		if isTerminalAgentRunStatus(newStatus) || newStatus == models.AgentRunStatusNeedsHuman {
			run.FinishedAt = &now
		}
		run.UpdatedAt = now
		if err := repo.UpdateAgentRun(ctx, run); err != nil {
			return nil, err
		}
		eventType := "agent_run.updated"
		payload := map[string]any{"agent_run_id": run.ID}
		if previous != newStatus {
			eventType = "agent_run.status_changed"
			payload["previous_status"] = previous
			payload["new_status"] = newStatus
		} else {
			payload["status"] = newStatus
		}
		event, err := s.createEvent(ctx, repo, actor, workItemID, eventType, req.IdempotencyKey, payload)
		if err != nil {
			return nil, err
		}
		return &ConveyorMutationResult{EntityID: run.ID, EventID: event.ID}, nil
	})
}

func (s *conveyorService) HeartbeatAgentRun(ctx context.Context, actor ConveyorActor, workItemID uuid.UUID, agentRunID uuid.UUID) (*ConveyorMutationResult, error) {
	if workItemID == uuid.Nil || agentRunID == uuid.Nil {
		return nil, fmt.Errorf("%w: work_item_id and agent_run_id are required", ErrValidation)
	}
	var result *ConveyorMutationResult
	err := s.repo.WithTransaction(ctx, func(repo ConveyorRepository) error {
		run, err := repo.GetAgentRun(ctx, agentRunID)
		if err != nil {
			return err
		}
		if run.WorkItemID != workItemID {
			return ErrNotFound
		}
		now := time.Now()
		run.HeartbeatAt = &now
		run.UpdatedAt = now
		if err := repo.UpdateAgentRun(ctx, run); err != nil {
			return err
		}
		event, err := s.createEvent(ctx, repo, actor, workItemID, "agent_run.heartbeat", "", map[string]any{"agent_run_id": run.ID, "status": run.Status})
		if err != nil {
			return err
		}
		result = &ConveyorMutationResult{EntityID: run.ID, EventID: event.ID}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *conveyorService) MarkStaleAgentRunsFailed(ctx context.Context, actor ConveyorActor, cutoff time.Time) ([]uuid.UUID, error) {
	if cutoff.IsZero() {
		return nil, fmt.Errorf("%w: cutoff is required", ErrValidation)
	}
	marked := []uuid.UUID{}
	err := s.repo.WithTransaction(ctx, func(repo ConveyorRepository) error {
		runs, err := repo.ListStaleAgentRuns(ctx, cutoff)
		if err != nil {
			return err
		}
		for i := range runs {
			run := runs[i]
			if isTerminalAgentRunStatus(run.Status) {
				continue
			}
			previous := run.Status
			now := time.Now()
			run.Status = models.AgentRunStatusFailed
			run.Summary = strings.TrimSpace(run.Summary)
			if run.Summary == "" {
				run.Summary = "Agent run marked failed after stale heartbeat"
			}
			run.FinishedAt = &now
			run.UpdatedAt = now
			if err := repo.UpdateAgentRun(ctx, &run); err != nil {
				return err
			}
			if _, err := s.createEvent(ctx, repo, actor, run.WorkItemID, "agent_run.status_changed", "", map[string]any{"agent_run_id": run.ID, "previous_status": previous, "new_status": models.AgentRunStatusFailed, "reason": "stale"}); err != nil {
				return err
			}
			marked = append(marked, run.ID)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return marked, nil
}

func (s *conveyorService) CreateWaiver(ctx context.Context, actor ConveyorActor, req CreateWaiverRequest) (*ConveyorMutationResult, error) {
	scope := strings.TrimSpace(req.Scope)
	if scope == "" {
		scope = models.WaiverScopeTask
	}
	if !validWaiverScope(scope) || req.WorkItemID == uuid.Nil || strings.TrimSpace(req.Reason) == "" {
		return nil, fmt.Errorf("%w: scope, work_item_id and reason are required", ErrValidation)
	}
	if scope == models.WaiverScopeCriterion && req.CriterionID == nil {
		return nil, fmt.Errorf("%w: criterion_id is required for a criterion-scoped waiver", ErrValidation)
	}
	return s.runIdempotent(ctx, actor, "waiver.create", req.IdempotencyKey, req, func(repo ConveyorRepository) (*ConveyorMutationResult, error) {
		if _, err := repo.GetTask(ctx, req.WorkItemID); err != nil {
			return nil, err
		}
		if req.CriterionID != nil {
			criterion, err := repo.GetAcceptanceCriterion(ctx, *req.CriterionID)
			if err != nil {
				return nil, err
			}
			if criterion.TaskID != req.WorkItemID {
				return nil, fmt.Errorf("%w: criterion does not belong to task", ErrValidation)
			}
		}
		var approvedBy *uuid.UUID
		if req.ApprovalRequestID != nil {
			approval, err := repo.GetApprovalRequest(ctx, *req.ApprovalRequestID)
			if err != nil {
				return nil, err
			}
			if approval.Status != models.ApprovalStatusGranted {
				return nil, fmt.Errorf("%w: linked approval is not granted", ErrValidation)
			}
			approvedBy = approval.DecidedBy
		}
		waiver := &models.Waiver{ID: uuid.New(), Scope: scope, WorkItemID: req.WorkItemID, CriterionID: req.CriterionID, Reason: strings.TrimSpace(req.Reason), ApprovalRequestID: req.ApprovalRequestID, CreatedBy: actor.ActorID, ApprovedBy: approvedBy, ExpiresAt: req.ExpiresAt, CreatedAt: time.Now()}
		if err := repo.CreateWaiver(ctx, waiver); err != nil {
			return nil, err
		}
		event, err := s.createEvent(ctx, repo, actor, req.WorkItemID, "waiver.created", req.IdempotencyKey, map[string]any{"waiver_id": waiver.ID, "scope": waiver.Scope, "criterion_id": waiver.CriterionID})
		if err != nil {
			return nil, err
		}
		return &ConveyorMutationResult{EntityID: waiver.ID, EventID: event.ID}, nil
	})
}

func (s *conveyorService) GetWaiver(ctx context.Context, id uuid.UUID) (*models.Waiver, error) {
	if id == uuid.Nil {
		return nil, fmt.Errorf("%w: waiver id is required", ErrValidation)
	}
	return s.repo.GetWaiver(ctx, id)
}

func (s *conveyorService) RequestApproval(ctx context.Context, actor ConveyorActor, req RequestApprovalRequest) (*ConveyorMutationResult, error) {
	action := strings.TrimSpace(req.Action)
	if !validApprovalAction(action) {
		return nil, fmt.Errorf("%w: invalid approval action", ErrValidation)
	}
	return s.runIdempotent(ctx, actor, "approval.request", req.IdempotencyKey, req, func(repo ConveyorRepository) (*ConveyorMutationResult, error) {
		if req.WorkItemID != nil {
			if _, err := repo.GetTask(ctx, *req.WorkItemID); err != nil {
				return nil, err
			}
		}
		risk := strings.TrimSpace(req.RiskLevel)
		if risk == "" {
			risk = "medium"
		}
		approval := &models.ApprovalRequest{ID: uuid.New(), WorkItemID: req.WorkItemID, Action: action, RiskLevel: risk, Status: models.ApprovalStatusPending, Reason: strings.TrimSpace(req.Reason), Resource: normalizedJSON(req.Resource), RequestedBy: actor.ActorID, ActorType: actorTypeOrDefault(actor.ActorType), AgentID: actor.AgentID, OnBehalfOfUserID: actor.OnBehalfOfUserID, APITokenID: actor.APITokenID, ExpiresAt: req.ExpiresAt, CreatedAt: time.Now()}
		if err := repo.CreateApprovalRequest(ctx, approval); err != nil {
			return nil, err
		}
		event, err := s.createEvent(ctx, repo, actor, approvalEventWorkItem(req.WorkItemID), "approval.requested", req.IdempotencyKey, map[string]any{"approval_id": approval.ID, "action": approval.Action, "risk_level": approval.RiskLevel})
		if err != nil {
			return nil, err
		}
		return &ConveyorMutationResult{EntityID: approval.ID, EventID: event.ID}, nil
	})
}

func (s *conveyorService) GrantApproval(ctx context.Context, actor ConveyorActor, approvalID uuid.UUID, req DecideApprovalRequest) (*ConveyorMutationResult, error) {
	return s.decideApproval(ctx, actor, approvalID, req, models.ApprovalStatusGranted, "approval.granted")
}

func (s *conveyorService) DenyApproval(ctx context.Context, actor ConveyorActor, approvalID uuid.UUID, req DecideApprovalRequest) (*ConveyorMutationResult, error) {
	return s.decideApproval(ctx, actor, approvalID, req, models.ApprovalStatusDenied, "approval.denied")
}

func (s *conveyorService) decideApproval(ctx context.Context, actor ConveyorActor, approvalID uuid.UUID, req DecideApprovalRequest, status string, eventType string) (*ConveyorMutationResult, error) {
	if approvalID == uuid.Nil {
		return nil, fmt.Errorf("%w: approval id is required", ErrValidation)
	}
	return s.runIdempotent(ctx, actor, "approval.decide", req.IdempotencyKey, struct {
		ApprovalID uuid.UUID `json:"approval_id"`
		Status     string    `json:"status"`
		Reason     string    `json:"reason"`
	}{approvalID, status, req.Reason}, func(repo ConveyorRepository) (*ConveyorMutationResult, error) {
		approval, err := repo.GetApprovalRequest(ctx, approvalID)
		if err != nil {
			return nil, err
		}
		if approval.Status != models.ApprovalStatusPending {
			return nil, fmt.Errorf("%w: approval already decided", ErrConflict)
		}
		now := time.Now()
		decidedBy := actor.ActorID
		approval.Status = status
		approval.DecidedBy = &decidedBy
		approval.DecisionReason = strings.TrimSpace(req.Reason)
		approval.DecidedAt = &now
		if err := repo.UpdateApprovalRequest(ctx, approval); err != nil {
			return nil, err
		}
		event, err := s.createEvent(ctx, repo, actor, approvalEventWorkItem(approval.WorkItemID), eventType, req.IdempotencyKey, map[string]any{"approval_id": approval.ID, "status": status})
		if err != nil {
			return nil, err
		}
		return &ConveyorMutationResult{EntityID: approval.ID, EventID: event.ID}, nil
	})
}

func (s *conveyorService) GetApprovalRequest(ctx context.Context, id uuid.UUID) (*models.ApprovalRequest, error) {
	if id == uuid.Nil {
		return nil, fmt.Errorf("%w: approval id is required", ErrValidation)
	}
	return s.repo.GetApprovalRequest(ctx, id)
}

func (s *conveyorService) ListApprovalRequests(ctx context.Context, workItemID uuid.UUID) ([]models.ApprovalRequest, error) {
	return s.repo.ListApprovalRequests(ctx, workItemID)
}

func (s *conveyorService) ListPendingApprovals(ctx context.Context, limit int) ([]models.ApprovalRequest, error) {
	return s.repo.ListPendingApprovals(ctx, limit)
}

func approvalEventWorkItem(id *uuid.UUID) uuid.UUID {
	if id != nil {
		return *id
	}
	return uuid.Nil
}

func validWaiverScope(value string) bool {
	return value == models.WaiverScopeTask || value == models.WaiverScopeCriterion
}

func validApprovalAction(value string) bool {
	switch value {
	case models.ApprovalActionCloseWithWaiver, models.ApprovalActionProductionDeploy, models.ApprovalActionSecretAccess, models.ApprovalActionRoleChange, models.ApprovalActionEvidenceRevoke, models.ApprovalActionWorkOrderAccept, models.ApprovalActionOther:
		return true
	default:
		return false
	}
}

// evidenceChecksum returns the caller-supplied sha256 when present, otherwise a
// deterministic fingerprint of the evidence payload so the record is tamper-evident.
func evidenceChecksum(req AttachEvidenceRequest) string {
	if trimmed := strings.TrimSpace(req.SHA256); trimmed != "" {
		return trimmed
	}
	fingerprint := strings.Join([]string{req.Type, req.Verdict, req.URI, req.Title, string(req.Metadata)}, "\x1f")
	sum := sha256.Sum256([]byte(fingerprint))
	return hex.EncodeToString(sum[:])
}

func (s *conveyorService) checkCloseGate(ctx context.Context, repo ConveyorRepository, taskID uuid.UUID, req CloseTaskRequest) error {
	approved := s.approvalSatisfied(ctx, repo, req.ApprovalToken, req.ApprovalGranted)
	// Risky waiver-based close requires an approval decision (kept before waiver
	// resolution so error ordering is stable for callers).
	if req.TaskWaiverID != nil && mediumOrHigherRisk(req.RiskLevel) && !approved {
		return ErrApprovalRequired
	}
	// TaskWaiverID must reference a real, applicable, unexpired waiver record.
	hasWaiver := false
	if req.TaskWaiverID != nil {
		waiver, err := repo.GetWaiver(ctx, *req.TaskWaiverID)
		if err != nil {
			return err
		}
		if waiver.WorkItemID != taskID || waiver.Scope != models.WaiverScopeTask {
			return fmt.Errorf("%w: waiver does not apply to this task", ErrValidation)
		}
		if waiver.ExpiresAt != nil && waiver.ExpiresAt.Before(time.Now()) {
			return fmt.Errorf("%w: waiver has expired", ErrValidation)
		}
		hasWaiver = true
	}
	criteria, err := repo.ListAcceptanceCriteria(ctx, taskID)
	if err != nil {
		return err
	}
	for _, criterion := range criteria {
		if criterion.Required && criterion.State != models.AcceptanceCriterionStatePassed && criterion.State != models.AcceptanceCriterionStateWaived {
			return fmt.Errorf("%w: required criterion is not satisfied", ErrValidation)
		}
	}
	evidence, err := repo.ListEvidence(ctx, taskID)
	if err != nil {
		return err
	}
	hasSupport := false
	for _, item := range evidence {
		if item.Revoked {
			continue
		}
		if item.Verdict == models.EvidenceVerdictContradicts {
			if hasWaiver && approved {
				continue
			}
			return fmt.Errorf("%w: active contradicting evidence blocks close", ErrValidation)
		}
		if item.Verdict == models.EvidenceVerdictSupports {
			hasSupport = true
		}
	}
	if !hasSupport && !hasWaiver {
		return fmt.Errorf("%w: supporting evidence is required", ErrValidation)
	}
	return nil
}

// approvalSatisfied reports whether a dangerous action is authorized. It accepts
// either a granted, unexpired ApprovalRequest referenced by ID (the auditable
// path, PRD SEC-03..06) or, for backward compatibility, a bare granted+token
// decision carried inline on the request.
func (s *conveyorService) approvalSatisfied(ctx context.Context, repo ConveyorRepository, token string, granted bool) bool {
	token = strings.TrimSpace(token)
	if token != "" {
		if id, err := uuid.Parse(token); err == nil {
			if approval, err := repo.GetApprovalRequest(ctx, id); err == nil {
				if approval.Status == models.ApprovalStatusGranted && (approval.ExpiresAt == nil || approval.ExpiresAt.After(time.Now())) {
					return true
				}
				return false
			}
		}
	}
	return hasApprovalDecision(granted, token)
}

func (s *conveyorService) runIdempotent(ctx context.Context, actor ConveyorActor, operation string, key string, request any, fn func(ConveyorRepository) (*ConveyorMutationResult, error)) (*ConveyorMutationResult, error) {
	hash, err := requestHash(request)
	if err != nil {
		return nil, err
	}
	var result *ConveyorMutationResult
	err = s.repo.WithTransaction(ctx, func(repo ConveyorRepository) error {
		if key != "" {
			record, err := repo.GetIdempotencyRecord(ctx, actor.ActorID, operation, key)
			if err == nil {
				if record.RequestHash != hash {
					return ErrConflict
				}
				var previous ConveyorMutationResult
				if err := json.Unmarshal(record.Result, &previous); err != nil {
					return fmt.Errorf("decode idempotency result: %w", err)
				}
				previous.Replayed = true
				result = &previous
				return nil
			}
			if !errors.Is(err, ErrNotFound) {
				return err
			}
		}
		mutationResult, err := fn(repo)
		if err != nil {
			return err
		}
		if key != "" {
			encoded, err := json.Marshal(mutationResult)
			if err != nil {
				return err
			}
			record := &models.IdempotencyRecord{ID: uuid.New(), ActorID: actor.ActorID, Operation: operation, IdempotencyKey: key, RequestHash: hash, Result: encoded, EventID: mutationResult.EventID, CreatedAt: time.Now()}
			if err := repo.CreateIdempotencyRecord(ctx, record); err != nil {
				return err
			}
		}
		result = mutationResult
		return nil
	})
	if err != nil {
		if key != "" && isUniqueConstraintError(err) {
			return s.replayIdempotency(ctx, actor, operation, key, hash)
		}
		return nil, err
	}
	return result, nil
}

func (s *conveyorService) replayIdempotency(ctx context.Context, actor ConveyorActor, operation string, key string, hash string) (*ConveyorMutationResult, error) {
	record, err := s.repo.GetIdempotencyRecord(ctx, actor.ActorID, operation, key)
	if err != nil {
		return nil, err
	}
	if record.RequestHash != hash {
		return nil, ErrConflict
	}
	var previous ConveyorMutationResult
	if err := json.Unmarshal(record.Result, &previous); err != nil {
		return nil, fmt.Errorf("decode idempotency result: %w", err)
	}
	previous.Replayed = true
	return &previous, nil
}

func isUniqueConstraintError(err error) bool {
	type sqlStateError interface {
		SQLState() string
	}
	var sqlState sqlStateError
	if errors.As(err, &sqlState) && sqlState.SQLState() == "23505" {
		return true
	}
	var pqErr *pq.Error
	if errors.As(err, &pqErr) && string(pqErr.Code) == "23505" {
		return true
	}
	return strings.Contains(err.Error(), "duplicate key value violates unique constraint")
}

func (s *conveyorService) createEvent(ctx context.Context, repo ConveyorRepository, actor ConveyorActor, taskID uuid.UUID, eventType string, idempotencyKey string, payload any) (*models.ConveyorEvent, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	if actor.ActorType == "" {
		actor.ActorType = "user"
	}
	if actor.Source == "" {
		actor.Source = "rest"
	}
	var keyPtr *string
	if idempotencyKey != "" {
		keyPtr = &idempotencyKey
	}
	event := &models.ConveyorEvent{ID: uuid.New(), WorkItemID: taskID, Type: eventType, Timestamp: time.Now(), ActorType: actor.ActorType, ActorID: actor.ActorID, AgentID: actor.AgentID, OnBehalfOfUserID: actor.OnBehalfOfUserID, APITokenID: actor.APITokenID, Payload: encoded, SchemaVersion: 1, IdempotencyKey: keyPtr, CorrelationID: actor.CorrelationID, RequestID: actor.RequestID, Source: actor.Source}
	if err := repo.CreateEvent(ctx, event); err != nil {
		return nil, err
	}
	// Realtime: лёгкий сигнал в SSE-шину (no-op если хаб не выставлен, напр. в тестах).
	publishGlobal(StreamEvent{Type: eventType, WorkItemID: taskID.String()})
	return event, nil
}

func requestHash(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func validCriterionState(state string) bool {
	switch state {
	case models.AcceptanceCriterionStateUnchecked, models.AcceptanceCriterionStatePassed, models.AcceptanceCriterionStateFailed, models.AcceptanceCriterionStateWaived:
		return true
	default:
		return false
	}
}

func validEvidenceType(value string) bool {
	switch value {
	case models.EvidenceTypeLink, models.EvidenceTypeFile, models.EvidenceTypeLog, models.EvidenceTypeMarkdownReport, models.EvidenceTypePR, models.EvidenceTypeCommit, models.EvidenceTypeScreenshot, models.EvidenceTypeHealthcheck, models.EvidenceTypeDeploymentLog:
		return true
	default:
		return false
	}
}

func hasApprovalDecision(granted bool, token string) bool {
	return granted && strings.TrimSpace(token) != ""
}

func containsSecretLikeEvidence(uri string, title string, metadata json.RawMessage) bool {
	if secretValuePattern.MatchString(uri) || secretValuePattern.MatchString(title) {
		return true
	}
	if len(metadata) == 0 {
		return false
	}
	var decoded any
	if err := json.Unmarshal(metadata, &decoded); err != nil {
		return secretValuePattern.Match(metadata)
	}
	return containsSecretLikeJSON(decoded)
}

func containsSecretLikeJSON(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, entry := range typed {
			if secretKeyPattern.MatchString(key) || containsSecretLikeJSON(entry) {
				return true
			}
		}
	case []any:
		for _, entry := range typed {
			if containsSecretLikeJSON(entry) {
				return true
			}
		}
	case string:
		return secretValuePattern.MatchString(typed)
	}
	return false
}

func validEvidenceVerdict(value string) bool {
	switch value {
	case models.EvidenceVerdictSupports, models.EvidenceVerdictContradicts, models.EvidenceVerdictInformational:
		return true
	default:
		return false
	}
}

func validTaskLinkType(value string) bool {
	switch value {
	case models.TaskLinkTypeBlocks, models.TaskLinkTypeBlockedBy, models.TaskLinkTypeRelatesTo, models.TaskLinkTypeDuplicates, models.TaskLinkTypeParent, models.TaskLinkTypeChild:
		return true
	default:
		return false
	}
}

func validAgentRunStatus(value string) bool {
	switch value {
	case models.AgentRunStatusQueued, models.AgentRunStatusRunning, models.AgentRunStatusSucceeded, models.AgentRunStatusFailed, models.AgentRunStatusCanceled, models.AgentRunStatusNeedsHuman:
		return true
	default:
		return false
	}
}

func validAgentRunTransition(previous string, next string) bool {
	if previous == next {
		return true
	}
	switch previous {
	case models.AgentRunStatusQueued:
		return next == models.AgentRunStatusRunning || next == models.AgentRunStatusCanceled
	case models.AgentRunStatusRunning:
		return next == models.AgentRunStatusSucceeded || next == models.AgentRunStatusFailed || next == models.AgentRunStatusCanceled || next == models.AgentRunStatusNeedsHuman
	case models.AgentRunStatusNeedsHuman:
		return next == models.AgentRunStatusRunning || next == models.AgentRunStatusCanceled
	default:
		return false
	}
}

func isTerminalAgentRunStatus(value string) bool {
	switch value {
	case models.AgentRunStatusSucceeded, models.AgentRunStatusFailed, models.AgentRunStatusCanceled:
		return true
	default:
		return false
	}
}

func actorTypeOrDefault(value string) string {
	if value == "" {
		return "user"
	}
	return value
}

func normalizedJSON(value json.RawMessage) json.RawMessage {
	if len(value) == 0 {
		return nil
	}
	copyValue := make([]byte, len(value))
	copy(copyValue, value)
	return json.RawMessage(copyValue)
}

func validConversationSourceType(value string) bool {
	switch value {
	case models.ConversationSourceTypeForum, models.ConversationSourceTypeTelegram:
		return true
	default:
		return false
	}
}

func sanitizeForumText(value string) string {
	value = strings.ToValidUTF8(value, "")
	value = html.UnescapeString(value)
	value = forumMarkupPattern.ReplaceAllString(value, "")
	value = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, value)
	return strings.TrimSpace(value)
}

func sanitizedForumMessages(messages []ForumDigestSourceMessage) []ForumDigestSourceMessage {
	out := make([]ForumDigestSourceMessage, 0, len(messages))
	for _, message := range messages {
		item := ForumDigestSourceMessage{Locator: strings.TrimSpace(message.Locator), Author: sanitizeForumText(message.Author), Accessible: message.Accessible}
		if message.Accessible {
			item.Text = sanitizeForumText(message.Text)
		} else {
			item.Text = ""
			item.Redacted = true
		}
		out = append(out, item)
	}
	return out
}

func forumDigestResponse(digest models.ForumDigest, candidates []models.ForumActionCandidate) *ForumDigestResponse {
	response := &ForumDigestResponse{ID: digest.ID, SourceType: digest.SourceType, SourceID: digest.SourceID, SourceTitle: digest.SourceTitle, SourceLocator: digest.SourceLocator, PeriodStart: digest.PeriodStart, PeriodEnd: digest.PeriodEnd, Summary: digest.Summary, Decisions: digest.Decisions, SourceMessages: digest.SourceMessages, SourceMetadata: digest.SourceMetadata, CreatedBy: digest.CreatedBy, ActorType: digest.ActorType, CreatedAt: digest.CreatedAt, UpdatedAt: digest.UpdatedAt}
	response.Candidates = make([]ForumActionCandidateResponse, 0, len(candidates))
	for _, candidate := range candidates {
		response.Candidates = append(response.Candidates, forumActionCandidateResponse(candidate))
	}
	return response
}

func forumActionCandidateResponse(candidate models.ForumActionCandidate) ForumActionCandidateResponse {
	return ForumActionCandidateResponse{ID: candidate.ID, DigestID: candidate.DigestID, Title: candidate.Title, Description: candidate.Description, SourceLocator: candidate.SourceLocator, SourceMetadata: candidate.SourceMetadata, Status: candidate.Status, WorkItemID: candidate.WorkItemID, CreatedBy: candidate.CreatedBy, CreatedAt: candidate.CreatedAt, UpdatedAt: candidate.UpdatedAt}
}

func boolPtr(value bool) *bool { return &value }

func GeneratedReportResponseFromModel(report models.GeneratedReport) (*GeneratedReportResponse, error) {
	response := &GeneratedReportResponse{ID: report.ID, ProjectID: report.ProjectID, PeriodStart: report.PeriodStart, PeriodEnd: report.PeriodEnd, CreatedBy: report.CreatedBy, ActorType: report.ActorType, CreatedAt: report.CreatedAt, UpdatedAt: report.UpdatedAt, LLMError: report.LLMError}
	if err := unmarshalJSON(report.SourceEventIDs, &response.SourceEventIDs); err != nil {
		return nil, err
	}
	if err := unmarshalJSON(report.SourceEvidenceIDs, &response.SourceEvidenceIDs); err != nil {
		return nil, err
	}
	if err := unmarshalJSON(report.Facts, &response.Facts); err != nil {
		return nil, err
	}
	if err := unmarshalJSON(report.Conclusions, &response.Conclusions); err != nil {
		return nil, err
	}
	if err := unmarshalJSON(report.Risks, &response.Risks); err != nil {
		return nil, err
	}
	if len(report.LLMDraft) > 0 && string(report.LLMDraft) != "null" {
		if err := unmarshalJSON(report.LLMDraft, &response.LLMDraft); err != nil {
			return nil, err
		}
	}
	return response, nil
}

func unmarshalJSON(data json.RawMessage, target any) error {
	if len(data) == 0 {
		data = json.RawMessage(`[]`)
	}
	return json.Unmarshal(data, target)
}

func mustMarshalJSON(value any) json.RawMessage {
	data, err := json.Marshal(value)
	if err != nil {
		panic(fmt.Sprintf("marshal generated report: %v", err))
	}
	return json.RawMessage(data)
}

func activeEvidenceByVerdict(items []models.Evidence, verdict string) []models.Evidence {
	out := []models.Evidence{}
	for _, item := range items {
		if !item.Revoked && item.Verdict == verdict {
			out = append(out, item)
		}
	}
	return out
}

func evidenceIDs(items []models.Evidence) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(items))
	for _, item := range items {
		out = append(out, item.ID)
	}
	return out
}

func eventsByType(items []models.ConveyorEvent, eventType string) []models.ConveyorEvent {
	out := []models.ConveyorEvent{}
	for _, item := range items {
		if item.Type == eventType {
			out = append(out, item)
		}
	}
	return out
}

func uniqueUUIDs(values []uuid.UUID) []uuid.UUID {
	seen := map[uuid.UUID]bool{}
	out := []uuid.UUID{}
	for _, value := range values {
		if value == uuid.Nil || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out
}

func completedWorkText(task models.Task) string {
	if task.Name != nil && strings.TrimSpace(*task.Name) != "" {
		return "Completed work: " + strings.TrimSpace(*task.Name)
	}
	return "Completed work: " + task.ID.String()
}

func firstReportAnchor(sources []ports.ProjectReportSource) uuid.UUID {
	for _, source := range sources {
		if source.Task.ID != uuid.Nil {
			return source.Task.ID
		}
	}
	return uuid.Nil
}

func firstReportWorkItem(report *GeneratedReportResponse) uuid.UUID {
	for _, item := range report.Facts {
		if item.WorkItemID != uuid.Nil {
			return item.WorkItemID
		}
	}
	for _, item := range report.Risks {
		if item.WorkItemID != uuid.Nil {
			return item.WorkItemID
		}
	}
	return uuid.Nil
}

func generatedReportMarkdown(report *GeneratedReportResponse) string {
	var builder strings.Builder
	builder.WriteString("# Generated Report\n\n")
	builder.WriteString("Project: ")
	builder.WriteString(report.ProjectID.String())
	builder.WriteString("\n")
	builder.WriteString("Period: ")
	builder.WriteString(report.PeriodStart.Format(time.RFC3339))
	builder.WriteString(" - ")
	builder.WriteString(report.PeriodEnd.Format(time.RFC3339))
	builder.WriteString("\n\n")
	writeReportItems(&builder, "Facts", report.Facts)
	writeReportItems(&builder, "Conclusions", report.Conclusions)
	writeReportItems(&builder, "Risks", report.Risks)
	if report.LLMDraft.Text != "" {
		builder.WriteString("## LLM Draft (Auxiliary)\n\n")
		builder.WriteString(escapeMarkdown(report.LLMDraft.Text))
		builder.WriteString("\n\n")
	}
	if report.LLMError != "" {
		builder.WriteString("## LLM Error\n\n")
		builder.WriteString(escapeMarkdown(report.LLMError))
		builder.WriteString("\n\n")
	}
	builder.WriteString("## Sources\n\n")
	builder.WriteString("Events: ")
	builder.WriteString(joinUUIDs(report.SourceEventIDs))
	builder.WriteString("\n")
	builder.WriteString("Evidence: ")
	builder.WriteString(joinUUIDs(report.SourceEvidenceIDs))
	builder.WriteString("\n")
	return builder.String()
}

func writeReportItems(builder *strings.Builder, title string, items []GeneratedReportItem) {
	builder.WriteString("## ")
	builder.WriteString(title)
	builder.WriteString("\n\n")
	if len(items) == 0 {
		builder.WriteString("- None\n\n")
		return
	}
	for _, item := range items {
		builder.WriteString("- ")
		builder.WriteString(escapeMarkdown(item.Text))
		if item.WorkItemID != uuid.Nil {
			builder.WriteString(" (work_item_id: ")
			builder.WriteString(item.WorkItemID.String())
			builder.WriteString(")")
		}
		if len(item.SourceEventIDs) > 0 {
			builder.WriteString(" events: ")
			builder.WriteString(joinUUIDs(item.SourceEventIDs))
		}
		if len(item.SourceEvidenceIDs) > 0 {
			builder.WriteString(" evidence: ")
			builder.WriteString(joinUUIDs(item.SourceEvidenceIDs))
		}
		builder.WriteString("\n")
	}
	builder.WriteString("\n")
}

func joinUUIDs(values []uuid.UUID) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, value.String())
	}
	return strings.Join(parts, ", ")
}

func escapeMarkdown(value string) string {
	replacer := strings.NewReplacer("\\", "\\\\", "*", "\\*", "[", "\\[", "]", "\\]", "_", "\\_", "`", "\\`")
	return replacer.Replace(value)
}

func reportPeriodEnd(value time.Time) time.Time {
	if value.Hour() == 0 && value.Minute() == 0 && value.Second() == 0 && value.Nanosecond() == 0 {
		return value.Add(24*time.Hour - time.Nanosecond)
	}
	return value
}

func mediumOrHigherRisk(value string) bool {
	switch strings.ToLower(value) {
	case "medium", "high", "critical":
		return true
	default:
		return false
	}
}
