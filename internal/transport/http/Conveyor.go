package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	models "emplacc-api/internal/domain"
	"emplacc-api/internal/service"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

type ConveyorController struct {
	svc      service.ConveyorService
	pmImport service.PMImportService
}

type conveyorErrorResponse struct {
	Error string `json:"error" example:"validation_error"`
}

type conveyorCriterionRequest struct {
	ACID           string   `json:"ac_id" example:"AC-1"`
	Title          string   `json:"title" example:"Run targeted backend tests"`
	SpecIDs        []string `json:"spec_ids" example:"SPEC-1,SPEC-2" swaggertype:"array,string"`
	Required       *bool    `json:"required" example:"true"`
	IdempotencyKey string   `json:"idempotency_key" example:"criterion-create-1"`
}

type conveyorCriterionStateRequest struct {
	State          string `json:"state" example:"passed"`
	IdempotencyKey string `json:"idempotency_key" example:"criterion-update-1"`
}

type conveyorEvidenceRequest struct {
	CriterionID     string          `json:"criterion_id" example:""`
	Type            string          `json:"type" example:"link"`
	Verdict         string          `json:"verdict" example:"supports"`
	URI             string          `json:"uri" example:"https://example.test/evidence"`
	Title           string          `json:"title" example:"Test evidence"`
	SHA256          string          `json:"sha256" example:""`
	Metadata        json.RawMessage `json:"metadata,omitempty" swaggertype:"object"`
	ApprovalGranted bool            `json:"approval_granted" example:"false"`
	ApprovalToken   string          `json:"approval_token" example:"approval-1"`
	IdempotencyKey  string          `json:"idempotency_key" example:"evidence-attach-1"`
}

type conveyorRevokeEvidenceRequest struct {
	Reason         string `json:"reason" example:"superseded"`
	IdempotencyKey string `json:"idempotency_key" example:"evidence-revoke-1"`
}

type conveyorTaskLinkRequest struct {
	TargetTaskID   string `json:"target_task_id" example:"9a888cc9-0e34-4f3e-8d9d-4924d9e35687"`
	LinkType       string `json:"link_type" example:"relates_to"`
	IdempotencyKey string `json:"idempotency_key" example:"task-link-1"`
}

type conveyorCloseTaskRequest struct {
	ToStatusID               string `json:"to_status_id" example:"9a888cc9-0e34-4f3e-8d9d-4924d9e35687"`
	TaskWaiverID             string `json:"task_waiver_id" example:""`
	RiskLevel                string `json:"risk_level" example:"medium"`
	ApprovalGranted          bool   `json:"approval_granted" example:"false"`
	ApprovalToken            string `json:"approval_token" example:"approval-1"`
	AllowDependencyAutoReady bool   `json:"allow_dependency_auto_ready" example:"false"`
	IdempotencyKey           string `json:"idempotency_key" example:"task-close-1"`
}

type conveyorWaiverRequest struct {
	Scope             string `json:"scope" example:"task"`
	CriterionID       string `json:"criterion_id" example:""`
	Reason            string `json:"reason" example:"deferred to follow-up ticket"`
	ApprovalRequestID string `json:"approval_request_id" example:""`
	ExpiresAt         string `json:"expires_at" example:""`
	IdempotencyKey    string `json:"idempotency_key" example:"waiver-create-1"`
}

type conveyorApprovalRequest struct {
	WorkItemID     string          `json:"work_item_id" example:""`
	Action         string          `json:"action" example:"production_deploy"`
	RiskLevel      string          `json:"risk_level" example:"high"`
	Reason         string          `json:"reason" example:"deploy product-a to staging"`
	Resource       json.RawMessage `json:"resource,omitempty" swaggertype:"object"`
	ExpiresAt      string          `json:"expires_at" example:""`
	IdempotencyKey string          `json:"idempotency_key" example:"approval-request-1"`
}

type conveyorApprovalDecisionRequest struct {
	Reason         string `json:"reason" example:"approved by owner"`
	IdempotencyKey string `json:"idempotency_key" example:"approval-decide-1"`
}

type conveyorWorkOrderCreateRequest struct {
	SourceTaskID             string          `json:"source_task_id"`
	ProviderBoardID          string          `json:"provider_board_id"`
	ProviderStatusID         string          `json:"provider_status_id"`
	Goal                     string          `json:"goal"`
	Inputs                   json.RawMessage `json:"inputs,omitempty"`
	AcceptanceCriteria       json.RawMessage `json:"acceptance_criteria,omitempty"`
	RequiredEvidenceMetadata json.RawMessage `json:"required_evidence_metadata,omitempty"`
	RequesterContext         json.RawMessage `json:"requester_context,omitempty"`
	ProviderContext          json.RawMessage `json:"provider_context,omitempty"`
	IdempotencyKey           string          `json:"idempotency_key"`
}

type conveyorWorkOrderAcceptRequest struct {
	TargetName     string `json:"target_name"`
	IdempotencyKey string `json:"idempotency_key"`
}

type conveyorWorkOrderReasonRequest struct {
	Reason         string `json:"reason"`
	IdempotencyKey string `json:"idempotency_key"`
}

type conveyorWorkOrderCompleteRequest struct {
	ResultEvidenceID string `json:"result_evidence_id"`
	EvidenceWaiver   string `json:"evidence_waiver"`
	IdempotencyKey   string `json:"idempotency_key"`
}

type conveyorAgentRunRequest struct {
	Source         string          `json:"source" example:"rest"`
	Harness        string          `json:"harness" example:"external-harness"`
	Status         string          `json:"status" example:"queued"`
	Summary        string          `json:"summary" example:"Run queued"`
	LogURI         string          `json:"log_uri" example:"https://example.test/log"`
	WorkspaceURI   string          `json:"workspace_uri" example:"file:///workspace"`
	Metadata       json.RawMessage `json:"metadata,omitempty" swaggertype:"object"`
	IdempotencyKey string          `json:"idempotency_key" example:"agent-run-register-1"`
}

type conveyorAgentRunUpdateRequest struct {
	Status         string          `json:"status" example:"running"`
	Summary        string          `json:"summary" example:"Run started"`
	LogURI         string          `json:"log_uri" example:"https://example.test/log"`
	WorkspaceURI   string          `json:"workspace_uri" example:"file:///workspace"`
	Metadata       json.RawMessage `json:"metadata,omitempty" swaggertype:"object"`
	IdempotencyKey string          `json:"idempotency_key" example:"agent-run-update-1"`
}

type conveyorGeneratedReportRequest struct {
	PeriodStart    time.Time `json:"period_start"`
	PeriodEnd      time.Time `json:"period_end"`
	IncludeLLM     bool      `json:"include_llm"`
	IdempotencyKey string    `json:"idempotency_key" example:"generated-report-1"`
}

type conveyorForumDigestRequest struct {
	SourceType      string                              `json:"source_type"`
	SourceID        string                              `json:"source_id"`
	SourceTitle     string                              `json:"source_title"`
	SourceLocator   string                              `json:"source_locator"`
	PeriodStart     time.Time                           `json:"period_start"`
	PeriodEnd       time.Time                           `json:"period_end"`
	Summary         string                              `json:"summary"`
	Decisions       json.RawMessage                     `json:"decisions,omitempty" swaggertype:"object"`
	SourceMetadata  json.RawMessage                     `json:"source_metadata,omitempty" swaggertype:"object"`
	Messages        []service.ForumDigestSourceMessage  `json:"messages,omitempty"`
	CandidateInputs []service.ForumActionCandidateInput `json:"candidate_inputs,omitempty"`
	UseLLM          bool                                `json:"use_llm" example:"false"`
}

