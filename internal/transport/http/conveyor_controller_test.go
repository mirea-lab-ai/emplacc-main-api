package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	models "emplacc-api/internal/domain"
	"emplacc-api/internal/service"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

func TestConveyorControllerRejectsInvalidWorkItemID(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/work-items/not-a-uuid/criteria", nil)
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)
	ctx.SetParamNames("id")
	ctx.SetParamValues("not-a-uuid")

	controller := &ConveyorController{svc: fakeConveyorService{}}
	err := controller.ListCriteria(ctx)

	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.JSONEq(t, `{"error":"validation_error"}`, rec.Body.String())
}

func TestConveyorControllerImportsPMCanonThroughApprovedBackendRoute(t *testing.T) {
	e := echo.New()
	actorID := uuid.New()
	statusID := uuid.New()
	root := t.TempDir()
	t.Setenv("PM_CANON_ROOT", root)
	body := strings.NewReader(`{"scope":"default","dry_run":true,"status_by_pm_state":{"TODO":"` + statusID.String() + `"},"approval_granted":true,"approval_token":"approval-1","idempotency_key":"pm-import-1"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/conveyor/pm-import", body)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set("Idempotency-Key", "pm-import-header")
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)
	ctx.Set("user_id", actorID.String())
	pmImport := &fakeControllerPMImportService{summary: &service.PMImportSummary{DryRun: true, Tickets: service.PMImportCounter{Total: 1}}}
	controller := &ConveyorController{svc: fakeConveyorService{}, pmImport: pmImport}

	err := controller.ImportPMCanon(ctx)

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, pmImport.called)
	require.Equal(t, actorID, pmImport.actor.ActorID)
	require.True(t, pmImport.req.DryRun)
	require.Equal(t, filepath.Join(root, "default"), pmImport.req.RootPath)
	require.Equal(t, statusID, pmImport.req.StatusByPMState["TODO"])
	require.JSONEq(t, `{"dry_run":true,"switch_over_accepted":false,"tickets":{"total":1,"created":0,"reused":0},"criteria":{"total":0,"created":0,"reused":0},"evidence":{"total":0,"created":0,"reused":0},"events":{"total":0,"created":0,"reused":0},"links":{"total":0,"created":0,"reused":0},"status_facts":{"total":0,"created":0,"reused":0}}`, rec.Body.String())
}

func TestConveyorControllerExecutesPMCanonImportWhenDryRunFalse(t *testing.T) {
	e := echo.New()
	actorID := uuid.New()
	statusID := uuid.New()
	root := t.TempDir()
	t.Setenv("PM_CANON_ROOT", root)
	body := strings.NewReader(`{"scope":"default","dry_run":false,"status_by_pm_state":{"DONE":"` + statusID.String() + `"},"approval_granted":true,"approval_token":"approval-1"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/conveyor/pm-import", body)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)
	ctx.Set("user_id", actorID.String())
	pmImport := &fakeControllerPMImportService{summary: &service.PMImportSummary{DryRun: false, Tickets: service.PMImportCounter{Total: 1, Created: 1}}}
	controller := &ConveyorController{svc: fakeConveyorService{}, pmImport: pmImport}

	err := controller.ImportPMCanon(ctx)

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, pmImport.called)
	require.False(t, pmImport.req.DryRun)
	require.Equal(t, filepath.Join(root, "default"), pmImport.req.RootPath)
	require.Equal(t, statusID, pmImport.req.StatusByPMState["DONE"])
	require.Contains(t, rec.Body.String(), `"created":1`)
}

func TestConveyorControllerPMImportRequiresApproval(t *testing.T) {
	e := echo.New()
	body := strings.NewReader(`{"scope":"default","dry_run":true}`)
	req := httptest.NewRequest(http.MethodPost, "/api/conveyor/pm-import", body)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)
	ctx.Set("user_id", uuid.New().String())
	pmImport := &fakeControllerPMImportService{summary: &service.PMImportSummary{DryRun: true}}
	controller := &ConveyorController{svc: fakeConveyorService{}, pmImport: pmImport}

	err := controller.ImportPMCanon(ctx)

	require.NoError(t, err)
	require.False(t, pmImport.called)
	require.Equal(t, http.StatusConflict, rec.Code)
	require.JSONEq(t, `{"error":"approval_required"}`, rec.Body.String())
}

func TestConveyorControllerMapsConflict(t *testing.T) {
	e := echo.New()
	itemID := uuid.New().String()
	req := httptest.NewRequest(http.MethodGet, "/api/work-items/"+itemID+"/criteria", nil)
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)
	ctx.SetParamNames("id")
	ctx.SetParamValues(itemID)

	ctx.Set("user_id", uuid.New().String())
	controller := &ConveyorController{svc: fakeConveyorService{listCriteriaErr: service.ErrConflict}}
	err := controller.ListCriteria(ctx)

	require.NoError(t, err)
	require.Equal(t, http.StatusConflict, rec.Code)
	require.JSONEq(t, `{"error":"conflict"}`, rec.Body.String())
}

func TestConveyorControllerListsLinks(t *testing.T) {
	e := echo.New()
	itemID := uuid.New()
	targetID := uuid.New()
	linkID := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/api/work-items/"+itemID.String()+"/links", nil)
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)
	ctx.SetParamNames("id")
	ctx.SetParamValues(itemID.String())

	ctx.Set("user_id", uuid.New().String())
	controller := &ConveyorController{svc: fakeConveyorService{links: []models.TaskLink{{ID: linkID, SourceTaskID: itemID, TargetTaskID: targetID, LinkType: models.TaskLinkTypeRelatesTo}}}}
	err := controller.ListLinks(ctx)

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `[{"id":"`+linkID.String()+`","source_task_id":"`+itemID.String()+`","target_task_id":"`+targetID.String()+`","link_type":"relates_to","created_by":"00000000-0000-0000-0000-000000000000","created_at":"0001-01-01T00:00:00Z"}]`, strings.TrimSpace(rec.Body.String()))
}

