package models

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrConveyorValidation       = errors.New("validation_error")
	ErrConveyorNotFound         = errors.New("not_found")
	ErrConveyorConflict         = errors.New("conflict")
	ErrConveyorApprovalRequired = errors.New("approval_required")
	ErrConveyorPermissionDenied = errors.New("permission_denied")
)

const (
	AcceptanceCriterionStateUnchecked = "unchecked"
	AcceptanceCriterionStatePassed    = "passed"
	AcceptanceCriterionStateFailed    = "failed"
	AcceptanceCriterionStateWaived    = "waived"

	EvidenceTypeLink           = "link"
	EvidenceTypeFile           = "file"
	EvidenceTypeLog            = "log"
	EvidenceTypeMarkdownReport = "markdown_report"
	EvidenceTypePR             = "pr"
	EvidenceTypeCommit         = "commit"
	EvidenceTypeScreenshot     = "screenshot"
	EvidenceTypeHealthcheck    = "healthcheck"
	EvidenceTypeDeploymentLog  = "deployment_log"

	EvidenceVerdictSupports      = "supports"
	EvidenceVerdictContradicts   = "contradicts"
	EvidenceVerdictInformational = "informational"

	TaskLinkTypeBlocks     = "blocks"
	TaskLinkTypeBlockedBy  = "blocked_by"
	TaskLinkTypeRelatesTo  = "relates_to"
	TaskLinkTypeDuplicates = "duplicates"
	TaskLinkTypeParent     = "parent"
	TaskLinkTypeChild      = "child"

	AgentRunStatusQueued     = "queued"
	AgentRunStatusRunning    = "running"
	AgentRunStatusSucceeded  = "succeeded"
	AgentRunStatusFailed     = "failed"
	AgentRunStatusCanceled   = "canceled"
	AgentRunStatusNeedsHuman = "needs_human"

	WorkOrderStatusRequested  = "requested"
	WorkOrderStatusAccepted   = "accepted"
	WorkOrderStatusRejected   = "rejected"
	WorkOrderStatusInProgress = "in_progress"
	WorkOrderStatusReview     = "review"
	WorkOrderStatusCompleted  = "completed"
	WorkOrderStatusFailed     = "failed"
	WorkOrderStatusCanceled   = "canceled"

	ConversationSourceTypeForum    = "forum"
	ConversationSourceTypeTelegram = "telegram"

	ForumActionCandidateStatusPending   = "pending"
	ForumActionCandidateStatusConfirmed = "confirmed"
	ForumActionCandidateStatusRejected  = "rejected"

	WaiverScopeTask      = "task"
	WaiverScopeCriterion = "criterion"

	ApprovalStatusPending = "pending"
	ApprovalStatusGranted = "granted"
	ApprovalStatusDenied  = "denied"

	ApprovalActionCloseWithWaiver  = "close_with_waiver"
	ApprovalActionProductionDeploy = "production_deploy"
	ApprovalActionSecretAccess     = "secret_access"
	ApprovalActionRoleChange       = "role_change"
	ApprovalActionEvidenceRevoke   = "evidence_revocation"
	ApprovalActionWorkOrderAccept  = "work_order_accept"
	ApprovalActionOther            = "other"
)

type AcceptanceCriterion struct {
	ID        uuid.UUID       `gorm:"type:uuid;primaryKey" json:"id"`
	TaskID    uuid.UUID       `gorm:"type:uuid;not null;index" json:"task_id"`
	ACID      string          `gorm:"size:100;index" json:"ac_id"`
	Title     string          `gorm:"type:text;not null" json:"title"`
	SpecIDs   json.RawMessage `gorm:"type:jsonb" json:"spec_ids,omitempty"`
	Required  bool            `gorm:"not null;default:true" json:"required"`
	State     string          `gorm:"size:32;not null;default:unchecked;index" json:"state"`
	CreatedBy uuid.UUID       `gorm:"type:uuid;not null;index" json:"created_by"`
	CreatedAt time.Time       `gorm:"type:timestamp;not null" json:"created_at"`
	UpdatedAt time.Time       `gorm:"type:timestamp;not null" json:"updated_at"`
}