type conveyorForumCandidateConfirmRequest struct {
	StatusID       string `json:"status_id"`
	AssignedTo     string `json:"assigned_to"`
	Priority       *int16 `json:"priority"`
	IdempotencyKey string `json:"idempotency_key"`
}

type conveyorForumCandidateRejectRequest struct {
	Reason         string `json:"reason"`
	IdempotencyKey string `json:"idempotency_key"`
}

type conveyorPMImportRequest struct {
	Scope           string            `json:"scope" example:"default"`
	DryRun          bool              `json:"dry_run" example:"true"`
	StatusByPMState map[string]string `json:"status_by_pm_state" swaggertype:"object"`
	ApprovalGranted bool              `json:"approval_granted" example:"false"`
	ApprovalToken   string            `json:"approval_token" example:"approval-1"`
	IdempotencyKey  string            `json:"idempotency_key" example:"pm-import-1"`
}

func RegisterConveyorRoutes(e Router, svc service.ConveyorService, pmImport service.PMImportService, employeeMw echo.MiddlewareFunc, managerMw echo.MiddlewareFunc) {
	c := &ConveyorController{svc: svc, pmImport: pmImport}
	g := e.Group("/api/work-items")
	g.GET("/:id/criteria", c.ListCriteria)
	g.GET("/:id/evidence", c.ListEvidence)
	g.GET("/:id/events", c.ListEvents)
	g.GET("/:id/links", c.ListLinks)
	g.GET("/:id/agent-runs", c.ListAgentRuns)
	g.GET("/:id/agent-runs/:agent_run_id", c.GetAgentRun)
	g.GET("/:id/work-orders", c.ListWorkOrders)
	g.POST("/:id/criteria", c.CreateCriterion, employeeMw)
	g.PATCH("/:id/criteria/:criterion_id", c.UpdateCriterionState, employeeMw)
	g.POST("/:id/evidence", c.AttachEvidence, employeeMw)
	g.POST("/:id/evidence/:evidence_id/revoke", c.RevokeEvidence, employeeMw)
	g.POST("/:id/links", c.LinkTasks, employeeMw)
	g.POST("/:id/close", c.CloseTask, employeeMw)
	g.POST("/:id/waivers", c.CreateWaiver, employeeMw)
	g.GET("/:id/approval-requests", c.ListApprovalRequests)
	g.POST("/:id/suggest-criteria", c.SuggestCriteria, employeeMw)
	g.POST("/:id/evidence-summary", c.SummarizeEvidence, employeeMw)
	g.POST("/:id/agent-runs", c.RegisterAgentRun, employeeMw)
	g.PATCH("/:id/agent-runs/:agent_run_id", c.UpdateAgentRun, employeeMw)
	g.POST("/:id/agent-runs/:agent_run_id/heartbeat", c.HeartbeatAgentRun, employeeMw)

	projects := e.Group("/api/projects")
	projects.POST("/:project_id/generated-reports", c.CreateGeneratedReport, employeeMw)
	reports := e.Group("/api/generated-reports")
	reports.GET("/:id", c.GetGeneratedReport)
	reports.GET("/:id/markdown", c.ExportGeneratedReportMarkdown)
	forumDigests := e.Group("/api/forum-digests")
	forumDigests.POST("", c.CreateForumDigest, employeeMw)
	forumDigests.GET("", c.ListForumDigests)
	forumDigests.GET("/:id", c.GetForumDigest)
	forumCandidates := e.Group("/api/forum-action-candidates")
	forumCandidates.POST("/:id/confirm", c.ConfirmForumActionCandidate, employeeMw)
	forumCandidates.POST("/:id/reject", c.RejectForumActionCandidate, employeeMw)

	workOrders := e.Group("/api/work-orders")
	workOrders.POST("", c.CreateWorkOrder, employeeMw)
	workOrders.GET("/:id", c.GetWorkOrder)
	workOrders.POST("/:id/accept", c.AcceptWorkOrder, employeeMw)
	workOrders.POST("/:id/reject", c.RejectWorkOrder, employeeMw)
	workOrders.POST("/:id/complete", c.CompleteWorkOrder, employeeMw)
	workOrders.POST("/:id/cancel", c.CancelWorkOrder, employeeMw)
	workOrders.POST("/:id/fail", c.FailWorkOrder, employeeMw)

	waivers := e.Group("/api/waivers")
	waivers.GET("/:id", c.GetWaiver)
	approvals := e.Group("/api/approval-requests")
	approvals.POST("", c.RequestApproval, employeeMw)
	approvals.GET("/:id", c.GetApprovalRequest)
	approvals.POST("/:id/grant", c.GrantApproval, employeeMw)
	approvals.POST("/:id/deny", c.DenyApproval, employeeMw)

	conveyor := e.Group("/api/conveyor")
	conveyor.GET("/inbox", c.ListAgentInbox, employeeMw)
	conveyor.POST("/inbox/:item_id/ack", c.AckAgentInboxItem, employeeMw)
	conveyor.POST("/pm-import", c.ImportPMCanon, employeeMw)
	conveyor.GET("/pending-approvals", c.ListPendingApprovals, managerMw)
}

// ImportPMCanon godoc
// @Summary Import PM canon into Conveyor
// @Tags conveyor
// @Accept json
// @Produce json
// @Param input body conveyorPMImportRequest true "PM import data"
// @Success 200 {object} service.PMImportSummary
// @Failure 400 {object} conveyorErrorResponse
// @Failure 409 {object} conveyorErrorResponse
// @Router /api/conveyor/pm-import [post]
// @Security BearerAuth
func (c *ConveyorController) ImportPMCanon(ctx echo.Context) error {
	actor, err := actorFromContext(ctx)
	if err != nil {
		return conveyorError(ctx, err)
	}
	var req conveyorPMImportRequest
	if err := ctx.Bind(&req); err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	if req.ApprovalGranted != true || strings.TrimSpace(req.ApprovalToken) == "" {
		return conveyorError(ctx, service.ErrApprovalRequired)
	}
	if c.pmImport == nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	rootPath, err := pmImportRootPath(req.Scope)
	if err != nil {
		return conveyorError(ctx, err)
	}
	statusByPMState, err := parsePMImportStatusMap(req.StatusByPMState)
	if err != nil {
		return conveyorError(ctx, err)
	}
	summary, err := c.pmImport.ImportPMCanon(ctx.Request().Context(), actor, service.PMImportRequest{RootPath: rootPath, DryRun: req.DryRun, StatusByPMState: statusByPMState})
	if err != nil {
		return conveyorError(ctx, err)
	}
	return ctx.JSON(http.StatusOK, summary)
}

// ListCriteria godoc
// @Summary List work item criteria
// @Tags conveyor
// @Produce json
// @Param id path string true "Work item ID"
// @Success 200 {array} models.AcceptanceCriterion
// @Failure 400 {object} conveyorErrorResponse
// @Router /api/work-items/{id}/criteria [get]
// @Security BearerAuth
func (c *ConveyorController) ListCriteria(ctx echo.Context) error {
	taskID, err := parseUUIDParam(ctx, "id")
	if err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	actor, err := actorFromContext(ctx)
	if err != nil {
		return conveyorError(ctx, err)
	}
	if err := c.svc.AuthorizeWorkItemAccess(ctx.Request().Context(), actor, taskID); err != nil {
		return conveyorError(ctx, err)
	}
	items, err := c.svc.ListAcceptanceCriteria(ctx.Request().Context(), taskID)
	if err != nil {
		return conveyorError(ctx, err)
	}
	return ctx.JSON(http.StatusOK, items)
}

