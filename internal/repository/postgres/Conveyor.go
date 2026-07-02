package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	models "emplacc-api/internal/domain"
	"emplacc-api/internal/ports"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func (r *conveyorRepository) CreateTask(ctx context.Context, task *models.Task) error {
	return r.db.WithContext(ctx).Create(task).Error
}

type conveyorRepository struct {
	db *gorm.DB
}

func NewConveyorRepository(db *gorm.DB) ports.ConveyorRepository {
	return &conveyorRepository{db: db}
}

func (r *conveyorRepository) WithTransaction(ctx context.Context, fn func(ports.ConveyorRepository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&conveyorRepository{db: tx})
	})
}

func (r *conveyorRepository) GetTask(ctx context.Context, id uuid.UUID) (*models.Task, error) {
	var task models.Task
	if err := r.db.WithContext(ctx).Where("id = ? AND deleted = ?", id, false).First(&task).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, models.ErrConveyorNotFound
		}
		return nil, err
	}
	return &task, nil
}

func (r *conveyorRepository) ActorCanAccessTask(ctx context.Context, actorID uuid.UUID, taskID uuid.UUID) (bool, error) {
	if actorID == uuid.Nil || taskID == uuid.Nil {
		return false, nil
	}
	var count int64
	err := r.db.WithContext(ctx).
		Model(&models.Task{}).
		Joins("LEFT JOIN statuses ON statuses.id = tasks.status_id AND statuses.deleted = ?", false).
		Joins("LEFT JOIN boards ON boards.id = statuses.board_id AND boards.deleted = ?", false).
		Joins("LEFT JOIN project_teams ON project_teams.project_id = boards.project_id AND project_teams.deleted = ?", false).
		Joins("LEFT JOIN team_members ON team_members.team_id = project_teams.team_id AND team_members.user_id = ? AND team_members.deleted = ?", actorID, false).
		Where(`tasks.id = ? AND tasks.deleted = ? AND (
			tasks.created_by = ?
			OR tasks.assigned_to = ?
			OR team_members.user_id IS NOT NULL
			OR EXISTS (
				SELECT 1 FROM user_roles ur
				JOIN roles r ON r.id = ur.role_id AND r.deleted = FALSE
				WHERE ur.user_id = ? AND ur.deleted = FALSE AND lower(r.name) IN ('admin', 'manager')
			)
		)`, taskID, false, actorID, actorID, actorID).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *conveyorRepository) GetStatus(ctx context.Context, id uuid.UUID) (*models.Status, error) {
	var status models.Status
	if err := r.db.WithContext(ctx).Where("id = ? AND deleted = ?", id, false).First(&status).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, models.ErrConveyorNotFound
		}
		return nil, err
	}
	return &status, nil
}

func (r *conveyorRepository) FindStatusByBoardName(ctx context.Context, boardID uuid.UUID, name string) (*models.Status, error) {
	var status models.Status
	if err := r.db.WithContext(ctx).Where("board_id = ? AND lower(name) = ? AND deleted = ?", boardID, strings.ToLower(name), false).First(&status).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, models.ErrConveyorNotFound
		}
		return nil, err
	}
	return &status, nil
}

func (r *conveyorRepository) UpdateTaskStatus(ctx context.Context, taskID uuid.UUID, statusID uuid.UUID, at time.Time) error {
	res := r.db.WithContext(ctx).Model(&models.Task{}).Where("id = ? AND deleted = ?", taskID, false).Updates(map[string]any{"status_id": statusID, "updated_at": at})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return models.ErrConveyorNotFound
	}
	return nil
}

func (r *conveyorRepository) CreateAcceptanceCriterion(ctx context.Context, criterion *models.AcceptanceCriterion) error {
	return r.db.WithContext(ctx).Create(criterion).Error
}