type Evidence struct {
	ID            uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	TaskID        uuid.UUID  `gorm:"type:uuid;not null;index" json:"task_id"`
	CriterionID   *uuid.UUID `gorm:"type:uuid;index" json:"criterion_id,omitempty"`
	Type          string     `gorm:"size:64;not null;index" json:"type"`
	Verdict       string     `gorm:"size:32;not null;index" json:"verdict"`
	URI           string     `gorm:"type:text" json:"uri"`
	Title         string     `gorm:"type:text" json:"title"`
	SHA256        string     `gorm:"size:64;index" json:"sha256,omitempty"`
	Metadata      []byte     `gorm:"type:jsonb" json:"metadata,omitempty"`
	CreatedBy     uuid.UUID  `gorm:"type:uuid;not null;index" json:"created_by"`
	CreatedAt     time.Time  `gorm:"type:timestamp;not null" json:"created_at"`
	Revoked       bool       `gorm:"not null;default:false;index" json:"revoked"`
	RevokedBy     *uuid.UUID `gorm:"type:uuid" json:"revoked_by,omitempty"`
	RevokedAt     *time.Time `gorm:"type:timestamp" json:"revoked_at,omitempty"`
	RevokedReason *string    `gorm:"type:text" json:"revoked_reason,omitempty"`
}

type ConveyorEvent struct {
	ID               uuid.UUID       `gorm:"type:uuid;primaryKey" json:"id"`
	WorkItemID       uuid.UUID       `gorm:"type:uuid;not null;index" json:"work_item_id"`
	Type             string          `gorm:"size:100;not null;index" json:"type"`
	Timestamp        time.Time       `gorm:"type:timestamp;not null;index" json:"timestamp"`
	ActorType        string          `gorm:"size:32;not null" json:"actor_type"`
	ActorID          uuid.UUID       `gorm:"type:uuid;not null;index" json:"actor_id"`
	AgentID          *string         `gorm:"size:100" json:"agent_id,omitempty"`
	OnBehalfOfUserID *uuid.UUID      `gorm:"type:uuid" json:"on_behalf_of_user_id,omitempty"`
	APITokenID       *uuid.UUID      `gorm:"type:uuid" json:"api_token_id,omitempty"`
	Payload          json.RawMessage `gorm:"type:jsonb" json:"payload"`
	SchemaVersion    int             `gorm:"not null;default:1" json:"schema_version"`
	IdempotencyKey   *string         `gorm:"size:200;index" json:"idempotency_key,omitempty"`
	CorrelationID    *string         `gorm:"size:100;index" json:"correlation_id,omitempty"`
	CausationID      *uuid.UUID      `gorm:"type:uuid" json:"causation_id,omitempty"`
	RequestID        *string         `gorm:"size:100;index" json:"request_id,omitempty"`
	Source           string          `gorm:"size:100" json:"source"`
}

type TaskLink struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	SourceTaskID uuid.UUID `gorm:"type:uuid;not null;index:idx_task_links_unique,unique" json:"source_task_id"`
	TargetTaskID uuid.UUID `gorm:"type:uuid;not null;index:idx_task_links_unique,unique" json:"target_task_id"`
	LinkType     string    `gorm:"size:32;not null;index:idx_task_links_unique,unique" json:"link_type"`
	CreatedBy    uuid.UUID `gorm:"type:uuid;not null" json:"created_by"`
	CreatedAt    time.Time `gorm:"type:timestamp;not null" json:"created_at"`
}