// ListWorkOrders — наряды, созданные из задачи (source_task_id).
func (c *ConveyorController) ListWorkOrders(ctx echo.Context) error {
	taskID, err := parseUUIDParam(ctx, "id")
	if err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	actor, err := actorFromContext(ctx)
	if err != nil {
		return conveyorError(ctx, err)
	}
	if err := c.svc.AuthorizeWorkItemAccess(ctx.Request().Context(), actor, taskID); err != nil {
		return conveyorError(ctx, err)
	}
	items, err := c.svc.ListWorkOrders(ctx.Request().Context(), taskID)
	if err != nil {
		return conveyorError(ctx, err)
	}
	return ctx.JSON(http.StatusOK, items)
}

// ListEvidence godoc
// @Summary List work item evidence
// @Tags conveyor
// @Produce json
// @Param id path string true "Work item ID"
// @Success 200 {array} models.Evidence
// @Failure 400 {object} conveyorErrorResponse
// @Router /api/work-items/{id}/evidence [get]
// @Security BearerAuth
func (c *ConveyorController) ListEvidence(ctx echo.Context) error {
	taskID, err := parseUUIDParam(ctx, "id")
	if err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	actor, err := actorFromContext(ctx)
	if err != nil {
		return conveyorError(ctx, err)
	}
	if err := c.svc.AuthorizeWorkItemAccess(ctx.Request().Context(), actor, taskID); err != nil {
		return conveyorError(ctx, err)
	}
	items, err := c.svc.ListEvidence(ctx.Request().Context(), taskID)
	if err != nil {
		return conveyorError(ctx, err)
	}
	return ctx.JSON(http.StatusOK, items)
}

// ListEvents godoc
// @Summary List work item events
// @Tags conveyor
// @Produce json
// @Param id path string true "Work item ID"
// @Success 200 {array} models.ConveyorEvent
// @Failure 400 {object} conveyorErrorResponse
// @Router /api/work-items/{id}/events [get]
// @Security BearerAuth
func (c *ConveyorController) ListEvents(ctx echo.Context) error {
	taskID, err := parseUUIDParam(ctx, "id")
	if err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	actor, err := actorFromContext(ctx)
	if err != nil {
		return conveyorError(ctx, err)
	}
	if err := c.svc.AuthorizeWorkItemAccess(ctx.Request().Context(), actor, taskID); err != nil {
		return conveyorError(ctx, err)
	}
	items, err := c.svc.ListEvents(ctx.Request().Context(), taskID)
	if err != nil {
		return conveyorError(ctx, err)
	}
	return ctx.JSON(http.StatusOK, items)
}

// ListLinks godoc
// @Summary List work item links
// @Tags conveyor
// @Produce json
// @Param id path string true "Work item ID"
// @Success 200 {array} models.TaskLink
// @Failure 400 {object} conveyorErrorResponse
// @Failure 404 {object} conveyorErrorResponse
// @Router /api/work-items/{id}/links [get]
// @Security BearerAuth
func (c *ConveyorController) ListLinks(ctx echo.Context) error {
	taskID, err := parseUUIDParam(ctx, "id")
	if err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	actor, err := actorFromContext(ctx)
	if err != nil {
		return conveyorError(ctx, err)
	}
	if err := c.svc.AuthorizeWorkItemAccess(ctx.Request().Context(), actor, taskID); err != nil {
		return conveyorError(ctx, err)
	}
	items, err := c.svc.ListTaskLinks(ctx.Request().Context(), taskID)
	if err != nil {
		return conveyorError(ctx, err)
	}
	return ctx.JSON(http.StatusOK, items)
}

// ListAgentRuns godoc
// @Summary List work item agent runs
// @Tags conveyor
// @Produce json
// @Param id path string true "Work item ID"
// @Success 200 {array} models.AgentRun
// @Failure 400 {object} conveyorErrorResponse
// @Router /api/work-items/{id}/agent-runs [get]
// @Security BearerAuth
func (c *ConveyorController) ListAgentRuns(ctx echo.Context) error {
	taskID, err := parseUUIDParam(ctx, "id")
	if err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	actor, err := actorFromContext(ctx)
	if err != nil {
		return conveyorError(ctx, err)
	}
	if err := c.svc.AuthorizeWorkItemAccess(ctx.Request().Context(), actor, taskID); err != nil {
		return conveyorError(ctx, err)
	}
	items, err := c.svc.ListAgentRuns(ctx.Request().Context(), taskID)
	if err != nil {
		return conveyorError(ctx, err)
	}
	return ctx.JSON(http.StatusOK, items)
}

// GetAgentRun godoc
// @Summary Get work item agent run
// @Tags conveyor
// @Produce json
// @Param id path string true "Work item ID"
// @Param agent_run_id path string true "Agent run ID"
// @Success 200 {object} models.AgentRun
// @Failure 400 {object} conveyorErrorResponse
// @Failure 404 {object} conveyorErrorResponse
// @Router /api/work-items/{id}/agent-runs/{agent_run_id} [get]
// @Security BearerAuth
func (c *ConveyorController) GetAgentRun(ctx echo.Context) error {
	taskID, err := parseUUIDParam(ctx, "id")
	if err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	runID, err := parseUUIDParam(ctx, "agent_run_id")
	if err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	actor, err := actorFromContext(ctx)
	if err != nil {
		return conveyorError(ctx, err)
	}
	if err := c.svc.AuthorizeWorkItemAccess(ctx.Request().Context(), actor, taskID); err != nil {
		return conveyorError(ctx, err)
	}
	item, err := c.svc.GetAgentRun(ctx.Request().Context(), taskID, runID)
	if err != nil {
		return conveyorError(ctx, err)
	}
	return ctx.JSON(http.StatusOK, item)
}

// CreateCriterion godoc
// @Summary Create work item criterion
// @Tags conveyor
// @Accept json
// @Produce json
// @Param id path string true "Work item ID"
// @Param input body conveyorCriterionRequest true "Criterion data"
// @Success 201 {object} service.ConveyorMutationResult
// @Failure 400 {object} conveyorErrorResponse
// @Failure 409 {object} conveyorErrorResponse
// @Router /api/work-items/{id}/criteria [post]
// @Security BearerAuth
func (c *ConveyorController) CreateCriterion(ctx echo.Context) error {
	taskID, err := parseUUIDParam(ctx, "id")
	if err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	actor, err := actorFromContext(ctx)
	if err != nil {
		return conveyorError(ctx, err)
	}
	var req conveyorCriterionRequest
	if err := ctx.Bind(&req); err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	required := true
	if req.Required != nil {
		required = *req.Required
	}
	result, err := c.svc.CreateAcceptanceCriterion(ctx.Request().Context(), actor, service.CreateAcceptanceCriterionRequest{TaskID: taskID, ACID: req.ACID, Title: req.Title, SpecIDs: req.SpecIDs, Required: required, IdempotencyKey: idempotencyKey(ctx, req.IdempotencyKey)})
	if err != nil {
		return conveyorError(ctx, err)
	}
	return ctx.JSON(http.StatusCreated, result)
}

