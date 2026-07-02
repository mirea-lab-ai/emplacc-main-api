package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	models "emplacc-api/internal/domain"
	"emplacc-api/internal/dto/request"
	"emplacc-api/internal/dto/response"
	"emplacc-api/internal/grpc/client"
	"emplacc-api/internal/service"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

func TestTaskMoveMapsCloseGateRequiredToApprovalRequired(t *testing.T) {
	e := echo.New()
	taskID := uuid.New()
	statusID := uuid.New()
	body := strings.NewReader(`{"task_id":"` + taskID.String() + `","status_id":"` + statusID.String() + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/task/move", body)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)
	controller := &TaskController{taskService: fakeTaskService{taskMoveErr: service.ErrTaskMoveCloseGateRequired}}

	err := controller.TaskMoveFunc(ctx)

	require.NoError(t, err)
	require.Equal(t, http.StatusConflict, rec.Code)
	require.JSONEq(t, `{"error":"approval_required"}`, rec.Body.String())
}

func TestImproveTaskReportRecordsAgentRunOnSuccess(t *testing.T) {
	e := echo.New()
	taskID := uuid.New()
	actorID := uuid.New()
	taskName := "Improve task"
	taskDescription := "Original task description"
	body := strings.NewReader(`{"user_text":"draft text"}`)
	req := httptest.NewRequest(http.MethodPost, "/task/"+taskID.String()+"/improve-report", body)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)
	ctx.SetParamNames("id")
	ctx.SetParamValues(taskID.String())
	ctx.Set("user_id", actorID.String())
	conveyorSvc := &recordingConveyorService{registeredID: uuid.New()}
	llm := &fakeTaskLLMClient{result: "improved text"}
	controller := &TaskController{taskService: fakeTaskService{task: &models.Task{ID: taskID, Name: &taskName, Description: &taskDescription}}, llmClient: llm, conveyorService: conveyorSvc}

	err := controller.ImproveTaskReport(ctx)

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"task_id":"`+taskID.String()+`","original_text":"draft text","improved_text":"improved text","task_title":"Improve task","task_description":"Original task description"}`, rec.Body.String())
	require.Len(t, conveyorSvc.registerRequests, 1)
	require.Equal(t, taskID, conveyorSvc.registerRequests[0].WorkItemID)
	require.Equal(t, "backend", conveyorSvc.registerRequests[0].Source)
	require.Equal(t, "llm-task-report", conveyorSvc.registerRequests[0].Harness)
	require.JSONEq(t, `{"task_id":"`+taskID.String()+`","route":"ImproveTaskReport","content_type":"text/plain","meta_keys":["system_prompt"],"outcome":"started"}`, string(conveyorSvc.registerRequests[0].Metadata))
	require.Len(t, conveyorSvc.updateRequests, 1)
	require.Equal(t, models.AgentRunStatusSucceeded, conveyorSvc.updateRequests[0].Status)
	require.NotContains(t, conveyorSvc.eventTypes, "task.completed")
}

func TestImproveTaskReportRecordsFailedAgentRunOnLLMError(t *testing.T) {
	e := echo.New()
	taskID := uuid.New()
	actorID := uuid.New()
	taskName := "Improve task"
	body := strings.NewReader(`{"user_text":"draft text"}`)
	req := httptest.NewRequest(http.MethodPost, "/task/"+taskID.String()+"/improve-report", body)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)
	ctx.SetParamNames("id")
	ctx.SetParamValues(taskID.String())
	ctx.Set("user_id", actorID.String())
	conveyorSvc := &recordingConveyorService{registeredID: uuid.New()}
	llm := &fakeTaskLLMClient{err: errors.New("llm unavailable")}
	controller := &TaskController{taskService: fakeTaskService{task: &models.Task{ID: taskID, Name: &taskName}}, llmClient: llm, conveyorService: conveyorSvc}

	err := controller.ImproveTaskReport(ctx)

	require.NoError(t, err)
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.Len(t, conveyorSvc.registerRequests, 1)
	require.Len(t, conveyorSvc.updateRequests, 1)
	require.Equal(t, models.AgentRunStatusFailed, conveyorSvc.updateRequests[0].Status)
	require.JSONEq(t, `{"task_id":"`+taskID.String()+`","route":"ImproveTaskReport","content_type":"text/plain","outcome":"grpc_error"}`, string(conveyorSvc.updateRequests[0].Metadata))
}

type fakeTaskService struct {
	taskMoveErr error
	task        *models.Task
	taskErr     error
}

func (s fakeTaskService) GetAllTasks(page, pageSize int) ([]models.Task, int64, error) {
	return nil, 0, errors.New("not implemented")
}

func (s fakeTaskService) GetTaskByID(taskID uuid.UUID) (*models.Task, error) {
	if s.taskErr != nil {
		return nil, s.taskErr
	}
	if s.task != nil {
		return s.task, nil
	}
	return nil, errors.New("not implemented")
}

func (s fakeTaskService) GetTasksByProjectID(projectID uuid.UUID, page, pageSize int) ([]models.Task, int64, error) {
	return nil, 0, errors.New("not implemented")
}

func (s fakeTaskService) CreateTask(req request.TaskCreateRequest) (uuid.UUID, error) {
	return uuid.Nil, errors.New("not implemented")
}

func (s fakeTaskService) UpdateTask(taskID uuid.UUID, req request.TaskUpdateRequest) error {
	return errors.New("not implemented")
}

func (s fakeTaskService) DeleteTask(taskID uuid.UUID) error {
	return errors.New("not implemented")
}

func (s fakeTaskService) GetTasksByUserId(userID uuid.UUID, page, pageSize int) ([]models.Task, int64, error) {
	return nil, 0, errors.New("not implemented")
}

func (s fakeTaskService) TaskMoveFunc(taskID, toStatusID uuid.UUID) ([]models.Status, error) {
	return nil, s.taskMoveErr
}

func (s fakeTaskService) GetTasksByUserIDAndProjectID(userID, projectID uuid.UUID, page, pageSize int) ([]models.Task, int64, error) {
	return nil, 0, errors.New("not implemented")
}

func (s fakeTaskService) GetActiveTasksByUserId(userID uuid.UUID, page, pageSize int) ([]models.Task, int64, error) {
	return nil, 0, errors.New("not implemented")
}

func (s fakeTaskService) GetTaskBoardAndProjectIDs(taskID uuid.UUID) (boardId, projectId uuid.UUID, err error) {
	return uuid.Nil, uuid.Nil, errors.New("not implemented")
}

func (s fakeTaskService) GetAllActiveTasksForXLSX() (*response.AllActiveTasksXLSXData, error) {
	return nil, errors.New("not implemented")
}

func (s fakeTaskService) SearchTasks(query, userID string, page, pageSize int) ([]models.Task, int64, error) {
	return nil, 0, errors.New("not implemented")
}

type fakeTaskLLMClient struct {
	result string
	err    error
}

func (c *fakeTaskLLMClient) ProcessTaskWithLLM(ctx context.Context, taskDescription, userText, taskID string, overrides *client.LLMOverrides) (string, error) {
	if c.err != nil {
		return "", c.err
	}
	return c.result, nil
}

type recordingConveyorService struct {
	registeredID     uuid.UUID
	registerRequests []service.RegisterAgentRunRequest
	updateRequests   []service.UpdateAgentRunRequest
	eventTypes       []string
}

func (s *recordingConveyorService) AuthorizeWorkItemAccess(ctx context.Context, actor service.ConveyorActor, taskID uuid.UUID) error {
	return nil
}

func (s *recordingConveyorService) RegisterAgentRun(ctx context.Context, actor service.ConveyorActor, req service.RegisterAgentRunRequest) (*service.ConveyorMutationResult, error) {
	s.registerRequests = append(s.registerRequests, req)
	return &service.ConveyorMutationResult{EntityID: s.registeredID, EventID: uuid.New()}, nil
}

func (s *recordingConveyorService) UpdateAgentRun(ctx context.Context, actor service.ConveyorActor, workItemID uuid.UUID, agentRunID uuid.UUID, req service.UpdateAgentRunRequest) (*service.ConveyorMutationResult, error) {
	s.updateRequests = append(s.updateRequests, req)
	return &service.ConveyorMutationResult{EntityID: agentRunID, EventID: uuid.New()}, nil
}

func (s *recordingConveyorService) HeartbeatAgentRun(ctx context.Context, actor service.ConveyorActor, workItemID uuid.UUID, agentRunID uuid.UUID) (*service.ConveyorMutationResult, error) {
	return nil, errors.New("not implemented")
}
func (s *recordingConveyorService) MarkStaleAgentRunsFailed(ctx context.Context, actor service.ConveyorActor, cutoff time.Time) ([]uuid.UUID, error) {
	return nil, errors.New("not implemented")
}
func (s *recordingConveyorService) GetAgentRun(ctx context.Context, workItemID uuid.UUID, agentRunID uuid.UUID) (*models.AgentRun, error) {
	return nil, errors.New("not implemented")
}
func (s *recordingConveyorService) ListAgentRuns(ctx context.Context, workItemID uuid.UUID) ([]models.AgentRun, error) {
	return nil, errors.New("not implemented")
}
func (s *recordingConveyorService) ListAgentInbox(ctx context.Context, actor service.ConveyorActor) ([]service.AgentInboxItemResponse, error) {
	return nil, errors.New("not implemented")
}
func (s *recordingConveyorService) AckAgentInboxItem(ctx context.Context, actor service.ConveyorActor, req service.AckAgentInboxItemRequest) (*service.AgentInboxItemResponse, error) {
	return nil, errors.New("not implemented")
}
func (s *recordingConveyorService) GenerateProjectReport(ctx context.Context, actor service.ConveyorActor, req service.GenerateProjectReportRequest) (*service.GeneratedReportResponse, error) {
	return nil, errors.New("not implemented")
}
func (s *recordingConveyorService) GetGeneratedReport(ctx context.Context, reportID uuid.UUID) (*service.GeneratedReportResponse, error) {
	return nil, errors.New("not implemented")
}
func (s *recordingConveyorService) ExportGeneratedReportMarkdown(ctx context.Context, reportID uuid.UUID) (string, error) {
	return "", errors.New("not implemented")
}
func (s *recordingConveyorService) CreateForumDigest(ctx context.Context, actor service.ConveyorActor, req service.CreateForumDigestRequest) (*service.ForumDigestResponse, error) {
	return nil, errors.New("not implemented")
}
func (s *recordingConveyorService) GetForumDigest(ctx context.Context, digestID uuid.UUID) (*service.ForumDigestResponse, error) {
	return nil, errors.New("not implemented")
}
func (s *recordingConveyorService) ListForumDigests(ctx context.Context, sourceType string, sourceID string) ([]service.ForumDigestResponse, error) {
	return nil, errors.New("not implemented")
}
func (s *recordingConveyorService) ConfirmForumActionCandidate(ctx context.Context, actor service.ConveyorActor, candidateID uuid.UUID, req service.ConfirmForumActionCandidateRequest) (*service.ConveyorMutationResult, error) {
	return nil, errors.New("not implemented")
}
func (s *recordingConveyorService) RejectForumActionCandidate(ctx context.Context, actor service.ConveyorActor, candidateID uuid.UUID, req service.RejectForumActionCandidateRequest) (*service.ConveyorMutationResult, error) {
	return nil, errors.New("not implemented")
}
func (s *recordingConveyorService) CreateAcceptanceCriterion(ctx context.Context, actor service.ConveyorActor, req service.CreateAcceptanceCriterionRequest) (*service.ConveyorMutationResult, error) {
	return nil, errors.New("not implemented")
}
func (s *recordingConveyorService) UpdateAcceptanceCriterionState(ctx context.Context, actor service.ConveyorActor, taskID uuid.UUID, criterionID uuid.UUID, req service.UpdateCriterionStateRequest) (*service.ConveyorMutationResult, error) {
	return nil, errors.New("not implemented")
}
func (s *recordingConveyorService) AttachEvidence(ctx context.Context, actor service.ConveyorActor, req service.AttachEvidenceRequest) (*service.ConveyorMutationResult, error) {
	return nil, errors.New("not implemented")
}
func (s *recordingConveyorService) RevokeEvidence(ctx context.Context, actor service.ConveyorActor, taskID uuid.UUID, evidenceID uuid.UUID, req service.RevokeEvidenceRequest) (*service.ConveyorMutationResult, error) {
	return nil, errors.New("not implemented")
}
func (s *recordingConveyorService) LinkTasks(ctx context.Context, actor service.ConveyorActor, req service.LinkTasksRequest) (*service.ConveyorMutationResult, error) {
	return nil, errors.New("not implemented")
}
func (s *recordingConveyorService) CloseTask(ctx context.Context, actor service.ConveyorActor, taskID uuid.UUID, req service.CloseTaskRequest) (*service.ConveyorMutationResult, error) {
	s.eventTypes = append(s.eventTypes, "task.completed")
	return nil, errors.New("not implemented")
}
func (s *recordingConveyorService) CreateWorkOrder(ctx context.Context, actor service.ConveyorActor, req service.CreateWorkOrderRequest) (*service.ConveyorMutationResult, error) {
	return nil, errors.New("not implemented")
}
func (s *recordingConveyorService) GetWorkOrder(ctx context.Context, workOrderID uuid.UUID) (*models.WorkOrder, error) {
	return nil, errors.New("not implemented")
}
func (s *recordingConveyorService) AcceptWorkOrder(ctx context.Context, actor service.ConveyorActor, workOrderID uuid.UUID, req service.AcceptWorkOrderRequest) (*service.ConveyorMutationResult, error) {
	return nil, errors.New("not implemented")
}
func (s *recordingConveyorService) RejectWorkOrder(ctx context.Context, actor service.ConveyorActor, workOrderID uuid.UUID, req service.RejectWorkOrderRequest) (*service.ConveyorMutationResult, error) {
	return nil, errors.New("not implemented")
}
func (s *recordingConveyorService) CompleteWorkOrder(ctx context.Context, actor service.ConveyorActor, workOrderID uuid.UUID, req service.CompleteWorkOrderRequest) (*service.ConveyorMutationResult, error) {
	return nil, errors.New("not implemented")
}
func (s *recordingConveyorService) CancelWorkOrder(ctx context.Context, actor service.ConveyorActor, workOrderID uuid.UUID, req service.CancelWorkOrderRequest) (*service.ConveyorMutationResult, error) {
	return nil, errors.New("not implemented")
}
func (s *recordingConveyorService) FailWorkOrder(ctx context.Context, actor service.ConveyorActor, workOrderID uuid.UUID, req service.FailWorkOrderRequest) (*service.ConveyorMutationResult, error) {
	return nil, errors.New("not implemented")
}
func (s *recordingConveyorService) ListAcceptanceCriteria(ctx context.Context, taskID uuid.UUID) ([]models.AcceptanceCriterion, error) {
	return nil, errors.New("not implemented")
}
func (s *recordingConveyorService) ListEvidence(ctx context.Context, taskID uuid.UUID) ([]models.Evidence, error) {
	return nil, errors.New("not implemented")
}
func (s *recordingConveyorService) ListEvents(ctx context.Context, taskID uuid.UUID) ([]models.ConveyorEvent, error) {
	return nil, errors.New("not implemented")
}
func (s *recordingConveyorService) ListTaskLinks(ctx context.Context, taskID uuid.UUID) ([]models.TaskLink, error) {
	return nil, errors.New("not implemented")
}
func (s *recordingConveyorService) CreateWaiver(ctx context.Context, actor service.ConveyorActor, req service.CreateWaiverRequest) (*service.ConveyorMutationResult, error) {
	return nil, errors.New("not implemented")
}
func (s *recordingConveyorService) GetWaiver(ctx context.Context, id uuid.UUID) (*models.Waiver, error) {
	return nil, errors.New("not implemented")
}
func (s *recordingConveyorService) RequestApproval(ctx context.Context, actor service.ConveyorActor, req service.RequestApprovalRequest) (*service.ConveyorMutationResult, error) {
	return nil, errors.New("not implemented")
}
func (s *recordingConveyorService) GrantApproval(ctx context.Context, actor service.ConveyorActor, approvalID uuid.UUID, req service.DecideApprovalRequest) (*service.ConveyorMutationResult, error) {
	return nil, errors.New("not implemented")
}
func (s *recordingConveyorService) DenyApproval(ctx context.Context, actor service.ConveyorActor, approvalID uuid.UUID, req service.DecideApprovalRequest) (*service.ConveyorMutationResult, error) {
	return nil, errors.New("not implemented")
}
func (s *recordingConveyorService) SuggestCriteria(ctx context.Context, actor service.ConveyorActor, taskID uuid.UUID) (string, error) {
	return "", errors.New("not implemented")
}
func (s *recordingConveyorService) SummarizeEvidence(ctx context.Context, actor service.ConveyorActor, taskID uuid.UUID) (string, error) {
	return "", errors.New("not implemented")
}
func (s *recordingConveyorService) GetApprovalRequest(ctx context.Context, id uuid.UUID) (*models.ApprovalRequest, error) {
	return nil, errors.New("not implemented")
}
func (s *recordingConveyorService) ListApprovalRequests(ctx context.Context, workItemID uuid.UUID) ([]models.ApprovalRequest, error) {
	return nil, errors.New("not implemented")
}

func (s *recordingConveyorService) ListPendingApprovals(ctx context.Context, limit int) ([]models.ApprovalRequest, error) {
	return nil, errors.New("not implemented")
}