func TestConveyorControllerAuthorizesWorkItemReadAndWrite(t *testing.T) {
	e := echo.New()
	actorID := uuid.New()
	workItemID := uuid.New()

	readReq := httptest.NewRequest(http.MethodGet, "/api/work-items/"+workItemID.String()+"/evidence", nil)
	readRec := httptest.NewRecorder()
	readCtx := e.NewContext(readReq, readRec)
	readCtx.SetParamNames("id")
	readCtx.SetParamValues(workItemID.String())
	readCtx.Set("user_id", actorID.String())
	readSvc := fakeConveyorService{authorizeErr: service.ErrPermissionDenied}
	readController := &ConveyorController{svc: readSvc}

	require.NoError(t, readController.ListEvidence(readCtx))
	require.Equal(t, http.StatusForbidden, readRec.Code)
	require.JSONEq(t, `{"error":"permission_denied"}`, readRec.Body.String())

	writeBody := strings.NewReader(`{"type":"link","verdict":"supports","uri":"https://example.test/evidence"}`)
	writeReq := httptest.NewRequest(http.MethodPost, "/api/work-items/"+workItemID.String()+"/evidence", writeBody)
	writeReq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	writeRec := httptest.NewRecorder()
	writeCtx := e.NewContext(writeReq, writeRec)
	writeCtx.SetParamNames("id")
	writeCtx.SetParamValues(workItemID.String())
	writeCtx.Set("user_id", actorID.String())
	writeSvc := fakeConveyorService{authorizeErr: service.ErrPermissionDenied, mutationResult: &service.ConveyorMutationResult{EntityID: uuid.New(), EventID: uuid.New()}}
	writeController := &ConveyorController{svc: writeSvc}

	require.NoError(t, writeController.AttachEvidence(writeCtx))
	require.Equal(t, http.StatusForbidden, writeRec.Code)
	require.JSONEq(t, `{"error":"permission_denied"}`, writeRec.Body.String())
}

func TestConveyorControllerMapsLinksError(t *testing.T) {
	e := echo.New()
	itemID := uuid.New().String()
	req := httptest.NewRequest(http.MethodGet, "/api/work-items/"+itemID+"/links", nil)
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)
	ctx.SetParamNames("id")
	ctx.SetParamValues(itemID)

	ctx.Set("user_id", uuid.New().String())
	controller := &ConveyorController{svc: fakeConveyorService{listLinksErr: service.ErrNotFound}}
	err := controller.ListLinks(ctx)

	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.JSONEq(t, `{"error":"not_found"}`, rec.Body.String())
}

func TestConveyorControllerCreatesAndListsAgentRuns(t *testing.T) {
	e := echo.New()
	workItemID := uuid.New()
	agentRunID := uuid.New()
	actorID := uuid.New()
	body := strings.NewReader(`{"source":"rest","harness":"external-harness","status":"queued","summary":"queued","log_uri":"https://example.test/log","workspace_uri":"file:///workspace","metadata":{"task_id":"task-1"},"idempotency_key":"run-key"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/work-items/"+workItemID.String()+"/agent-runs", body)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)
	ctx.SetParamNames("id")
	ctx.SetParamValues(workItemID.String())
	ctx.Set("user_id", actorID.String())

	controller := &ConveyorController{svc: fakeConveyorService{mutationResult: &service.ConveyorMutationResult{EntityID: agentRunID, EventID: uuid.New()}}}
	err := controller.RegisterAgentRun(ctx)

	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, rec.Code)
	require.Contains(t, rec.Body.String(), agentRunID.String())

	listReq := httptest.NewRequest(http.MethodGet, "/api/work-items/"+workItemID.String()+"/agent-runs", nil)
	listRec := httptest.NewRecorder()
	listCtx := e.NewContext(listReq, listRec)
	listCtx.SetParamNames("id")
	listCtx.SetParamValues(workItemID.String())
	listCtx.Set("user_id", actorID.String())

	run := models.AgentRun{ID: agentRunID, WorkItemID: workItemID, Source: "rest", Harness: "external-harness", Status: models.AgentRunStatusQueued, Summary: "queued", LogURI: "https://example.test/log", WorkspaceURI: "file:///workspace", Metadata: json.RawMessage(`{"task_id":"task-1"}`)}
	controller = &ConveyorController{svc: fakeConveyorService{agentRuns: []models.AgentRun{run}}}
	err = controller.ListAgentRuns(listCtx)

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, listRec.Code)
	require.JSONEq(t, `[{"id":"`+agentRunID.String()+`","work_item_id":"`+workItemID.String()+`","source":"rest","harness":"external-harness","status":"queued","summary":"queued","log_uri":"https://example.test/log","workspace_uri":"file:///workspace","metadata":{"task_id":"task-1"},"created_by":"00000000-0000-0000-0000-000000000000","created_at":"0001-01-01T00:00:00Z","updated_at":"0001-01-01T00:00:00Z"}]`, strings.TrimSpace(listRec.Body.String()))
}

func TestConveyorControllerUpdatesAgentRunAndHeartbeat(t *testing.T) {
	e := echo.New()
	workItemID := uuid.New()
	agentRunID := uuid.New()
	actorID := uuid.New()
	body := strings.NewReader(`{"status":"running","summary":"running","metadata":{"attempt":"1"}}`)
	req := httptest.NewRequest(http.MethodPatch, "/api/work-items/"+workItemID.String()+"/agent-runs/"+agentRunID.String(), body)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)
	ctx.SetParamNames("id", "agent_run_id")
	ctx.SetParamValues(workItemID.String(), agentRunID.String())
	ctx.Set("api_token_user_id", actorID)

	controller := &ConveyorController{svc: fakeConveyorService{mutationResult: &service.ConveyorMutationResult{EntityID: agentRunID, EventID: uuid.New()}}}
	err := controller.UpdateAgentRun(ctx)

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), agentRunID.String())

	heartbeatReq := httptest.NewRequest(http.MethodPost, "/api/work-items/"+workItemID.String()+"/agent-runs/"+agentRunID.String()+"/heartbeat", nil)
	heartbeatRec := httptest.NewRecorder()
	heartbeatCtx := e.NewContext(heartbeatReq, heartbeatRec)
	heartbeatCtx.SetParamNames("id", "agent_run_id")
	heartbeatCtx.SetParamValues(workItemID.String(), agentRunID.String())
	heartbeatCtx.Set("api_token_user_id", actorID)

	err = controller.HeartbeatAgentRun(heartbeatCtx)

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, heartbeatRec.Code)
	require.Contains(t, heartbeatRec.Body.String(), agentRunID.String())
}

func TestConveyorControllerRejectsAgentRunMutationWithoutActor(t *testing.T) {
	e := echo.New()
	workItemID := uuid.New()
	body := strings.NewReader(`{"source":"rest","harness":"external-harness","status":"queued"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/work-items/"+workItemID.String()+"/agent-runs", body)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)
	ctx.SetParamNames("id")
	ctx.SetParamValues(workItemID.String())

	controller := &ConveyorController{svc: fakeConveyorService{mutationResult: &service.ConveyorMutationResult{EntityID: uuid.New(), EventID: uuid.New()}}}
	err := controller.RegisterAgentRun(ctx)

	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, rec.Code)
	require.JSONEq(t, `{"error":"permission_denied"}`, rec.Body.String())
}

func TestConveyorControllerMapsAgentRunNotFound(t *testing.T) {
	e := echo.New()
	workItemID := uuid.New()
	runID := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/api/work-items/"+workItemID.String()+"/agent-runs/"+runID.String(), nil)
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)
	ctx.SetParamNames("id", "agent_run_id")
	ctx.SetParamValues(workItemID.String(), runID.String())

	ctx.Set("user_id", uuid.New().String())
	controller := &ConveyorController{svc: fakeConveyorService{getAgentRunErr: service.ErrNotFound}}
	err := controller.GetAgentRun(ctx)

	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.JSONEq(t, `{"error":"not_found"}`, rec.Body.String())
}