func (r *conveyorRepository) GetAcceptanceCriterion(ctx context.Context, id uuid.UUID) (*models.AcceptanceCriterion, error) {
	var criterion models.AcceptanceCriterion
	if err := r.db.WithContext(ctx).First(&criterion, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, models.ErrConveyorNotFound
		}
		return nil, err
	}
	return &criterion, nil
}

func (r *conveyorRepository) UpdateAcceptanceCriterionState(ctx context.Context, id uuid.UUID, state string, at time.Time) error {
	res := r.db.WithContext(ctx).Model(&models.AcceptanceCriterion{}).Where("id = ?", id).Updates(map[string]any{"state": state, "updated_at": at})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return models.ErrConveyorNotFound
	}
	return nil
}

func (r *conveyorRepository) ListAcceptanceCriteria(ctx context.Context, taskID uuid.UUID) ([]models.AcceptanceCriterion, error) {
	var criteria []models.AcceptanceCriterion
	err := r.db.WithContext(ctx).Where("task_id = ?", taskID).Order("created_at ASC").Find(&criteria).Error
	return criteria, err
}

func (r *conveyorRepository) CreateEvidence(ctx context.Context, evidence *models.Evidence) error {
	return r.db.WithContext(ctx).Create(evidence).Error
}

func (r *conveyorRepository) GetEvidence(ctx context.Context, id uuid.UUID) (*models.Evidence, error) {
	var evidence models.Evidence
	if err := r.db.WithContext(ctx).First(&evidence, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, models.ErrConveyorNotFound
		}
		return nil, err
	}
	return &evidence, nil
}

func (r *conveyorRepository) RevokeEvidence(ctx context.Context, id uuid.UUID, actorID uuid.UUID, reason string, at time.Time) error {
	res := r.db.WithContext(ctx).Model(&models.Evidence{}).Where("id = ? AND revoked = ?", id, false).Updates(map[string]any{"revoked": true, "revoked_by": actorID, "revoked_at": at, "revoked_reason": reason})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return models.ErrConveyorNotFound
	}
	return nil
}

func (r *conveyorRepository) ListEvidence(ctx context.Context, taskID uuid.UUID) ([]models.Evidence, error) {
	var evidence []models.Evidence
	err := r.db.WithContext(ctx).Where("task_id = ?", taskID).Order("created_at ASC").Find(&evidence).Error
	return evidence, err
}

func (r *conveyorRepository) CreateEvent(ctx context.Context, event *models.ConveyorEvent) error {
	return r.db.WithContext(ctx).Create(event).Error
}

func (r *conveyorRepository) ListEvents(ctx context.Context, taskID uuid.UUID) ([]models.ConveyorEvent, error) {
	var events []models.ConveyorEvent
	err := r.db.WithContext(ctx).Where("work_item_id = ?", taskID).Order("timestamp ASC").Find(&events).Error
	return events, err
}

func (r *conveyorRepository) GetEvent(ctx context.Context, id uuid.UUID) (*models.ConveyorEvent, error) {
	var event models.ConveyorEvent
	if err := r.db.WithContext(ctx).First(&event, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, models.ErrConveyorNotFound
		}
		return nil, err
	}
	return &event, nil
}

func (r *conveyorRepository) CreateTaskLink(ctx context.Context, link *models.TaskLink) error {
	if err := r.db.WithContext(ctx).Create(link).Error; err != nil {
		return err
	}
	return nil
}

func (r *conveyorRepository) TaskLinkExists(ctx context.Context, sourceID uuid.UUID, targetID uuid.UUID, linkType string) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&models.TaskLink{}).Where("source_task_id = ? AND target_task_id = ? AND link_type = ?", sourceID, targetID, linkType).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *conveyorRepository) ListTaskLinks(ctx context.Context, taskID uuid.UUID) ([]models.TaskLink, error) {
	var links []models.TaskLink
	err := r.db.WithContext(ctx).Where("source_task_id = ? OR target_task_id = ?", taskID, taskID).Order("created_at ASC").Find(&links).Error
	return links, err
}