type WorkOrder struct {
	ID                       uuid.UUID       `gorm:"type:uuid;primaryKey" json:"id"`
	SourceTaskID             uuid.UUID       `gorm:"type:uuid;not null;index" json:"source_task_id"`
	TargetTaskID             *uuid.UUID      `gorm:"type:uuid;index" json:"target_task_id,omitempty"`
	ProviderBoardID          uuid.UUID       `gorm:"type:uuid;not null;index" json:"provider_board_id"`
	ProviderStatusID         uuid.UUID       `gorm:"type:uuid;not null;index" json:"provider_status_id"`
	Goal                     string          `gorm:"type:text;not null" json:"goal"`
	Inputs                   json.RawMessage `gorm:"type:jsonb" json:"inputs,omitempty"`
	AcceptanceCriteria       json.RawMessage `gorm:"type:jsonb" json:"acceptance_criteria,omitempty"`
	RequiredEvidenceMetadata json.RawMessage `gorm:"type:jsonb" json:"required_evidence_metadata,omitempty"`
	RequesterContext         json.RawMessage `gorm:"type:jsonb" json:"requester_context,omitempty"`
	ProviderContext          json.RawMessage `gorm:"type:jsonb" json:"provider_context,omitempty"`
	Status                   string          `gorm:"size:32;not null;index" json:"status"`
	RejectReason             string          `gorm:"type:text" json:"reject_reason,omitempty"`
	CancelReason             string          `gorm:"type:text" json:"cancel_reason,omitempty"`
	FailureReason            string          `gorm:"type:text" json:"failure_reason,omitempty"`
	ResultEvidenceID         *uuid.UUID      `gorm:"type:uuid;index" json:"result_evidence_id,omitempty"`
	EvidenceWaiver           string          `gorm:"type:text" json:"evidence_waiver,omitempty"`
	RequestedBy              uuid.UUID       `gorm:"type:uuid;not null;index" json:"requested_by"`
	ProviderActorID          *uuid.UUID      `gorm:"type:uuid;index" json:"provider_actor_id,omitempty"`
	CreatedAt                time.Time       `gorm:"type:timestamp;not null" json:"created_at"`
	UpdatedAt                time.Time       `gorm:"type:timestamp;not null" json:"updated_at"`
	AcceptedAt               *time.Time      `gorm:"type:timestamp" json:"accepted_at,omitempty"`
	CompletedAt              *time.Time      `gorm:"type:timestamp" json:"completed_at,omitempty"`
}

type AgentRun struct {
	ID               uuid.UUID       `gorm:"type:uuid;primaryKey" json:"id"`
	WorkItemID       uuid.UUID       `gorm:"type:uuid;not null;index" json:"work_item_id"`
	Source           string          `gorm:"size:100;not null;index" json:"source"`
	Harness          string          `gorm:"size:100;not null;index" json:"harness"`
	Status           string          `gorm:"size:32;not null;index" json:"status"`
	Summary          string          `gorm:"type:text" json:"summary"`
	LogURI           string          `gorm:"type:text" json:"log_uri"`
	WorkspaceURI     string          `gorm:"type:text" json:"workspace_uri"`
	Metadata         json.RawMessage `gorm:"type:jsonb" json:"metadata,omitempty"`
	CreatedBy        uuid.UUID       `gorm:"type:uuid;not null;index" json:"created_by"`
	ActorType        string          `gorm:"size:32;not null" json:"actor_type,omitempty"`
	AgentID          *string         `gorm:"size:100" json:"agent_id,omitempty"`
	OnBehalfOfUserID *uuid.UUID      `gorm:"type:uuid" json:"on_behalf_of_user_id,omitempty"`
	APITokenID       *uuid.UUID      `gorm:"type:uuid" json:"api_token_id,omitempty"`
	StartedAt        *time.Time      `gorm:"type:timestamp" json:"started_at,omitempty"`
	HeartbeatAt      *time.Time      `gorm:"type:timestamp" json:"heartbeat_at,omitempty"`
	FinishedAt       *time.Time      `gorm:"type:timestamp" json:"finished_at,omitempty"`
	CreatedAt        time.Time       `gorm:"type:timestamp;not null" json:"created_at"`
	UpdatedAt        time.Time       `gorm:"type:timestamp;not null" json:"updated_at"`
}

type IdempotencyRecord struct {
	ID             uuid.UUID       `gorm:"type:uuid;primaryKey" json:"id"`
	ActorID        uuid.UUID       `gorm:"type:uuid;not null;index:idx_idempotency_unique,unique" json:"actor_id"`
	Operation      string          `gorm:"size:100;not null;index:idx_idempotency_unique,unique" json:"operation"`
	IdempotencyKey string          `gorm:"size:200;not null;index:idx_idempotency_unique,unique" json:"idempotency_key"`
	RequestHash    string          `gorm:"size:64;not null" json:"request_hash"`
	Result         json.RawMessage `gorm:"type:jsonb;not null" json:"result"`
	EventID        uuid.UUID       `gorm:"type:uuid;not null" json:"event_id"`
	CreatedAt      time.Time       `gorm:"type:timestamp;not null" json:"created_at"`
}