func TestConveyorControllerExposesAgentRunTimingFields(t *testing.T) {
	e := echo.New()
	workItemID := uuid.New()
	agentRunID := uuid.New()
	startedAt := time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)
	heartbeatAt := startedAt.Add(time.Minute)
	finishedAt := startedAt.Add(2 * time.Minute)
	req := httptest.NewRequest(http.MethodGet, "/api/work-items/"+workItemID.String()+"/agent-runs/"+agentRunID.String(), nil)
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)
	ctx.SetParamNames("id", "agent_run_id")
	ctx.SetParamValues(workItemID.String(), agentRunID.String())

	ctx.Set("user_id", uuid.New().String())
	run := &models.AgentRun{ID: agentRunID, WorkItemID: workItemID, Source: "rest", Harness: "llm-task-report", Status: models.AgentRunStatusSucceeded, Summary: "completed", StartedAt: &startedAt, HeartbeatAt: &heartbeatAt, FinishedAt: &finishedAt, CreatedAt: startedAt, UpdatedAt: finishedAt}
	controller := &ConveyorController{svc: fakeConveyorService{agentRun: run}}
	err := controller.GetAgentRun(ctx)

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"started_at":"2026-06-17T10:00:00Z"`)
	require.Contains(t, rec.Body.String(), `"heartbeat_at":"2026-06-17T10:01:00Z"`)
	require.Contains(t, rec.Body.String(), `"finished_at":"2026-06-17T10:02:00Z"`)
}

func TestConveyorControllerCreatesForumDigestAndConfirmsCandidate(t *testing.T) {
	e := echo.New()
	actorID := uuid.New()
	digestID := uuid.New()
	candidateID := uuid.New()
	workItemID := uuid.New()
	statusID := uuid.New()
	body := strings.NewReader(`{"source_type":"forum","source_id":"topic-1","source_title":"Planning","source_locator":"https://forum.example.test/topic/1","candidate_inputs":[{"title":"Create endpoint","description":"Add handler","source_locator":"https://forum.example.test/topic/1#msg-1"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/forum-digests", body)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)
	ctx.Set("user_id", actorID.String())
	digest := &service.ForumDigestResponse{ID: digestID, SourceType: models.ConversationSourceTypeForum, SourceID: "topic-1", Candidates: []service.ForumActionCandidateResponse{{ID: candidateID, DigestID: digestID, Title: "Create endpoint", Status: models.ForumActionCandidateStatusPending}}}
	controller := &ConveyorController{svc: fakeConveyorService{forumDigest: digest, mutationResult: &service.ConveyorMutationResult{EntityID: workItemID, EventID: uuid.New()}}}

	err := controller.CreateForumDigest(ctx)

	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, rec.Code)
	require.Contains(t, rec.Body.String(), digestID.String())
	require.Contains(t, rec.Body.String(), candidateID.String())

	confirmBody := strings.NewReader(`{"status_id":"` + statusID.String() + `","idempotency_key":"confirm-1"}`)
	confirmReq := httptest.NewRequest(http.MethodPost, "/api/forum-action-candidates/"+candidateID.String()+"/confirm", confirmBody)
	confirmReq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	confirmRec := httptest.NewRecorder()
	confirmCtx := e.NewContext(confirmReq, confirmRec)
	confirmCtx.SetParamNames("id")
	confirmCtx.SetParamValues(candidateID.String())
	confirmCtx.Set("user_id", actorID.String())

	err = controller.ConfirmForumActionCandidate(confirmCtx)

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, confirmRec.Code)
	require.Contains(t, confirmRec.Body.String(), workItemID.String())
}

func TestConveyorControllerMapsForumCandidateConflict(t *testing.T) {
	e := echo.New()
	candidateID := uuid.New()
	statusID := uuid.New()
	body := strings.NewReader(`{"status_id":"` + statusID.String() + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/forum-action-candidates/"+candidateID.String()+"/confirm", body)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)
	ctx.SetParamNames("id")
	ctx.SetParamValues(candidateID.String())
	ctx.Set("user_id", uuid.New().String())

	controller := &ConveyorController{svc: fakeConveyorService{mutationErr: service.ErrConflict}}
	err := controller.ConfirmForumActionCandidate(ctx)

	require.NoError(t, err)
	require.Equal(t, http.StatusConflict, rec.Code)
	require.JSONEq(t, `{"error":"conflict"}`, rec.Body.String())
}

func TestConveyorControllerRejectsForumCandidateAndBlocksConfirmAfterReject(t *testing.T) {
	e := echo.New()
	candidateID := uuid.New()
	actorID := uuid.New()
	statusID := uuid.New()
	rejected := &statefulForumCandidateControllerService{candidateID: candidateID, status: models.ForumActionCandidateStatusPending, mutationID: candidateID}
	controller := &ConveyorController{svc: rejected}

	rejectReq := httptest.NewRequest(http.MethodPost, "/api/forum-action-candidates/"+candidateID.String()+"/reject", strings.NewReader(`{"reason":"out of scope","idempotency_key":"reject-1"}`))
	rejectReq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rejectRec := httptest.NewRecorder()
	rejectCtx := e.NewContext(rejectReq, rejectRec)
	rejectCtx.SetParamNames("id")
	rejectCtx.SetParamValues(candidateID.String())
	rejectCtx.Set("user_id", actorID.String())

	err := controller.RejectForumActionCandidate(rejectCtx)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rejectRec.Code)
	require.Contains(t, rejectRec.Body.String(), candidateID.String())
	require.Equal(t, models.ForumActionCandidateStatusRejected, rejected.status)

	confirmReq := httptest.NewRequest(http.MethodPost, "/api/forum-action-candidates/"+candidateID.String()+"/confirm", strings.NewReader(`{"status_id":"`+statusID.String()+`"}`))
	confirmReq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	confirmRec := httptest.NewRecorder()
	confirmCtx := e.NewContext(confirmReq, confirmRec)
	confirmCtx.SetParamNames("id")
	confirmCtx.SetParamValues(candidateID.String())
	confirmCtx.Set("user_id", actorID.String())

	err = controller.ConfirmForumActionCandidate(confirmCtx)
	require.NoError(t, err)
	require.Equal(t, http.StatusConflict, confirmRec.Code)
	require.JSONEq(t, `{"error":"conflict"}`, confirmRec.Body.String())
}