func (r *conveyorRepository) ListDownstreamDependencies(ctx context.Context, upstreamID uuid.UUID) ([]models.TaskLink, error) {
	var links []models.TaskLink
	err := r.db.WithContext(ctx).
		Where("(source_task_id = ? AND link_type = ?) OR (target_task_id = ? AND link_type = ?)", upstreamID, models.TaskLinkTypeBlocks, upstreamID, models.TaskLinkTypeBlockedBy).
		Order("created_at ASC").
		Find(&links).Error
	return links, err
}

func (r *conveyorRepository) ListUpstreamDependencies(ctx context.Context, downstreamID uuid.UUID) ([]models.TaskLink, error) {
	var links []models.TaskLink
	err := r.db.WithContext(ctx).
		Where("(target_task_id = ? AND link_type = ?) OR (source_task_id = ? AND link_type = ?)", downstreamID, models.TaskLinkTypeBlocks, downstreamID, models.TaskLinkTypeBlockedBy).
		Order("created_at ASC").
		Find(&links).Error
	return links, err
}

func (r *conveyorRepository) CreateWorkOrder(ctx context.Context, workOrder *models.WorkOrder) error {
	return r.db.WithContext(ctx).Create(workOrder).Error
}

func (r *conveyorRepository) GetWorkOrder(ctx context.Context, id uuid.UUID) (*models.WorkOrder, error) {
	var workOrder models.WorkOrder
	if err := r.db.WithContext(ctx).First(&workOrder, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, models.ErrConveyorNotFound
		}
		return nil, err
	}
	return &workOrder, nil
}

func (r *conveyorRepository) ListWorkOrdersByTask(ctx context.Context, taskID uuid.UUID) ([]models.WorkOrder, error) {
	var out []models.WorkOrder
	err := r.db.WithContext(ctx).Where("source_task_id = ?", taskID).Order("created_at DESC").Find(&out).Error
	return out, err
}

func (r *conveyorRepository) UpdateWorkOrder(ctx context.Context, workOrder *models.WorkOrder) error {
	res := r.db.WithContext(ctx).Model(&models.WorkOrder{}).Where("id = ?", workOrder.ID).Updates(workOrder)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return models.ErrConveyorNotFound
	}
	return nil
}

func (r *conveyorRepository) CreateAgentRun(ctx context.Context, run *models.AgentRun) error {
	return r.db.WithContext(ctx).Create(run).Error
}

func (r *conveyorRepository) GetAgentRun(ctx context.Context, id uuid.UUID) (*models.AgentRun, error) {
	var run models.AgentRun
	if err := r.db.WithContext(ctx).First(&run, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, models.ErrConveyorNotFound
		}
		return nil, err
	}
	return &run, nil
}

func (r *conveyorRepository) ListAgentRuns(ctx context.Context, workItemID uuid.UUID) ([]models.AgentRun, error) {
	var runs []models.AgentRun
	err := r.db.WithContext(ctx).Where("work_item_id = ?", workItemID).Order("created_at ASC").Find(&runs).Error
	return runs, err
}

func (r *conveyorRepository) ListStaleAgentRuns(ctx context.Context, cutoff time.Time) ([]models.AgentRun, error) {
	var runs []models.AgentRun
	err := r.db.WithContext(ctx).
		Where("status IN ?", []string{models.AgentRunStatusQueued, models.AgentRunStatusRunning, models.AgentRunStatusNeedsHuman}).
		Where("COALESCE(heartbeat_at, updated_at) < ?", cutoff).
		Order("updated_at ASC").
		Find(&runs).Error
	return runs, err
}