// UpdateCriterionState godoc
// @Summary Update criterion state
// @Tags conveyor
// @Accept json
// @Produce json
// @Param id path string true "Work item ID"
// @Param criterion_id path string true "Criterion ID"
// @Param input body conveyorCriterionStateRequest true "Criterion state"
// @Success 200 {object} service.ConveyorMutationResult
// @Failure 400 {object} conveyorErrorResponse
// @Failure 404 {object} conveyorErrorResponse
// @Failure 409 {object} conveyorErrorResponse
// @Router /api/work-items/{id}/criteria/{criterion_id} [patch]
// @Security BearerAuth
func (c *ConveyorController) UpdateCriterionState(ctx echo.Context) error {
	taskID, err := parseUUIDParam(ctx, "id")
	if err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	criterionID, err := parseUUIDParam(ctx, "criterion_id")
	if err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	actor, err := actorFromContext(ctx)
	if err != nil {
		return conveyorError(ctx, err)
	}
	var req conveyorCriterionStateRequest
	if err := ctx.Bind(&req); err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	result, err := c.svc.UpdateAcceptanceCriterionState(ctx.Request().Context(), actor, taskID, criterionID, service.UpdateCriterionStateRequest{State: req.State, IdempotencyKey: idempotencyKey(ctx, req.IdempotencyKey)})
	if err != nil {
		return conveyorError(ctx, err)
	}
	return ctx.JSON(http.StatusOK, result)
}

// AttachEvidence godoc
// @Summary Attach work item evidence
// @Tags conveyor
// @Accept json
// @Produce json
// @Param id path string true "Work item ID"
// @Param input body conveyorEvidenceRequest true "Evidence data"
// @Success 201 {object} service.ConveyorMutationResult
// @Failure 400 {object} conveyorErrorResponse
// @Failure 409 {object} conveyorErrorResponse
// @Router /api/work-items/{id}/evidence [post]
// @Security BearerAuth
func (c *ConveyorController) AttachEvidence(ctx echo.Context) error {
	taskID, err := parseUUIDParam(ctx, "id")
	if err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	actor, err := actorFromContext(ctx)
	if err != nil {
		return conveyorError(ctx, err)
	}
	if err := c.svc.AuthorizeWorkItemAccess(ctx.Request().Context(), actor, taskID); err != nil {
		return conveyorError(ctx, err)
	}
	var req conveyorEvidenceRequest
	if err := ctx.Bind(&req); err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	var criterionID *uuid.UUID
	if trimmed := strings.TrimSpace(req.CriterionID); trimmed != "" {
		parsed, perr := uuid.Parse(trimmed)
		if perr != nil {
			return conveyorError(ctx, service.ErrValidation)
		}
		criterionID = &parsed
	}
	result, err := c.svc.AttachEvidence(ctx.Request().Context(), actor, service.AttachEvidenceRequest{TaskID: taskID, CriterionID: criterionID, Type: req.Type, Verdict: req.Verdict, URI: req.URI, Title: req.Title, SHA256: req.SHA256, Metadata: []byte(req.Metadata), ApprovalGranted: req.ApprovalGranted, ApprovalToken: req.ApprovalToken, IdempotencyKey: idempotencyKey(ctx, req.IdempotencyKey)})
	if err != nil {
		return conveyorError(ctx, err)
	}
	return ctx.JSON(http.StatusCreated, result)
}

// RevokeEvidence godoc
// @Summary Revoke work item evidence
// @Tags conveyor
// @Accept json
// @Produce json
// @Param id path string true "Work item ID"
// @Param evidence_id path string true "Evidence ID"
// @Param input body conveyorRevokeEvidenceRequest true "Revocation data"
// @Success 200 {object} service.ConveyorMutationResult
// @Failure 400 {object} conveyorErrorResponse
// @Failure 404 {object} conveyorErrorResponse
// @Router /api/work-items/{id}/evidence/{evidence_id}/revoke [post]
// @Security BearerAuth
func (c *ConveyorController) RevokeEvidence(ctx echo.Context) error {
	taskID, err := parseUUIDParam(ctx, "id")
	if err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	evidenceID, err := parseUUIDParam(ctx, "evidence_id")
	if err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	actor, err := actorFromContext(ctx)
	if err != nil {
		return conveyorError(ctx, err)
	}
	var req conveyorRevokeEvidenceRequest
	if err := ctx.Bind(&req); err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	result, err := c.svc.RevokeEvidence(ctx.Request().Context(), actor, taskID, evidenceID, service.RevokeEvidenceRequest{Reason: req.Reason, IdempotencyKey: idempotencyKey(ctx, req.IdempotencyKey)})
	if err != nil {
		return conveyorError(ctx, err)
	}
	return ctx.JSON(http.StatusOK, result)
}

// LinkTasks godoc
// @Summary Link work items
// @Tags conveyor
// @Accept json
// @Produce json
// @Param id path string true "Source work item ID"
// @Param input body conveyorTaskLinkRequest true "Task link data"
// @Success 201 {object} service.ConveyorMutationResult
// @Failure 400 {object} conveyorErrorResponse
// @Failure 409 {object} conveyorErrorResponse
// @Router /api/work-items/{id}/links [post]
// @Security BearerAuth
func (c *ConveyorController) LinkTasks(ctx echo.Context) error {
	sourceID, err := parseUUIDParam(ctx, "id")
	if err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	actor, err := actorFromContext(ctx)
	if err != nil {
		return conveyorError(ctx, err)
	}
	var req conveyorTaskLinkRequest
	if err := ctx.Bind(&req); err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	targetID, err := uuid.Parse(req.TargetTaskID)
	if err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	result, err := c.svc.LinkTasks(ctx.Request().Context(), actor, service.LinkTasksRequest{SourceTaskID: sourceID, TargetTaskID: targetID, LinkType: req.LinkType, IdempotencyKey: idempotencyKey(ctx, req.IdempotencyKey)})
	if err != nil {
		return conveyorError(ctx, err)
	}
	return ctx.JSON(http.StatusCreated, result)
}

// CloseTask godoc
// @Summary Close work item with evidence gate
// @Tags conveyor
// @Accept json
// @Produce json
// @Param id path string true "Work item ID"
// @Param input body conveyorCloseTaskRequest true "Close request"
// @Success 200 {object} service.ConveyorMutationResult
// @Failure 400 {object} conveyorErrorResponse
// @Failure 409 {object} conveyorErrorResponse
// @Router /api/work-items/{id}/close [post]
// @Security BearerAuth
func (c *ConveyorController) CloseTask(ctx echo.Context) error {
	taskID, err := parseUUIDParam(ctx, "id")
	if err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	actor, err := actorFromContext(ctx)
	if err != nil {
		return conveyorError(ctx, err)
	}
	var req conveyorCloseTaskRequest
	if err := ctx.Bind(&req); err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	statusID, err := uuid.Parse(req.ToStatusID)
	if err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	var waiverID *uuid.UUID
	if req.TaskWaiverID != "" {
		parsed, err := uuid.Parse(req.TaskWaiverID)
		if err != nil {
			return conveyorError(ctx, service.ErrValidation)
		}
		waiverID = &parsed
	}
	result, err := c.svc.CloseTask(ctx.Request().Context(), actor, taskID, service.CloseTaskRequest{ToStatusID: statusID, TaskWaiverID: waiverID, RiskLevel: req.RiskLevel, ApprovalGranted: req.ApprovalGranted, ApprovalToken: req.ApprovalToken, AllowDependencyAutoReady: req.AllowDependencyAutoReady, IdempotencyKey: idempotencyKey(ctx, req.IdempotencyKey)})
	if err != nil {
		return conveyorError(ctx, err)
	}
	return ctx.JSON(http.StatusOK, result)
}