type GeneratedReport struct {
	ID                uuid.UUID       `gorm:"type:uuid;primaryKey" json:"id"`
	ProjectID         uuid.UUID       `gorm:"type:uuid;not null;index" json:"project_id"`
	PeriodStart       time.Time       `gorm:"type:timestamp;not null;index" json:"period_start"`
	PeriodEnd         time.Time       `gorm:"type:timestamp;not null;index" json:"period_end"`
	SourceEventIDs    json.RawMessage `gorm:"type:jsonb;not null" json:"source_event_ids"`
	SourceEvidenceIDs json.RawMessage `gorm:"type:jsonb;not null" json:"source_evidence_ids"`
	Facts             json.RawMessage `gorm:"type:jsonb;not null" json:"facts"`
	Conclusions       json.RawMessage `gorm:"type:jsonb;not null" json:"conclusions"`
	Risks             json.RawMessage `gorm:"type:jsonb;not null" json:"risks"`
	LLMDraft          json.RawMessage `gorm:"type:jsonb" json:"llm_draft,omitempty"`
	LLMError          string          `gorm:"type:text" json:"llm_error,omitempty"`
	CreatedBy         uuid.UUID       `gorm:"type:uuid;not null;index" json:"created_by"`
	ActorType         string          `gorm:"size:32;not null" json:"actor_type"`
	AgentID           *string         `gorm:"size:100" json:"agent_id,omitempty"`
	CreatedAt         time.Time       `gorm:"type:timestamp;not null" json:"created_at"`
	UpdatedAt         time.Time       `gorm:"type:timestamp;not null" json:"updated_at"`
}

type ForumDigest struct {
	ID             uuid.UUID       `gorm:"type:uuid;primaryKey" json:"id"`
	SourceType     string          `gorm:"size:32;not null;index" json:"source_type"`
	SourceID       string          `gorm:"type:text;not null;index" json:"source_id"`
	SourceTitle    string          `gorm:"type:text" json:"source_title"`
	SourceLocator  string          `gorm:"type:text" json:"source_locator"`
	PeriodStart    *time.Time      `gorm:"type:timestamp;index" json:"period_start,omitempty"`
	PeriodEnd      *time.Time      `gorm:"type:timestamp;index" json:"period_end,omitempty"`
	Summary        string          `gorm:"type:text" json:"summary"`
	Decisions      json.RawMessage `gorm:"type:jsonb" json:"decisions,omitempty"`
	SourceMessages json.RawMessage `gorm:"type:jsonb;not null" json:"source_messages"`
	SourceMetadata json.RawMessage `gorm:"type:jsonb" json:"source_metadata,omitempty"`
	CreatedBy      uuid.UUID       `gorm:"type:uuid;not null;index" json:"created_by"`
	ActorType      string          `gorm:"size:32;not null" json:"actor_type"`
	CreatedAt      time.Time       `gorm:"type:timestamp;not null" json:"created_at"`
	UpdatedAt      time.Time       `gorm:"type:timestamp;not null" json:"updated_at"`
}

type ForumActionCandidate struct {
	ID             uuid.UUID       `gorm:"type:uuid;primaryKey" json:"id"`
	DigestID       uuid.UUID       `gorm:"type:uuid;not null;index" json:"digest_id"`
	Title          string          `gorm:"type:text;not null" json:"title"`
	Description    string          `gorm:"type:text" json:"description"`
	SourceLocator  string          `gorm:"type:text" json:"source_locator"`
	SourceMetadata json.RawMessage `gorm:"type:jsonb" json:"source_metadata,omitempty"`
	Status         string          `gorm:"size:32;not null;index" json:"status"`
	WorkItemID     *uuid.UUID      `gorm:"type:uuid;index" json:"work_item_id,omitempty"`
	CreatedBy      uuid.UUID       `gorm:"type:uuid;not null;index" json:"created_by"`
	ConfirmedBy    *uuid.UUID      `gorm:"type:uuid;index" json:"confirmed_by,omitempty"`
	ConfirmedAt    *time.Time      `gorm:"type:timestamp" json:"confirmed_at,omitempty"`
	RejectedBy     *uuid.UUID      `gorm:"type:uuid;index" json:"rejected_by,omitempty"`
	RejectedAt     *time.Time      `gorm:"type:timestamp" json:"rejected_at,omitempty"`
	RejectReason   string          `gorm:"type:text" json:"reject_reason,omitempty"`
	CreatedAt      time.Time       `gorm:"type:timestamp;not null" json:"created_at"`
	UpdatedAt      time.Time       `gorm:"type:timestamp;not null" json:"updated_at"`
}