func (r *conveyorRepository) UpdateAgentRun(ctx context.Context, run *models.AgentRun) error {
	res := r.db.WithContext(ctx).Model(&models.AgentRun{}).Where("id = ? AND work_item_id = ?", run.ID, run.WorkItemID).Updates(map[string]any{
		"status":        run.Status,
		"summary":       run.Summary,
		"log_uri":       run.LogURI,
		"workspace_uri": run.WorkspaceURI,
		"metadata":      run.Metadata,
		"started_at":    run.StartedAt,
		"heartbeat_at":  run.HeartbeatAt,
		"finished_at":   run.FinishedAt,
		"updated_at":    run.UpdatedAt,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return models.ErrConveyorNotFound
	}
	return nil
}

func (r *conveyorRepository) ListProjectReportSources(ctx context.Context, projectID uuid.UUID, start time.Time, end time.Time) ([]ports.ProjectReportSource, error) {
	var tasks []models.Task
	if err := r.db.WithContext(ctx).
		Joins("JOIN statuses ON statuses.id = tasks.status_id").
		Joins("JOIN boards ON boards.id = statuses.board_id").
		Where("boards.project_id = ? AND tasks.deleted = ?", projectID, false).
		Order("tasks.created_at ASC").
		Find(&tasks).Error; err != nil {
		return nil, err
	}
	out := make([]ports.ProjectReportSource, 0, len(tasks))
	for _, task := range tasks {
		var events []models.ConveyorEvent
		if err := r.db.WithContext(ctx).
			Where("work_item_id = ? AND timestamp >= ? AND timestamp <= ?", task.ID, start, end).
			Order("timestamp ASC").
			Find(&events).Error; err != nil {
			return nil, err
		}
		var evidence []models.Evidence
		if err := r.db.WithContext(ctx).
			Where("task_id = ?", task.ID).
			Order("created_at ASC").
			Find(&evidence).Error; err != nil {
			return nil, err
		}
		out = append(out, ports.ProjectReportSource{Task: task, Events: events, Evidence: evidence})
	}
	return out, nil
}

func (r *conveyorRepository) CreateGeneratedReport(ctx context.Context, report *models.GeneratedReport) error {
	return r.db.WithContext(ctx).Create(report).Error
}

func (r *conveyorRepository) GetGeneratedReport(ctx context.Context, id uuid.UUID) (*models.GeneratedReport, error) {
	var report models.GeneratedReport
	if err := r.db.WithContext(ctx).First(&report, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, models.ErrConveyorNotFound
		}
		return nil, err
	}
	return &report, nil
}

func (r *conveyorRepository) UpdateGeneratedReportLLM(ctx context.Context, id uuid.UUID, draft json.RawMessage, llmError string, at time.Time) error {
	res := r.db.WithContext(ctx).Model(&models.GeneratedReport{}).Where("id = ?", id).Updates(map[string]any{"llm_draft": draft, "llm_error": llmError, "updated_at": at})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return models.ErrConveyorNotFound
	}
	return nil
}

func (r *conveyorRepository) CreateForumDigest(ctx context.Context, digest *models.ForumDigest) error {
	return r.db.WithContext(ctx).Create(digest).Error
}

func (r *conveyorRepository) GetForumDigest(ctx context.Context, id uuid.UUID) (*models.ForumDigest, error) {
	var digest models.ForumDigest
	if err := r.db.WithContext(ctx).First(&digest, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, models.ErrConveyorNotFound
		}
		return nil, err
	}
	return &digest, nil
}

func (r *conveyorRepository) ListForumDigests(ctx context.Context, sourceType string, sourceID string) ([]models.ForumDigest, error) {
	query := r.db.WithContext(ctx).Model(&models.ForumDigest{})
	if sourceType != "" {
		query = query.Where("source_type = ?", sourceType)
	}
	if sourceID != "" {
		query = query.Where("source_id = ?", sourceID)
	}
	var digests []models.ForumDigest
	err := query.Order("created_at DESC").Find(&digests).Error
	return digests, err
}

func (r *conveyorRepository) CreateForumActionCandidate(ctx context.Context, candidate *models.ForumActionCandidate) error {
	return r.db.WithContext(ctx).Create(candidate).Error
}