func (c *ConveyorController) CreateWorkOrder(ctx echo.Context) error {
	actor, err := actorFromContext(ctx)
	if err != nil {
		return conveyorError(ctx, err)
	}
	var req conveyorWorkOrderCreateRequest
	if err := ctx.Bind(&req); err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	sourceTaskID, err := uuid.Parse(req.SourceTaskID)
	if err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	providerBoardID, err := uuid.Parse(req.ProviderBoardID)
	if err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	providerStatusID, err := uuid.Parse(req.ProviderStatusID)
	if err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	result, err := c.svc.CreateWorkOrder(ctx.Request().Context(), actor, service.CreateWorkOrderRequest{SourceTaskID: sourceTaskID, ProviderBoardID: providerBoardID, ProviderStatusID: providerStatusID, Goal: req.Goal, Inputs: req.Inputs, AcceptanceCriteria: req.AcceptanceCriteria, RequiredEvidenceMetadata: req.RequiredEvidenceMetadata, RequesterContext: req.RequesterContext, ProviderContext: req.ProviderContext, IdempotencyKey: idempotencyKey(ctx, req.IdempotencyKey)})
	if err != nil {
		return conveyorError(ctx, err)
	}
	return ctx.JSON(http.StatusCreated, result)
}

func (c *ConveyorController) GetWorkOrder(ctx echo.Context) error {
	workOrderID, err := parseUUIDParam(ctx, "id")
	if err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	workOrder, err := c.svc.GetWorkOrder(ctx.Request().Context(), workOrderID)
	if err != nil {
		return conveyorError(ctx, err)
	}
	return ctx.JSON(http.StatusOK, workOrder)
}

func (c *ConveyorController) AcceptWorkOrder(ctx echo.Context) error {
	workOrderID, err := parseUUIDParam(ctx, "id")
	if err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	actor, err := actorFromContext(ctx)
	if err != nil {
		return conveyorError(ctx, err)
	}
	var req conveyorWorkOrderAcceptRequest
	if err := ctx.Bind(&req); err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	result, err := c.svc.AcceptWorkOrder(ctx.Request().Context(), actor, workOrderID, service.AcceptWorkOrderRequest{TargetName: req.TargetName, IdempotencyKey: idempotencyKey(ctx, req.IdempotencyKey)})
	if err != nil {
		return conveyorError(ctx, err)
	}
	return ctx.JSON(http.StatusOK, result)
}

func (c *ConveyorController) RejectWorkOrder(ctx echo.Context) error {
	return c.workOrderReasonTransition(ctx, func(actor service.ConveyorActor, id uuid.UUID, req conveyorWorkOrderReasonRequest) (*service.ConveyorMutationResult, error) {
		return c.svc.RejectWorkOrder(ctx.Request().Context(), actor, id, service.RejectWorkOrderRequest{Reason: req.Reason, IdempotencyKey: idempotencyKey(ctx, req.IdempotencyKey)})
	})
}

func (c *ConveyorController) CancelWorkOrder(ctx echo.Context) error {
	return c.workOrderReasonTransition(ctx, func(actor service.ConveyorActor, id uuid.UUID, req conveyorWorkOrderReasonRequest) (*service.ConveyorMutationResult, error) {
		return c.svc.CancelWorkOrder(ctx.Request().Context(), actor, id, service.CancelWorkOrderRequest{Reason: req.Reason, IdempotencyKey: idempotencyKey(ctx, req.IdempotencyKey)})
	})
}

func (c *ConveyorController) FailWorkOrder(ctx echo.Context) error {
	return c.workOrderReasonTransition(ctx, func(actor service.ConveyorActor, id uuid.UUID, req conveyorWorkOrderReasonRequest) (*service.ConveyorMutationResult, error) {
		return c.svc.FailWorkOrder(ctx.Request().Context(), actor, id, service.FailWorkOrderRequest{Reason: req.Reason, IdempotencyKey: idempotencyKey(ctx, req.IdempotencyKey)})
	})
}

func (c *ConveyorController) CompleteWorkOrder(ctx echo.Context) error {
	workOrderID, err := parseUUIDParam(ctx, "id")
	if err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	actor, err := actorFromContext(ctx)
	if err != nil {
		return conveyorError(ctx, err)
	}
	var req conveyorWorkOrderCompleteRequest
	if err := ctx.Bind(&req); err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	var evidenceID *uuid.UUID
	if req.ResultEvidenceID != "" {
		parsed, err := uuid.Parse(req.ResultEvidenceID)
		if err != nil {
			return conveyorError(ctx, service.ErrValidation)
		}
		evidenceID = &parsed
	}
	result, err := c.svc.CompleteWorkOrder(ctx.Request().Context(), actor, workOrderID, service.CompleteWorkOrderRequest{ResultEvidenceID: evidenceID, EvidenceWaiver: req.EvidenceWaiver, IdempotencyKey: idempotencyKey(ctx, req.IdempotencyKey)})
	if err != nil {
		return conveyorError(ctx, err)
	}
	return ctx.JSON(http.StatusOK, result)
}

func (c *ConveyorController) workOrderReasonTransition(ctx echo.Context, fn func(service.ConveyorActor, uuid.UUID, conveyorWorkOrderReasonRequest) (*service.ConveyorMutationResult, error)) error {
	workOrderID, err := parseUUIDParam(ctx, "id")
	if err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	actor, err := actorFromContext(ctx)
	if err != nil {
		return conveyorError(ctx, err)
	}
	var req conveyorWorkOrderReasonRequest
	if err := ctx.Bind(&req); err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	result, err := fn(actor, workOrderID, req)
	if err != nil {
		return conveyorError(ctx, err)
	}
	return ctx.JSON(http.StatusOK, result)
}

// RegisterAgentRun godoc
// @Summary Register work item agent run
// @Tags conveyor
// @Accept json
// @Produce json
// @Param id path string true "Work item ID"
// @Param input body conveyorAgentRunRequest true "Agent run data"
// @Success 201 {object} service.ConveyorMutationResult
// @Failure 400 {object} conveyorErrorResponse
// @Failure 403 {object} conveyorErrorResponse
// @Failure 409 {object} conveyorErrorResponse
// @Router /api/work-items/{id}/agent-runs [post]
// @Security BearerAuth
func (c *ConveyorController) RegisterAgentRun(ctx echo.Context) error {
	taskID, err := parseUUIDParam(ctx, "id")
	if err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	actor, err := actorFromContext(ctx)
	if err != nil {
		return conveyorError(ctx, err)
	}
	var req conveyorAgentRunRequest
	if err := ctx.Bind(&req); err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	result, err := c.svc.RegisterAgentRun(ctx.Request().Context(), actor, service.RegisterAgentRunRequest{WorkItemID: taskID, Source: req.Source, Harness: req.Harness, Status: req.Status, Summary: req.Summary, LogURI: req.LogURI, WorkspaceURI: req.WorkspaceURI, Metadata: []byte(req.Metadata), IdempotencyKey: idempotencyKey(ctx, req.IdempotencyKey)})
	if err != nil {
		return conveyorError(ctx, err)
	}
	return ctx.JSON(http.StatusCreated, result)
}