func TestConveyorControllerForumValidationFailures(t *testing.T) {
	e := echo.New()
	actorID := uuid.New()
	controller := &ConveyorController{svc: fakeConveyorService{}}

	invalidSourceReq := httptest.NewRequest(http.MethodPost, "/api/forum-digests", strings.NewReader(`{"source_type":"email","source_id":"topic-1"}`))
	invalidSourceReq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	invalidSourceRec := httptest.NewRecorder()
	invalidSourceCtx := e.NewContext(invalidSourceReq, invalidSourceRec)
	invalidSourceCtx.Set("user_id", actorID.String())
	require.NoError(t, controller.CreateForumDigest(invalidSourceCtx))
	require.Equal(t, http.StatusBadRequest, invalidSourceRec.Code)
	require.JSONEq(t, `{"error":"validation_error"}`, invalidSourceRec.Body.String())

	invalidPeriodReq := httptest.NewRequest(http.MethodPost, "/api/forum-digests", strings.NewReader(`{"source_type":"forum","source_id":"topic-1","period_start":"2026-06-17T10:00:00Z","period_end":"2026-06-17T09:00:00Z"}`))
	invalidPeriodReq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	invalidPeriodRec := httptest.NewRecorder()
	invalidPeriodCtx := e.NewContext(invalidPeriodReq, invalidPeriodRec)
	invalidPeriodCtx.Set("user_id", actorID.String())
	require.NoError(t, controller.CreateForumDigest(invalidPeriodCtx))
	require.Equal(t, http.StatusBadRequest, invalidPeriodRec.Code)
	require.JSONEq(t, `{"error":"validation_error"}`, invalidPeriodRec.Body.String())

	invalidListReq := httptest.NewRequest(http.MethodGet, "/api/forum-digests?source_type=email", nil)
	invalidListRec := httptest.NewRecorder()
	invalidListCtx := e.NewContext(invalidListReq, invalidListRec)
	require.NoError(t, controller.ListForumDigests(invalidListCtx))
	require.Equal(t, http.StatusBadRequest, invalidListRec.Code)
	require.JSONEq(t, `{"error":"validation_error"}`, invalidListRec.Body.String())

	candidateID := uuid.New()
	invalidStatusReq := httptest.NewRequest(http.MethodPost, "/api/forum-action-candidates/"+candidateID.String()+"/confirm", strings.NewReader(`{"status_id":"not-a-uuid"}`))
	invalidStatusReq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	invalidStatusRec := httptest.NewRecorder()
	invalidStatusCtx := e.NewContext(invalidStatusReq, invalidStatusRec)
	invalidStatusCtx.SetParamNames("id")
	invalidStatusCtx.SetParamValues(candidateID.String())
	invalidStatusCtx.Set("user_id", actorID.String())
	require.NoError(t, controller.ConfirmForumActionCandidate(invalidStatusCtx))
	require.Equal(t, http.StatusBadRequest, invalidStatusRec.Code)

	statusID := uuid.New()
	invalidAssignedReq := httptest.NewRequest(http.MethodPost, "/api/forum-action-candidates/"+candidateID.String()+"/confirm", strings.NewReader(`{"status_id":"`+statusID.String()+`","assigned_to":"not-a-uuid"}`))
	invalidAssignedReq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	invalidAssignedRec := httptest.NewRecorder()
	invalidAssignedCtx := e.NewContext(invalidAssignedReq, invalidAssignedRec)
	invalidAssignedCtx.SetParamNames("id")
	invalidAssignedCtx.SetParamValues(candidateID.String())
	invalidAssignedCtx.Set("user_id", actorID.String())
	require.NoError(t, controller.ConfirmForumActionCandidate(invalidAssignedCtx))
	require.Equal(t, http.StatusBadRequest, invalidAssignedRec.Code)
}

func TestConveyorControllerListsAndReadsForumDigests(t *testing.T) {
	e := echo.New()
	digestID := uuid.New()
	digest := service.ForumDigestResponse{ID: digestID, SourceType: models.ConversationSourceTypeForum, SourceID: "topic-list"}
	controller := &ConveyorController{svc: fakeConveyorService{forumDigest: &digest, forumDigests: []service.ForumDigestResponse{digest}}}

	listReq := httptest.NewRequest(http.MethodGet, "/api/forum-digests?source_type=forum&source_id=topic-list", nil)
	listRec := httptest.NewRecorder()
	listCtx := e.NewContext(listReq, listRec)
	err := controller.ListForumDigests(listCtx)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, listRec.Code)
	require.Contains(t, listRec.Body.String(), digestID.String())

	getReq := httptest.NewRequest(http.MethodGet, "/api/forum-digests/"+digestID.String(), nil)
	getRec := httptest.NewRecorder()
	getCtx := e.NewContext(getReq, getRec)
	getCtx.SetParamNames("id")
	getCtx.SetParamValues(digestID.String())
	err = controller.GetForumDigest(getCtx)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, getRec.Code)
	require.Contains(t, getRec.Body.String(), digestID.String())
}