// Waiver is a justified exception that lets a task close with an unmet criterion
// or without supporting evidence (PRD 9.14). It is a first-class, auditable record:
// CloseTask resolves TaskWaiverID against this table instead of trusting a bare pointer.
type Waiver struct {
	ID                uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	Scope             string     `gorm:"size:32;not null;index" json:"scope"`
	WorkItemID        uuid.UUID  `gorm:"type:uuid;not null;index" json:"work_item_id"`
	CriterionID       *uuid.UUID `gorm:"type:uuid;index" json:"criterion_id,omitempty"`
	Reason            string     `gorm:"type:text;not null" json:"reason"`
	ApprovalRequestID *uuid.UUID `gorm:"type:uuid;index" json:"approval_request_id,omitempty"`
	CreatedBy         uuid.UUID  `gorm:"type:uuid;not null;index" json:"created_by"`
	ApprovedBy        *uuid.UUID `gorm:"type:uuid;index" json:"approved_by,omitempty"`
	ExpiresAt         *time.Time `gorm:"type:timestamp" json:"expires_at,omitempty"`
	CreatedAt         time.Time  `gorm:"type:timestamp;not null" json:"created_at"`
}

// ApprovalRequest is a persisted, auditable approval gate for dangerous actions
// (PRD 12.13 / SEC-03..06). request/grant/deny each emit an event, so every
// approval decision lands in the event log.
type ApprovalRequest struct {
	ID               uuid.UUID       `gorm:"type:uuid;primaryKey" json:"id"`
	WorkItemID       *uuid.UUID      `gorm:"type:uuid;index" json:"work_item_id,omitempty"`
	Action           string          `gorm:"size:64;not null;index" json:"action"`
	RiskLevel        string          `gorm:"size:32;not null" json:"risk_level"`
	Status           string          `gorm:"size:32;not null;index" json:"status"`
	Reason           string          `gorm:"type:text" json:"reason"`
	Resource         json.RawMessage `gorm:"type:jsonb" json:"resource,omitempty"`
	RequestedBy      uuid.UUID       `gorm:"type:uuid;not null;index" json:"requested_by"`
	ActorType        string          `gorm:"size:32;not null" json:"actor_type"`
	AgentID          *string         `gorm:"size:100" json:"agent_id,omitempty"`
	OnBehalfOfUserID *uuid.UUID      `gorm:"type:uuid" json:"on_behalf_of_user_id,omitempty"`
	APITokenID       *uuid.UUID      `gorm:"type:uuid" json:"api_token_id,omitempty"`
	DecidedBy        *uuid.UUID      `gorm:"type:uuid;index" json:"decided_by,omitempty"`
	DecisionReason   string          `gorm:"type:text" json:"decision_reason,omitempty"`
	ExpiresAt        *time.Time      `gorm:"type:timestamp" json:"expires_at,omitempty"`
	CreatedAt        time.Time       `gorm:"type:timestamp;not null" json:"created_at"`
	DecidedAt        *time.Time      `gorm:"type:timestamp" json:"decided_at,omitempty"`
}

func (Waiver) TableName() string          { return "waivers" }
func (ApprovalRequest) TableName() string { return "approval_requests" }

func (AcceptanceCriterion) TableName() string  { return "acceptance_criteria" }
func (Evidence) TableName() string             { return "evidence" }
func (ConveyorEvent) TableName() string        { return "conveyor_events" }
func (TaskLink) TableName() string             { return "task_links" }
func (WorkOrder) TableName() string            { return "work_orders" }
func (AgentRun) TableName() string             { return "agent_runs" }
func (IdempotencyRecord) TableName() string    { return "idempotency_records" }
func (GeneratedReport) TableName() string      { return "generated_reports" }
func (ForumDigest) TableName() string          { return "forum_digests" }
func (ForumActionCandidate) TableName() string { return "forum_action_candidates" }