// UpdateAgentRun godoc
// @Summary Update work item agent run
// @Tags conveyor
// @Accept json
// @Produce json
// @Param id path string true "Work item ID"
// @Param agent_run_id path string true "Agent run ID"
// @Param input body conveyorAgentRunUpdateRequest true "Agent run update"
// @Success 200 {object} service.ConveyorMutationResult
// @Failure 400 {object} conveyorErrorResponse
// @Failure 403 {object} conveyorErrorResponse
// @Failure 404 {object} conveyorErrorResponse
// @Failure 409 {object} conveyorErrorResponse
// @Router /api/work-items/{id}/agent-runs/{agent_run_id} [patch]
// @Security BearerAuth
func (c *ConveyorController) UpdateAgentRun(ctx echo.Context) error {
	taskID, err := parseUUIDParam(ctx, "id")
	if err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	runID, err := parseUUIDParam(ctx, "agent_run_id")
	if err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	actor, err := actorFromContext(ctx)
	if err != nil {
		return conveyorError(ctx, err)
	}
	var req conveyorAgentRunUpdateRequest
	if err := ctx.Bind(&req); err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	result, err := c.svc.UpdateAgentRun(ctx.Request().Context(), actor, taskID, runID, service.UpdateAgentRunRequest{Status: req.Status, Summary: req.Summary, LogURI: req.LogURI, WorkspaceURI: req.WorkspaceURI, Metadata: []byte(req.Metadata), IdempotencyKey: idempotencyKey(ctx, req.IdempotencyKey)})
	if err != nil {
		return conveyorError(ctx, err)
	}
	return ctx.JSON(http.StatusOK, result)
}

// HeartbeatAgentRun godoc
// @Summary Heartbeat work item agent run
// @Tags conveyor
// @Produce json
// @Param id path string true "Work item ID"
// @Param agent_run_id path string true "Agent run ID"
// @Success 200 {object} service.ConveyorMutationResult
// @Failure 400 {object} conveyorErrorResponse
// @Failure 403 {object} conveyorErrorResponse
// @Failure 404 {object} conveyorErrorResponse
// @Router /api/work-items/{id}/agent-runs/{agent_run_id}/heartbeat [post]
// @Security BearerAuth
func (c *ConveyorController) HeartbeatAgentRun(ctx echo.Context) error {
	taskID, err := parseUUIDParam(ctx, "id")
	if err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	runID, err := parseUUIDParam(ctx, "agent_run_id")
	if err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	actor, err := actorFromContext(ctx)
	if err != nil {
		return conveyorError(ctx, err)
	}
	result, err := c.svc.HeartbeatAgentRun(ctx.Request().Context(), actor, taskID, runID)
	if err != nil {
		return conveyorError(ctx, err)
	}
	return ctx.JSON(http.StatusOK, result)
}

func (c *ConveyorController) CreateGeneratedReport(ctx echo.Context) error {
	projectID, err := parseUUIDParam(ctx, "project_id")
	if err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	actor, err := actorFromContext(ctx)
	if err != nil {
		return conveyorError(ctx, err)
	}
	var req conveyorGeneratedReportRequest
	if err := ctx.Bind(&req); err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	report, err := c.svc.GenerateProjectReport(ctx.Request().Context(), actor, service.GenerateProjectReportRequest{ProjectID: projectID, PeriodStart: req.PeriodStart, PeriodEnd: req.PeriodEnd, IncludeLLM: req.IncludeLLM, IdempotencyKey: idempotencyKey(ctx, req.IdempotencyKey)})
	if err != nil {
		return conveyorError(ctx, err)
	}
	return ctx.JSON(http.StatusCreated, report)
}

func (c *ConveyorController) GetGeneratedReport(ctx echo.Context) error {
	reportID, err := parseUUIDParam(ctx, "id")
	if err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	report, err := c.svc.GetGeneratedReport(ctx.Request().Context(), reportID)
	if err != nil {
		return conveyorError(ctx, err)
	}
	return ctx.JSON(http.StatusOK, report)
}

func (c *ConveyorController) ExportGeneratedReportMarkdown(ctx echo.Context) error {
	reportID, err := parseUUIDParam(ctx, "id")
	if err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	markdown, err := c.svc.ExportGeneratedReportMarkdown(ctx.Request().Context(), reportID)
	if err != nil {
		return conveyorError(ctx, err)
	}
	ctx.Response().Header().Set(echo.HeaderContentType, "text/markdown; charset=UTF-8")
	return ctx.String(http.StatusOK, markdown)
}

func (c *ConveyorController) CreateForumDigest(ctx echo.Context) error {
	actor, err := actorFromContext(ctx)
	if err != nil {
		return conveyorError(ctx, err)
	}
	var req conveyorForumDigestRequest
	if err := ctx.Bind(&req); err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	digest, err := c.svc.CreateForumDigest(ctx.Request().Context(), actor, service.CreateForumDigestRequest{SourceType: req.SourceType, SourceID: req.SourceID, SourceTitle: req.SourceTitle, SourceLocator: req.SourceLocator, PeriodStart: req.PeriodStart, PeriodEnd: req.PeriodEnd, Summary: req.Summary, Decisions: req.Decisions, SourceMetadata: req.SourceMetadata, Messages: req.Messages, CandidateInputs: req.CandidateInputs, UseLLM: req.UseLLM})
	if err != nil {
		return conveyorError(ctx, err)
	}
	return ctx.JSON(http.StatusCreated, digest)
}

func (c *ConveyorController) GetForumDigest(ctx echo.Context) error {
	digestID, err := parseUUIDParam(ctx, "id")
	if err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	digest, err := c.svc.GetForumDigest(ctx.Request().Context(), digestID)
	if err != nil {
		return conveyorError(ctx, err)
	}
	return ctx.JSON(http.StatusOK, digest)
}

func (c *ConveyorController) ListForumDigests(ctx echo.Context) error {
	digests, err := c.svc.ListForumDigests(ctx.Request().Context(), ctx.QueryParam("source_type"), ctx.QueryParam("source_id"))
	if err != nil {
		return conveyorError(ctx, err)
	}
	return ctx.JSON(http.StatusOK, digests)
}

func (c *ConveyorController) ConfirmForumActionCandidate(ctx echo.Context) error {
	candidateID, err := parseUUIDParam(ctx, "id")
	if err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	actor, err := actorFromContext(ctx)
	if err != nil {
		return conveyorError(ctx, err)
	}
	var req conveyorForumCandidateConfirmRequest
	if err := ctx.Bind(&req); err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	statusID, err := uuid.Parse(req.StatusID)
	if err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	var assignedTo *uuid.UUID
	if req.AssignedTo != "" {
		parsed, err := uuid.Parse(req.AssignedTo)
		if err != nil {
			return conveyorError(ctx, service.ErrValidation)
		}
		assignedTo = &parsed
	}
	result, err := c.svc.ConfirmForumActionCandidate(ctx.Request().Context(), actor, candidateID, service.ConfirmForumActionCandidateRequest{StatusID: statusID, AssignedTo: assignedTo, Priority: req.Priority, IdempotencyKey: idempotencyKey(ctx, req.IdempotencyKey)})
	if err != nil {
		return conveyorError(ctx, err)
	}
	return ctx.JSON(http.StatusOK, result)
}