func TestConveyorControllerCreatesGetsAndExportsGeneratedReport(t *testing.T) {
	e := echo.New()
	projectID := uuid.New()
	reportID := uuid.New()
	actorID := uuid.New()
	periodStart := time.Date(2026, 6, 9, 0, 0, 0, 0, time.UTC)
	periodEnd := time.Date(2026, 6, 16, 0, 0, 0, 0, time.UTC)
	report := &models.GeneratedReport{ID: reportID, ProjectID: projectID, PeriodStart: periodStart, PeriodEnd: periodEnd, Facts: json.RawMessage(`[{
		"kind":"completed_work",
		"text":"Completed work",
		"source_event_ids":["11111111-1111-1111-1111-111111111111"],
		"source_evidence_ids":["22222222-2222-2222-2222-222222222222"]
	}]`), Conclusions: json.RawMessage(`[{
		"kind":"summary",
		"text":"1 completed work item"
	}]`), Risks: json.RawMessage(`[]`), SourceEventIDs: json.RawMessage(`[
		"11111111-1111-1111-1111-111111111111"
	]`), SourceEvidenceIDs: json.RawMessage(`[
		"22222222-2222-2222-2222-222222222222"
	]`), CreatedBy: actorID, CreatedAt: periodEnd, UpdatedAt: periodEnd}

	body := strings.NewReader(`{"period_start":"2026-06-09T00:00:00Z","period_end":"2026-06-16T00:00:00Z","include_llm":false}`)
	req := httptest.NewRequest(http.MethodPost, "/api/projects/"+projectID.String()+"/generated-reports", body)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)
	ctx.SetParamNames("project_id")
	ctx.SetParamValues(projectID.String())
	ctx.Set("user_id", actorID.String())
	controller := &ConveyorController{svc: fakeConveyorService{generatedReport: report}}

	err := controller.CreateGeneratedReport(ctx)

	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, rec.Code)
	require.Contains(t, rec.Body.String(), reportID.String())
	require.Contains(t, rec.Body.String(), "facts")
	require.NotContains(t, rec.Body.String(), "llm_draft\":\"")

	getReq := httptest.NewRequest(http.MethodGet, "/api/generated-reports/"+reportID.String(), nil)
	getRec := httptest.NewRecorder()
	getCtx := e.NewContext(getReq, getRec)
	getCtx.SetParamNames("id")
	getCtx.SetParamValues(reportID.String())
	err = controller.GetGeneratedReport(getCtx)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, getRec.Code)
	require.Contains(t, getRec.Body.String(), "completed_work")

	markdownReq := httptest.NewRequest(http.MethodGet, "/api/generated-reports/"+reportID.String()+"/markdown", nil)
	markdownRec := httptest.NewRecorder()
	markdownCtx := e.NewContext(markdownReq, markdownRec)
	markdownCtx.SetParamNames("id")
	markdownCtx.SetParamValues(reportID.String())
	controller = &ConveyorController{svc: fakeConveyorService{markdown: "# Generated Report\n\n- event 11111111-1111-1111-1111-111111111111"}}
	err = controller.ExportGeneratedReportMarkdown(markdownCtx)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, markdownRec.Code)
	require.Equal(t, "text/markdown; charset=UTF-8", markdownRec.Header().Get(echo.HeaderContentType))
	require.Contains(t, markdownRec.Body.String(), "11111111-1111-1111-1111-111111111111")
}

func TestConveyorControllerGeneratedReportValidationNotFoundAndPermissionDenied(t *testing.T) {
	e := echo.New()
	projectID := uuid.New()
	reportID := uuid.New()

	invalidReq := httptest.NewRequest(http.MethodPost, "/api/projects/not-a-uuid/generated-reports", strings.NewReader(`{}`))
	invalidRec := httptest.NewRecorder()
	invalidCtx := e.NewContext(invalidReq, invalidRec)
	invalidCtx.SetParamNames("project_id")
	invalidCtx.SetParamValues("not-a-uuid")
	controller := &ConveyorController{svc: fakeConveyorService{}}
	require.NoError(t, controller.CreateGeneratedReport(invalidCtx))
	require.Equal(t, http.StatusBadRequest, invalidRec.Code)

	deniedReq := httptest.NewRequest(http.MethodPost, "/api/projects/"+projectID.String()+"/generated-reports", strings.NewReader(`{"period_start":"2026-06-16T00:00:00Z","period_end":"2026-06-09T00:00:00Z"}`))
	deniedReq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	deniedRec := httptest.NewRecorder()
	deniedCtx := e.NewContext(deniedReq, deniedRec)
	deniedCtx.SetParamNames("project_id")
	deniedCtx.SetParamValues(projectID.String())
	require.NoError(t, controller.CreateGeneratedReport(deniedCtx))
	require.Equal(t, http.StatusForbidden, deniedRec.Code)
	require.JSONEq(t, `{"error":"permission_denied"}`, deniedRec.Body.String())

	getReq := httptest.NewRequest(http.MethodGet, "/api/generated-reports/"+reportID.String(), nil)
	getRec := httptest.NewRecorder()
	getCtx := e.NewContext(getReq, getRec)
	getCtx.SetParamNames("id")
	getCtx.SetParamValues(reportID.String())
	controller = &ConveyorController{svc: fakeConveyorService{getGeneratedReportErr: service.ErrNotFound}}
	require.NoError(t, controller.GetGeneratedReport(getCtx))
	require.Equal(t, http.StatusNotFound, getRec.Code)
}