func (r *conveyorRepository) GetForumActionCandidate(ctx context.Context, id uuid.UUID) (*models.ForumActionCandidate, error) {
	var candidate models.ForumActionCandidate
	if err := r.db.WithContext(ctx).First(&candidate, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, models.ErrConveyorNotFound
		}
		return nil, err
	}
	return &candidate, nil
}

func (r *conveyorRepository) ListForumActionCandidates(ctx context.Context, digestID uuid.UUID) ([]models.ForumActionCandidate, error) {
	var candidates []models.ForumActionCandidate
	err := r.db.WithContext(ctx).Where("digest_id = ?", digestID).Order("created_at ASC").Find(&candidates).Error
	return candidates, err
}

func (r *conveyorRepository) UpdateForumActionCandidate(ctx context.Context, candidate *models.ForumActionCandidate) error {
	res := r.db.WithContext(ctx).Model(&models.ForumActionCandidate{}).Where("id = ?", candidate.ID).Updates(candidate)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return models.ErrConveyorNotFound
	}
	return nil
}

func (r *conveyorRepository) GetIdempotencyRecord(ctx context.Context, actorID uuid.UUID, operation string, key string) (*models.IdempotencyRecord, error) {
	var record models.IdempotencyRecord
	if err := r.db.WithContext(ctx).Where("actor_id = ? AND operation = ? AND idempotency_key = ?", actorID, operation, key).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, models.ErrConveyorNotFound
		}
		return nil, err
	}
	return &record, nil
}

func (r *conveyorRepository) CreateIdempotencyRecord(ctx context.Context, record *models.IdempotencyRecord) error {
	if err := r.db.WithContext(ctx).Create(record).Error; err != nil {
		return err
	}
	return nil
}

func (r *conveyorRepository) CreateWaiver(ctx context.Context, waiver *models.Waiver) error {
	return r.db.WithContext(ctx).Create(waiver).Error
}

func (r *conveyorRepository) GetWaiver(ctx context.Context, id uuid.UUID) (*models.Waiver, error) {
	var waiver models.Waiver
	if err := r.db.WithContext(ctx).First(&waiver, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, models.ErrConveyorNotFound
		}
		return nil, err
	}
	return &waiver, nil
}

func (r *conveyorRepository) CreateApprovalRequest(ctx context.Context, request *models.ApprovalRequest) error {
	return r.db.WithContext(ctx).Create(request).Error
}

func (r *conveyorRepository) GetApprovalRequest(ctx context.Context, id uuid.UUID) (*models.ApprovalRequest, error) {
	var request models.ApprovalRequest
	if err := r.db.WithContext(ctx).First(&request, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, models.ErrConveyorNotFound
		}
		return nil, err
	}
	return &request, nil
}

func (r *conveyorRepository) UpdateApprovalRequest(ctx context.Context, request *models.ApprovalRequest) error {
	res := r.db.WithContext(ctx).Model(&models.ApprovalRequest{}).Where("id = ?", request.ID).Updates(map[string]any{
		"status":          request.Status,
		"decided_by":      request.DecidedBy,
		"decision_reason": request.DecisionReason,
		"decided_at":      request.DecidedAt,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return models.ErrConveyorNotFound
	}
	return nil
}

func (r *conveyorRepository) ListApprovalRequests(ctx context.Context, workItemID uuid.UUID) ([]models.ApprovalRequest, error) {
	var requests []models.ApprovalRequest
	err := r.db.WithContext(ctx).Where("work_item_id = ?", workItemID).Order("created_at ASC").Find(&requests).Error
	return requests, err
}

// ListPendingApprovals — все ожидающие решения approval-запросы по платформе
// (для админ-очереди). Старые сверху, чтобы решать в порядке поступления.
func (r *conveyorRepository) ListPendingApprovals(ctx context.Context, limit int) ([]models.ApprovalRequest, error) {
	var requests []models.ApprovalRequest
	q := r.db.WithContext(ctx).
		Where("status = ?", models.ApprovalStatusPending).
		Order("created_at ASC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	err := q.Find(&requests).Error
	return requests, err
}