func (c *ConveyorController) RejectForumActionCandidate(ctx echo.Context) error {
	candidateID, err := parseUUIDParam(ctx, "id")
	if err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	actor, err := actorFromContext(ctx)
	if err != nil {
		return conveyorError(ctx, err)
	}
	var req conveyorForumCandidateRejectRequest
	if err := ctx.Bind(&req); err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	result, err := c.svc.RejectForumActionCandidate(ctx.Request().Context(), actor, candidateID, service.RejectForumActionCandidateRequest{Reason: req.Reason, IdempotencyKey: idempotencyKey(ctx, req.IdempotencyKey)})
	if err != nil {
		return conveyorError(ctx, err)
	}
	return ctx.JSON(http.StatusOK, result)
}

func actorFromContext(ctx echo.Context) (service.ConveyorActor, error) {
	userID, err := getUserIDFromContext(ctx)
	if err != nil || userID == uuid.Nil {
		return service.ConveyorActor{}, models.ErrConveyorPermissionDenied
	}
	return service.ConveyorActor{ActorType: "user", ActorID: userID, Source: "rest", RequestID: headerPtr(ctx, echo.HeaderXRequestID), CorrelationID: headerPtr(ctx, "X-Correlation-ID")}, nil
}

func parseUUIDParam(ctx echo.Context, name string) (uuid.UUID, error) {
	return uuid.Parse(ctx.Param(name))
}

func idempotencyKey(ctx echo.Context, bodyValue string) string {
	if header := ctx.Request().Header.Get("Idempotency-Key"); header != "" {
		return header
	}
	return bodyValue
}

func pmImportRootPath(scope string) (string, error) {
	scope = strings.TrimSpace(scope)
	if scope == "" || filepath.IsAbs(scope) || strings.Contains(scope, "..") || strings.ContainsAny(scope, `/\\`) {
		return "", service.ErrValidation
	}
	base := strings.TrimSpace(os.Getenv("PM_CANON_ROOT"))
	if base == "" {
		base = filepath.Join(".pm", "scopes")
	}
	return filepath.Join(base, scope), nil
}

func parsePMImportStatusMap(input map[string]string) (map[string]uuid.UUID, error) {
	if len(input) == 0 {
		return nil, service.ErrValidation
	}
	output := make(map[string]uuid.UUID, len(input))
	for state, rawID := range input {
		state = strings.TrimSpace(state)
		if state == "" {
			return nil, service.ErrValidation
		}
		id, err := uuid.Parse(strings.TrimSpace(rawID))
		if err != nil {
			return nil, service.ErrValidation
		}
		output[state] = id
	}
	return output, nil
}

func headerPtr(ctx echo.Context, name string) *string {
	value := ctx.Request().Header.Get(name)
	if value == "" {
		return nil
	}
	return &value
}

func optionalUUID(value string) (*uuid.UUID, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, nil
	}
	parsed, err := uuid.Parse(trimmed)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func optionalTime(value string) (*time.Time, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, trimmed)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

// CreateWaiver godoc
// @Summary Create a waiver for a work item
// @Tags conveyor
// @Accept json
// @Produce json
// @Param id path string true "Work item ID"
// @Param input body conveyorWaiverRequest true "Waiver data"
// @Success 201 {object} service.ConveyorMutationResult
// @Failure 400 {object} conveyorErrorResponse
// @Failure 403 {object} conveyorErrorResponse
// @Failure 404 {object} conveyorErrorResponse
// @Router /api/work-items/{id}/waivers [post]
// @Security BearerAuth
func (c *ConveyorController) CreateWaiver(ctx echo.Context) error {
	taskID, err := parseUUIDParam(ctx, "id")
	if err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	actor, err := actorFromContext(ctx)
	if err != nil {
		return conveyorError(ctx, err)
	}
	if err := c.svc.AuthorizeWorkItemAccess(ctx.Request().Context(), actor, taskID); err != nil {
		return conveyorError(ctx, err)
	}
	var req conveyorWaiverRequest
	if err := ctx.Bind(&req); err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	criterionID, err := optionalUUID(req.CriterionID)
	if err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	approvalID, err := optionalUUID(req.ApprovalRequestID)
	if err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	expiresAt, err := optionalTime(req.ExpiresAt)
	if err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	result, err := c.svc.CreateWaiver(ctx.Request().Context(), actor, service.CreateWaiverRequest{Scope: req.Scope, WorkItemID: taskID, CriterionID: criterionID, Reason: req.Reason, ApprovalRequestID: approvalID, ExpiresAt: expiresAt, IdempotencyKey: idempotencyKey(ctx, req.IdempotencyKey)})
	if err != nil {
		return conveyorError(ctx, err)
	}
	return ctx.JSON(http.StatusCreated, result)
}

// GetWaiver godoc
// @Summary Get a waiver
// @Tags conveyor
// @Produce json
// @Param id path string true "Waiver ID"
// @Success 200 {object} models.Waiver
// @Failure 404 {object} conveyorErrorResponse
// @Router /api/waivers/{id} [get]
// @Security BearerAuth
func (c *ConveyorController) GetWaiver(ctx echo.Context) error {
	id, err := parseUUIDParam(ctx, "id")
	if err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	waiver, err := c.svc.GetWaiver(ctx.Request().Context(), id)
	if err != nil {
		return conveyorError(ctx, err)
	}
	return ctx.JSON(http.StatusOK, waiver)
}

// RequestApproval godoc
// @Summary Request approval for a dangerous action
// @Tags conveyor
// @Accept json
// @Produce json
// @Param input body conveyorApprovalRequest true "Approval request"
// @Success 201 {object} service.ConveyorMutationResult
// @Failure 400 {object} conveyorErrorResponse
// @Failure 404 {object} conveyorErrorResponse
// @Router /api/approval-requests [post]
// @Security BearerAuth
func (c *ConveyorController) RequestApproval(ctx echo.Context) error {
	actor, err := actorFromContext(ctx)
	if err != nil {
		return conveyorError(ctx, err)
	}
	var req conveyorApprovalRequest
	if err := ctx.Bind(&req); err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	workItemID, err := optionalUUID(req.WorkItemID)
	if err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	expiresAt, err := optionalTime(req.ExpiresAt)
	if err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	result, err := c.svc.RequestApproval(ctx.Request().Context(), actor, service.RequestApprovalRequest{WorkItemID: workItemID, Action: req.Action, RiskLevel: req.RiskLevel, Reason: req.Reason, Resource: req.Resource, ExpiresAt: expiresAt, IdempotencyKey: idempotencyKey(ctx, req.IdempotencyKey)})
	if err != nil {
		return conveyorError(ctx, err)
	}
	return ctx.JSON(http.StatusCreated, result)
}

// GetApprovalRequest godoc
// @Summary Get an approval request
// @Tags conveyor
// @Produce json
// @Param id path string true "Approval request ID"
// @Success 200 {object} models.ApprovalRequest
// @Failure 404 {object} conveyorErrorResponse
// @Router /api/approval-requests/{id} [get]
// @Security BearerAuth
func (c *ConveyorController) GetApprovalRequest(ctx echo.Context) error {
	id, err := parseUUIDParam(ctx, "id")
	if err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	approval, err := c.svc.GetApprovalRequest(ctx.Request().Context(), id)
	if err != nil {
		return conveyorError(ctx, err)
	}
	return ctx.JSON(http.StatusOK, approval)
}