func TestConveyorControllerWorkOrderCreateAcceptCompleteAndGetCoord(t *testing.T) {
	e := echo.New()
	actorID := uuid.New()
	workOrderID := uuid.New()
	targetID := uuid.New()
	eventID := uuid.New()
	sourceTaskID := uuid.New()
	providerBoardID := uuid.New()
	providerStatusID := uuid.New()
	mutation := &service.ConveyorMutationResult{EntityID: workOrderID, EventID: eventID}
	controller := &ConveyorController{svc: fakeConveyorService{mutationResult: mutation, workOrder: &models.WorkOrder{ID: workOrderID, SourceTaskID: sourceTaskID, ProviderBoardID: providerBoardID, ProviderStatusID: providerStatusID, Goal: "Do work", Status: models.WorkOrderStatusRequested}}}

	body := strings.NewReader(`{"source_task_id":"` + sourceTaskID.String() + `","provider_board_id":"` + providerBoardID.String() + `","provider_status_id":"` + providerStatusID.String() + `","goal":"Do work"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/work-orders", body)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)
	ctx.Set("user_id", actorID.String())
	require.NoError(t, controller.CreateWorkOrder(ctx))
	require.Equal(t, http.StatusCreated, rec.Code)
	require.Contains(t, rec.Body.String(), workOrderID.String())

	getReq := httptest.NewRequest(http.MethodGet, "/api/work-orders/"+workOrderID.String(), nil)
	getRec := httptest.NewRecorder()
	getCtx := e.NewContext(getReq, getRec)
	getCtx.SetParamNames("id")
	getCtx.SetParamValues(workOrderID.String())
	require.NoError(t, controller.GetWorkOrder(getCtx))
	require.Equal(t, http.StatusOK, getRec.Code)
	require.Contains(t, getRec.Body.String(), models.WorkOrderStatusRequested)

	acceptReq := httptest.NewRequest(http.MethodPost, "/api/work-orders/"+workOrderID.String()+"/accept", strings.NewReader(`{"target_name":"Provider task"}`))
	acceptReq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	acceptRec := httptest.NewRecorder()
	acceptCtx := e.NewContext(acceptReq, acceptRec)
	acceptCtx.SetParamNames("id")
	acceptCtx.SetParamValues(workOrderID.String())
	acceptCtx.Set("api_token_user_id", actorID)
	controller = &ConveyorController{svc: fakeConveyorService{mutationResult: &service.ConveyorMutationResult{EntityID: targetID, EventID: eventID}}}
	require.NoError(t, controller.AcceptWorkOrder(acceptCtx))
	require.Equal(t, http.StatusOK, acceptRec.Code)
	require.Contains(t, acceptRec.Body.String(), targetID.String())

	evidenceID := uuid.New()
	completeReq := httptest.NewRequest(http.MethodPost, "/api/work-orders/"+workOrderID.String()+"/complete", strings.NewReader(`{"result_evidence_id":"`+evidenceID.String()+`"}`))
	completeReq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	completeRec := httptest.NewRecorder()
	completeCtx := e.NewContext(completeReq, completeRec)
	completeCtx.SetParamNames("id")
	completeCtx.SetParamValues(workOrderID.String())
	completeCtx.Set("user_id", actorID.String())
	controller = &ConveyorController{svc: fakeConveyorService{mutationResult: mutation}}
	require.NoError(t, controller.CompleteWorkOrder(completeCtx))
	require.Equal(t, http.StatusOK, completeRec.Code)
}

func TestConveyorControllerWorkOrderErrorsCoord(t *testing.T) {
	e := echo.New()
	workOrderID := uuid.New()
	controller := &ConveyorController{svc: fakeConveyorService{mutationErr: service.ErrConflict}}

	invalidReq := httptest.NewRequest(http.MethodGet, "/api/work-orders/not-a-uuid", nil)
	invalidRec := httptest.NewRecorder()
	invalidCtx := e.NewContext(invalidReq, invalidRec)
	invalidCtx.SetParamNames("id")
	invalidCtx.SetParamValues("not-a-uuid")
	require.NoError(t, controller.GetWorkOrder(invalidCtx))
	require.Equal(t, http.StatusBadRequest, invalidRec.Code)

	deniedReq := httptest.NewRequest(http.MethodPost, "/api/work-orders/"+workOrderID.String()+"/accept", strings.NewReader(`{}`))
	deniedReq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	deniedRec := httptest.NewRecorder()
	deniedCtx := e.NewContext(deniedReq, deniedRec)
	deniedCtx.SetParamNames("id")
	deniedCtx.SetParamValues(workOrderID.String())
	require.NoError(t, controller.AcceptWorkOrder(deniedCtx))
	require.Equal(t, http.StatusForbidden, deniedRec.Code)

	notFoundReq := httptest.NewRequest(http.MethodGet, "/api/work-orders/"+workOrderID.String(), nil)
	notFoundRec := httptest.NewRecorder()
	notFoundCtx := e.NewContext(notFoundReq, notFoundRec)
	notFoundCtx.SetParamNames("id")
	notFoundCtx.SetParamValues(workOrderID.String())
	controller = &ConveyorController{svc: fakeConveyorService{getWorkOrderErr: service.ErrNotFound}}
	require.NoError(t, controller.GetWorkOrder(notFoundCtx))
	require.Equal(t, http.StatusNotFound, notFoundRec.Code)

	conflictReq := httptest.NewRequest(http.MethodPost, "/api/work-orders/"+workOrderID.String()+"/complete", strings.NewReader(`{"evidence_waiver":"approved exception"}`))
	conflictReq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	conflictRec := httptest.NewRecorder()
	conflictCtx := e.NewContext(conflictReq, conflictRec)
	conflictCtx.SetParamNames("id")
	conflictCtx.SetParamValues(workOrderID.String())
	conflictCtx.Set("user_id", uuid.New().String())
	controller = &ConveyorController{svc: fakeConveyorService{mutationErr: service.ErrConflict}}
	require.NoError(t, controller.CompleteWorkOrder(conflictCtx))
	require.Equal(t, http.StatusConflict, conflictRec.Code)
}

type fakeConveyorService struct {
	listCriteriaErr       error
	authorizeErr          error
	links                 []models.TaskLink
	listLinksErr          error
	agentRuns             []models.AgentRun
	agentRun              *models.AgentRun
	getAgentRunErr        error
	mutationResult        *service.ConveyorMutationResult
	mutationErr           error
	workOrder             *models.WorkOrder
	getWorkOrderErr       error
	generatedReport       *models.GeneratedReport
	getGeneratedReportErr error
	markdown              string
	markdownErr           error
	forumDigest           *service.ForumDigestResponse
	forumDigests          []service.ForumDigestResponse
	inboxItems            []service.AgentInboxItemResponse
	inboxItem             *service.AgentInboxItemResponse
	ackInboxErr           error
	ackInboxReq           *service.AckAgentInboxItemRequest
}

func (s fakeConveyorService) AuthorizeWorkItemAccess(ctx context.Context, actor service.ConveyorActor, taskID uuid.UUID) error {
	return s.authorizeErr
}

type fakeControllerPMImportService struct {
	called  bool
	actor   service.ConveyorActor
	req     service.PMImportRequest
	summary *service.PMImportSummary
	err     error
}

func (s *fakeControllerPMImportService) ImportPMCanon(ctx context.Context, actor service.ConveyorActor, req service.PMImportRequest) (*service.PMImportSummary, error) {
	s.called = true
	s.actor = actor
	s.req = req
	return s.summary, s.err
}

func (s fakeConveyorService) CreateAcceptanceCriterion(ctx context.Context, actor service.ConveyorActor, req service.CreateAcceptanceCriterionRequest) (*service.ConveyorMutationResult, error) {
	return nil, errors.New("not implemented")
}
func (s fakeConveyorService) UpdateAcceptanceCriterionState(ctx context.Context, actor service.ConveyorActor, taskID uuid.UUID, criterionID uuid.UUID, req service.UpdateCriterionStateRequest) (*service.ConveyorMutationResult, error) {
	return nil, errors.New("not implemented")
}
func (s fakeConveyorService) AttachEvidence(ctx context.Context, actor service.ConveyorActor, req service.AttachEvidenceRequest) (*service.ConveyorMutationResult, error) {
	return nil, errors.New("not implemented")
}
func (s fakeConveyorService) RevokeEvidence(ctx context.Context, actor service.ConveyorActor, taskID uuid.UUID, evidenceID uuid.UUID, req service.RevokeEvidenceRequest) (*service.ConveyorMutationResult, error) {
	return nil, errors.New("not implemented")
}
func (s fakeConveyorService) LinkTasks(ctx context.Context, actor service.ConveyorActor, req service.LinkTasksRequest) (*service.ConveyorMutationResult, error) {
	return nil, errors.New("not implemented")
}
func (s fakeConveyorService) CloseTask(ctx context.Context, actor service.ConveyorActor, taskID uuid.UUID, req service.CloseTaskRequest) (*service.ConveyorMutationResult, error) {
	return nil, errors.New("not implemented")
}
func (s fakeConveyorService) CreateWorkOrder(ctx context.Context, actor service.ConveyorActor, req service.CreateWorkOrderRequest) (*service.ConveyorMutationResult, error) {
	if s.mutationErr != nil {
		return nil, s.mutationErr
	}
	if s.mutationResult != nil {
		return s.mutationResult, nil
	}
	return nil, errors.New("not implemented")
}
func (s fakeConveyorService) GetWorkOrder(ctx context.Context, workOrderID uuid.UUID) (*models.WorkOrder, error) {
	if s.getWorkOrderErr != nil {
		return nil, s.getWorkOrderErr
	}
	if s.workOrder != nil {
		return s.workOrder, nil
	}
	return nil, errors.New("not implemented")
}
func (s fakeConveyorService) AcceptWorkOrder(ctx context.Context, actor service.ConveyorActor, workOrderID uuid.UUID, req service.AcceptWorkOrderRequest) (*service.ConveyorMutationResult, error) {
	if s.mutationErr != nil {
		return nil, s.mutationErr
	}
	if s.mutationResult != nil {
		return s.mutationResult, nil
	}
	return nil, errors.New("not implemented")
}
func (s fakeConveyorService) RejectWorkOrder(ctx context.Context, actor service.ConveyorActor, workOrderID uuid.UUID, req service.RejectWorkOrderRequest) (*service.ConveyorMutationResult, error) {
	if s.mutationErr != nil {
		return nil, s.mutationErr
	}
	if s.mutationResult != nil {
		return s.mutationResult, nil
	}
	return nil, errors.New("not implemented")
}
func (s fakeConveyorService) CompleteWorkOrder(ctx context.Context, actor service.ConveyorActor, workOrderID uuid.UUID, req service.CompleteWorkOrderRequest) (*service.ConveyorMutationResult, error) {
	if s.mutationErr != nil {
		return nil, s.mutationErr
	}
	if s.mutationResult != nil {
		return s.mutationResult, nil
	}
	return nil, errors.New("not implemented")
}
func (s fakeConveyorService) CancelWorkOrder(ctx context.Context, actor service.ConveyorActor, workOrderID uuid.UUID, req service.CancelWorkOrderRequest) (*service.ConveyorMutationResult, error) {
	if s.mutationErr != nil {
		return nil, s.mutationErr
	}
	if s.mutationResult != nil {
		return s.mutationResult, nil
	}
	return nil, errors.New("not implemented")
}
func (s fakeConveyorService) FailWorkOrder(ctx context.Context, actor service.ConveyorActor, workOrderID uuid.UUID, req service.FailWorkOrderRequest) (*service.ConveyorMutationResult, error) {
	if s.mutationErr != nil {
		return nil, s.mutationErr
	}
	if s.mutationResult != nil {
		return s.mutationResult, nil
	}
	return nil, errors.New("not implemented")
}
func (s fakeConveyorService) RegisterAgentRun(ctx context.Context, actor service.ConveyorActor, req service.RegisterAgentRunRequest) (*service.ConveyorMutationResult, error) {
	if s.mutationResult != nil {
		return s.mutationResult, nil
	}
	return nil, errors.New("not implemented")
}
func (s fakeConveyorService) UpdateAgentRun(ctx context.Context, actor service.ConveyorActor, workItemID uuid.UUID, agentRunID uuid.UUID, req service.UpdateAgentRunRequest) (*service.ConveyorMutationResult, error) {
	if s.mutationResult != nil {
		return s.mutationResult, nil
	}
	return nil, errors.New("not implemented")
}
func (s fakeConveyorService) HeartbeatAgentRun(ctx context.Context, actor service.ConveyorActor, workItemID uuid.UUID, agentRunID uuid.UUID) (*service.ConveyorMutationResult, error) {
	if s.mutationResult != nil {
		return s.mutationResult, nil
	}
	return nil, errors.New("not implemented")
}
func (s fakeConveyorService) MarkStaleAgentRunsFailed(ctx context.Context, actor service.ConveyorActor, cutoff time.Time) ([]uuid.UUID, error) {
	return nil, errors.New("not implemented")
}
func (s fakeConveyorService) GetAgentRun(ctx context.Context, workItemID uuid.UUID, agentRunID uuid.UUID) (*models.AgentRun, error) {
	return s.agentRun, s.getAgentRunErr
}
func (s fakeConveyorService) ListAgentRuns(ctx context.Context, workItemID uuid.UUID) ([]models.AgentRun, error) {
	return s.agentRuns, nil
}
func (s fakeConveyorService) ListAgentInbox(ctx context.Context, actor service.ConveyorActor) ([]service.AgentInboxItemResponse, error) {
	return s.inboxItems, nil
}
func (s fakeConveyorService) AckAgentInboxItem(ctx context.Context, actor service.ConveyorActor, req service.AckAgentInboxItemRequest) (*service.AgentInboxItemResponse, error) {
	if s.ackInboxReq != nil {
		*s.ackInboxReq = req
	}
	if s.ackInboxErr != nil {
		return nil, s.ackInboxErr
	}
	if s.inboxItem != nil {
		return s.inboxItem, nil
	}
	return nil, errors.New("not implemented")
}
func (s fakeConveyorService) GenerateProjectReport(ctx context.Context, actor service.ConveyorActor, req service.GenerateProjectReportRequest) (*service.GeneratedReportResponse, error) {
	if s.generatedReport != nil {
		return service.GeneratedReportResponseFromModel(*s.generatedReport)
	}
	return nil, errors.New("not implemented")
}
func (s fakeConveyorService) GetGeneratedReport(ctx context.Context, reportID uuid.UUID) (*service.GeneratedReportResponse, error) {
	if s.getGeneratedReportErr != nil {
		return nil, s.getGeneratedReportErr
	}
	if s.generatedReport != nil {
		return service.GeneratedReportResponseFromModel(*s.generatedReport)
	}
	return nil, errors.New("not implemented")
}
func (s fakeConveyorService) ExportGeneratedReportMarkdown(ctx context.Context, reportID uuid.UUID) (string, error) {
	return s.markdown, s.markdownErr
}
func (s fakeConveyorService) CreateForumDigest(ctx context.Context, actor service.ConveyorActor, req service.CreateForumDigestRequest) (*service.ForumDigestResponse, error) {
	if req.SourceType != models.ConversationSourceTypeForum && req.SourceType != models.ConversationSourceTypeTelegram {
		return nil, service.ErrValidation
	}
	if !req.PeriodStart.IsZero() && !req.PeriodEnd.IsZero() && req.PeriodEnd.Before(req.PeriodStart) {
		return nil, service.ErrValidation
	}
	if s.forumDigest != nil {
		return s.forumDigest, nil
	}
	return nil, errors.New("not implemented")
}
func (s fakeConveyorService) GetForumDigest(ctx context.Context, digestID uuid.UUID) (*service.ForumDigestResponse, error) {
	if s.forumDigest != nil {
		return s.forumDigest, nil
	}
	return nil, errors.New("not implemented")
}
func (s fakeConveyorService) ListForumDigests(ctx context.Context, sourceType string, sourceID string) ([]service.ForumDigestResponse, error) {
	if sourceType != "" && sourceType != models.ConversationSourceTypeForum && sourceType != models.ConversationSourceTypeTelegram {
		return nil, service.ErrValidation
	}
	return s.forumDigests, nil
}
func (s fakeConveyorService) ConfirmForumActionCandidate(ctx context.Context, actor service.ConveyorActor, candidateID uuid.UUID, req service.ConfirmForumActionCandidateRequest) (*service.ConveyorMutationResult, error) {
	if s.mutationErr != nil {
		return nil, s.mutationErr
	}
	if s.mutationResult != nil {
		return s.mutationResult, nil
	}
	return nil, errors.New("not implemented")
}
func (s fakeConveyorService) RejectForumActionCandidate(ctx context.Context, actor service.ConveyorActor, candidateID uuid.UUID, req service.RejectForumActionCandidateRequest) (*service.ConveyorMutationResult, error) {
	if s.mutationErr != nil {
		return nil, s.mutationErr
	}
	if s.mutationResult != nil {
		return s.mutationResult, nil
	}
	return nil, errors.New("not implemented")
}

type statefulForumCandidateControllerService struct {
	fakeConveyorService
	candidateID uuid.UUID
	status      string
	mutationID  uuid.UUID
}

func (s *statefulForumCandidateControllerService) ConfirmForumActionCandidate(ctx context.Context, actor service.ConveyorActor, candidateID uuid.UUID, req service.ConfirmForumActionCandidateRequest) (*service.ConveyorMutationResult, error) {
	if candidateID != s.candidateID {
		return nil, service.ErrNotFound
	}
	if s.status != models.ForumActionCandidateStatusPending {
		return nil, service.ErrConflict
	}
	s.status = models.ForumActionCandidateStatusConfirmed
	return &service.ConveyorMutationResult{EntityID: s.mutationID, EventID: uuid.New()}, nil
}

func (s *statefulForumCandidateControllerService) RejectForumActionCandidate(ctx context.Context, actor service.ConveyorActor, candidateID uuid.UUID, req service.RejectForumActionCandidateRequest) (*service.ConveyorMutationResult, error) {
	if candidateID != s.candidateID {
		return nil, service.ErrNotFound
	}
	if s.status != models.ForumActionCandidateStatusPending {
		return nil, service.ErrConflict
	}
	s.status = models.ForumActionCandidateStatusRejected
	return &service.ConveyorMutationResult{EntityID: s.mutationID, EventID: uuid.New()}, nil
}

func (s fakeConveyorService) ListAcceptanceCriteria(ctx context.Context, taskID uuid.UUID) ([]models.AcceptanceCriterion, error) {
	return nil, s.listCriteriaErr
}
func (s fakeConveyorService) ListEvidence(ctx context.Context, taskID uuid.UUID) ([]models.Evidence, error) {
	return nil, errors.New("not implemented")
}
func (s fakeConveyorService) ListEvents(ctx context.Context, taskID uuid.UUID) ([]models.ConveyorEvent, error) {
	return nil, errors.New("not implemented")
}
func (s fakeConveyorService) ListTaskLinks(ctx context.Context, taskID uuid.UUID) ([]models.TaskLink, error) {
	return s.links, s.listLinksErr
}

func (s fakeConveyorService) CreateWaiver(ctx context.Context, actor service.ConveyorActor, req service.CreateWaiverRequest) (*service.ConveyorMutationResult, error) {
	if s.mutationErr != nil {
		return nil, s.mutationErr
	}
	if s.mutationResult != nil {
		return s.mutationResult, nil
	}
	return nil, errors.New("not implemented")
}
func (s fakeConveyorService) GetWaiver(ctx context.Context, id uuid.UUID) (*models.Waiver, error) {
	return nil, errors.New("not implemented")
}
func (s fakeConveyorService) RequestApproval(ctx context.Context, actor service.ConveyorActor, req service.RequestApprovalRequest) (*service.ConveyorMutationResult, error) {
	if s.mutationErr != nil {
		return nil, s.mutationErr
	}
	if s.mutationResult != nil {
		return s.mutationResult, nil
	}
	return nil, errors.New("not implemented")
}
func (s fakeConveyorService) GrantApproval(ctx context.Context, actor service.ConveyorActor, approvalID uuid.UUID, req service.DecideApprovalRequest) (*service.ConveyorMutationResult, error) {
	if s.mutationErr != nil {
		return nil, s.mutationErr
	}
	if s.mutationResult != nil {
		return s.mutationResult, nil
	}
	return nil, errors.New("not implemented")
}
func (s fakeConveyorService) DenyApproval(ctx context.Context, actor service.ConveyorActor, approvalID uuid.UUID, req service.DecideApprovalRequest) (*service.ConveyorMutationResult, error) {
	if s.mutationErr != nil {
		return nil, s.mutationErr
	}
	if s.mutationResult != nil {
		return s.mutationResult, nil
	}
	return nil, errors.New("not implemented")
}
func (s fakeConveyorService) SuggestCriteria(ctx context.Context, actor service.ConveyorActor, taskID uuid.UUID) (string, error) {
	if s.markdownErr != nil {
		return "", s.markdownErr
	}
	return s.markdown, nil
}
func (s fakeConveyorService) SummarizeEvidence(ctx context.Context, actor service.ConveyorActor, taskID uuid.UUID) (string, error) {
	if s.markdownErr != nil {
		return "", s.markdownErr
	}
	return s.markdown, nil
}
func (s fakeConveyorService) GetApprovalRequest(ctx context.Context, id uuid.UUID) (*models.ApprovalRequest, error) {
	return nil, errors.New("not implemented")
}
func (s fakeConveyorService) ListApprovalRequests(ctx context.Context, workItemID uuid.UUID) ([]models.ApprovalRequest, error) {
	return nil, errors.New("not implemented")
}

func (s fakeConveyorService) ListPendingApprovals(ctx context.Context, limit int) ([]models.ApprovalRequest, error) {
	return nil, errors.New("not implemented")
}