// GrantApproval godoc
// @Summary Grant an approval request
// @Tags conveyor
// @Accept json
// @Produce json
// @Param id path string true "Approval request ID"
// @Param input body conveyorApprovalDecisionRequest true "Decision"
// @Success 200 {object} service.ConveyorMutationResult
// @Failure 400 {object} conveyorErrorResponse
// @Failure 404 {object} conveyorErrorResponse
// @Failure 409 {object} conveyorErrorResponse
// @Router /api/approval-requests/{id}/grant [post]
// @Security BearerAuth
func (c *ConveyorController) GrantApproval(ctx echo.Context) error {
	return c.decideApproval(ctx, true)
}

// DenyApproval godoc
// @Summary Deny an approval request
// @Tags conveyor
// @Accept json
// @Produce json
// @Param id path string true "Approval request ID"
// @Param input body conveyorApprovalDecisionRequest true "Decision"
// @Success 200 {object} service.ConveyorMutationResult
// @Failure 400 {object} conveyorErrorResponse
// @Failure 404 {object} conveyorErrorResponse
// @Failure 409 {object} conveyorErrorResponse
// @Router /api/approval-requests/{id}/deny [post]
// @Security BearerAuth
func (c *ConveyorController) DenyApproval(ctx echo.Context) error {
	return c.decideApproval(ctx, false)
}

func (c *ConveyorController) decideApproval(ctx echo.Context, grant bool) error {
	id, err := parseUUIDParam(ctx, "id")
	if err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	actor, err := actorFromContext(ctx)
	if err != nil {
		return conveyorError(ctx, err)
	}
	var req conveyorApprovalDecisionRequest
	if err := ctx.Bind(&req); err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	decision := service.DecideApprovalRequest{Reason: req.Reason, IdempotencyKey: idempotencyKey(ctx, req.IdempotencyKey)}
	var result *service.ConveyorMutationResult
	if grant {
		result, err = c.svc.GrantApproval(ctx.Request().Context(), actor, id, decision)
	} else {
		result, err = c.svc.DenyApproval(ctx.Request().Context(), actor, id, decision)
	}
	if err != nil {
		return conveyorError(ctx, err)
	}
	return ctx.JSON(http.StatusOK, result)
}

// ListApprovalRequests godoc
// @Summary List approval requests for a work item
// @Tags conveyor
// @Produce json
// @Param id path string true "Work item ID"
// @Success 200 {array} models.ApprovalRequest
// @Failure 400 {object} conveyorErrorResponse
// @Router /api/work-items/{id}/approval-requests [get]
// @Security BearerAuth
func (c *ConveyorController) ListApprovalRequests(ctx echo.Context) error {
	taskID, err := parseUUIDParam(ctx, "id")
	if err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	actor, err := actorFromContext(ctx)
	if err != nil {
		return conveyorError(ctx, err)
	}
	if err := c.svc.AuthorizeWorkItemAccess(ctx.Request().Context(), actor, taskID); err != nil {
		return conveyorError(ctx, err)
	}
	approvals, err := c.svc.ListApprovalRequests(ctx.Request().Context(), taskID)
	if err != nil {
		return conveyorError(ctx, err)
	}
	return ctx.JSON(http.StatusOK, approvals)
}

// ListPendingApprovals godoc
// @Summary List all pending approval requests across the platform (admin queue)
// @Tags conveyor
// @Produce json
// @Param limit query int false "Max items (default 200)"
// @Success 200 {array} models.ApprovalRequest
// @Failure 403 {object} conveyorErrorResponse
// @Router /api/conveyor/pending-approvals [get]
// @Security BearerAuth
func (c *ConveyorController) ListPendingApprovals(ctx echo.Context) error {
	limit := 200
	if raw := ctx.QueryParam("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	items, err := c.svc.ListPendingApprovals(ctx.Request().Context(), limit)
	if err != nil {
		return conveyorError(ctx, err)
	}
	return ctx.JSON(http.StatusOK, items)
}

type conveyorLLMTextResponse struct {
	Text string `json:"text" example:"draft text"`
}

// SuggestCriteria godoc
// @Summary Suggest acceptance criteria for a work item via LLM (draft)
// @Tags conveyor
// @Produce json
// @Param id path string true "Work item ID"
// @Success 200 {object} conveyorLLMTextResponse
// @Failure 400 {object} conveyorErrorResponse
// @Failure 403 {object} conveyorErrorResponse
// @Failure 404 {object} conveyorErrorResponse
// @Router /api/work-items/{id}/suggest-criteria [post]
// @Security BearerAuth
func (c *ConveyorController) SuggestCriteria(ctx echo.Context) error {
	taskID, err := parseUUIDParam(ctx, "id")
	if err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	actor, err := actorFromContext(ctx)
	if err != nil {
		return conveyorError(ctx, err)
	}
	if err := c.svc.AuthorizeWorkItemAccess(ctx.Request().Context(), actor, taskID); err != nil {
		return conveyorError(ctx, err)
	}
	text, err := c.svc.SuggestCriteria(ctx.Request().Context(), actor, taskID)
	if err != nil {
		return conveyorError(ctx, err)
	}
	return ctx.JSON(http.StatusOK, conveyorLLMTextResponse{Text: text})
}

// SummarizeEvidence godoc
// @Summary Summarize a work item's evidence via LLM (draft)
// @Tags conveyor
// @Produce json
// @Param id path string true "Work item ID"
// @Success 200 {object} conveyorLLMTextResponse
// @Failure 400 {object} conveyorErrorResponse
// @Failure 403 {object} conveyorErrorResponse
// @Failure 404 {object} conveyorErrorResponse
// @Router /api/work-items/{id}/evidence-summary [post]
// @Security BearerAuth
func (c *ConveyorController) SummarizeEvidence(ctx echo.Context) error {
	taskID, err := parseUUIDParam(ctx, "id")
	if err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	actor, err := actorFromContext(ctx)
	if err != nil {
		return conveyorError(ctx, err)
	}
	if err := c.svc.AuthorizeWorkItemAccess(ctx.Request().Context(), actor, taskID); err != nil {
		return conveyorError(ctx, err)
	}
	text, err := c.svc.SummarizeEvidence(ctx.Request().Context(), actor, taskID)
	if err != nil {
		return conveyorError(ctx, err)
	}
	return ctx.JSON(http.StatusOK, conveyorLLMTextResponse{Text: text})
}

func conveyorError(ctx echo.Context, err error) error {
	switch {
	case errors.Is(err, service.ErrNotFound):
		return ctx.JSON(http.StatusNotFound, conveyorErrorResponse{Error: "not_found"})
	case errors.Is(err, service.ErrConflict):
		return ctx.JSON(http.StatusConflict, conveyorErrorResponse{Error: "conflict"})
	case errors.Is(err, service.ErrApprovalRequired):
		return ctx.JSON(http.StatusConflict, conveyorErrorResponse{Error: "approval_required"})
	case errors.Is(err, service.ErrPermissionDenied):
		return ctx.JSON(http.StatusForbidden, conveyorErrorResponse{Error: "permission_denied"})
	case errors.Is(err, service.ErrValidation):
		return ctx.JSON(http.StatusBadRequest, conveyorErrorResponse{Error: "validation_error"})
	default:
		return ctx.JSON(http.StatusServiceUnavailable, conveyorErrorResponse{Error: "dependency_unavailable"})
	}
}
