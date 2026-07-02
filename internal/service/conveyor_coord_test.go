package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	models "emplacc-api/internal/domain"
	"emplacc-api/internal/ports"

	"github.com/google/uuid"
)

func TestDependencySignalAndAutoReadyCoord(t *testing.T) {
	ctx := context.Background()
	requestID := "req-coord-1"
	correlationID := "corr-coord-1"
	actor := ConveyorActor{ActorType: "user", ActorID: uuid.New(), Source: "rest", RequestID: &requestID, CorrelationID: &correlationID}
	repo := newCoordRepo()
	svc := NewConveyorService(repo)

	boardID := uuid.New()
	readyID := repo.addStatus(boardID, "ready", true)
	blockedID := repo.addStatus(boardID, "blocked", true)
	closedID := repo.addStatus(boardID, "done", false)
	upstreamID := repo.addTask("upstream", closedID)
	downstreamID := repo.addTask("downstream", blockedID)
	repo.addEvidence(upstreamID, models.EvidenceVerdictSupports)
	repo.addLink(upstreamID, downstreamID, models.TaskLinkTypeBlocks)

	result, err := svc.CloseTask(ctx, actor, upstreamID, CloseTaskRequest{ToStatusID: closedID, AllowDependencyAutoReady: true})
	if err != nil {
		t.Fatalf("CloseTask returned error: %v", err)
	}
	if result.EventID == uuid.Nil {
		t.Fatalf("CloseTask did not return the completion event")
	}
	if got := repo.tasks[downstreamID].StatusID; got != readyID {
		t.Fatalf("downstream status = %s, want ready %s", got, readyID)
	}
	completed := repo.findEvent(upstreamID, "task.completed")
	if completed == nil {
		t.Fatalf("task.completed event was not written")
	}
	signal := repo.findEvent(downstreamID, "dependency.signal")
	if signal == nil {
		t.Fatalf("dependency.signal event was not written")
	}
	if repo.eventIndex(completed.ID) >= repo.eventIndex(signal.ID) {
		t.Fatalf("event order = %v, want task.completed before dependency.signal", collectCoordEventTypes(repo.events))
	}
	if signal.ActorType != actor.ActorType || signal.ActorID != actor.ActorID || signal.Source != actor.Source {
		t.Fatalf("signal attribution = %#v, want actor/source attribution", signal)
	}
	if signal.RequestID == nil || *signal.RequestID != requestID || signal.CorrelationID == nil || *signal.CorrelationID != correlationID {
		t.Fatalf("signal request/correlation attribution = request:%v correlation:%v", signal.RequestID, signal.CorrelationID)
	}
	payload := map[string]any{}
	if err := json.Unmarshal(signal.Payload, &payload); err != nil {
		t.Fatalf("signal payload is invalid JSON: %v", err)
	}
	if payload["upstream_work_item_id"] != upstreamID.String() {
		t.Fatalf("signal upstream_work_item_id = %v, want %s", payload["upstream_work_item_id"], upstreamID)
	}
	if payload["completed_event_id"] != completed.ID.String() {
		t.Fatalf("signal completed_event_id = %v, want %s", payload["completed_event_id"], completed.ID)
	}
	ids, ok := payload["supporting_evidence_ids"].([]any)
	if !ok || len(ids) != 1 || ids[0] != repo.evidenceByTask[upstreamID][0].ID.String() {
		t.Fatalf("signal supporting_evidence_ids = %#v", payload["supporting_evidence_ids"])
	}
}

func TestDependencySignalSupportsBlockedByDirectionCoord(t *testing.T) {
	ctx := context.Background()
	actor := ConveyorActor{ActorType: "user", ActorID: uuid.New(), Source: "rest"}
	repo := newCoordRepo()
	svc := NewConveyorService(repo)
	boardID := uuid.New()
	readyID := repo.addStatus(boardID, "ready", true)
	blockedID := repo.addStatus(boardID, "blocked", true)
	openID := repo.addStatus(boardID, "open", true)
	closedID := repo.addStatus(boardID, "done", false)
	upstreamID := repo.addTask("upstream", openID)
	downstreamID := repo.addTask("downstream", blockedID)
	repo.addEvidence(upstreamID, models.EvidenceVerdictSupports)
	repo.addLink(downstreamID, upstreamID, models.TaskLinkTypeBlockedBy)

	_, err := svc.CloseTask(ctx, actor, upstreamID, CloseTaskRequest{ToStatusID: closedID, AllowDependencyAutoReady: true})
	if err != nil {
		t.Fatalf("CloseTask returned error: %v", err)
	}
	if got := repo.tasks[downstreamID].StatusID; got != readyID {
		t.Fatalf("downstream status = %s, want ready %s", got, readyID)
	}
	signal := repo.findEvent(downstreamID, "dependency.signal")
	if signal == nil {
		t.Fatalf("dependency.signal event was not written for blocked_by")
	}
	payload := decodeCoordPayload(t, signal)
	if payload["link_type"] != models.TaskLinkTypeBlockedBy {
		t.Fatalf("signal link_type = %v, want blocked_by", payload["link_type"])
	}
	if payload["upstream_work_item_id"] != upstreamID.String() || payload["downstream_work_item_id"] != downstreamID.String() {
		t.Fatalf("signal dependency ids = %#v", payload)
	}
}

func TestAutoReadyRequiresEveryGuardCoord(t *testing.T) {
	cases := []struct {
		name        string
		configure   func(*coordRepo, uuid.UUID, uuid.UUID, uuid.UUID)
		allowPolicy bool
	}{
		{name: "policy denied", allowPolicy: false},
		{name: "another blocker is open", allowPolicy: true, configure: func(r *coordRepo, _, downstreamID, openStatusID uuid.UUID) {
			other := r.addTask("other", openStatusID)
			r.addLink(other, downstreamID, models.TaskLinkTypeBlocks)
		}},
		{name: "downstream is not blocked", allowPolicy: true, configure: func(r *coordRepo, _, downstreamID, openStatusID uuid.UUID) {
			task := r.tasks[downstreamID]
			task.StatusID = openStatusID
			r.tasks[downstreamID] = task
		}},
		{name: "ready status missing", allowPolicy: true, configure: func(r *coordRepo, _, _, _ uuid.UUID) {
			for id, status := range r.statuses {
				if status.Name != nil && *status.Name == "ready" {
					delete(r.statuses, id)
				}
			}
		}},
		{name: "contradicting evidence exists", allowPolicy: true, configure: func(r *coordRepo, _, downstreamID, _ uuid.UUID) {
			r.addEvidence(downstreamID, models.EvidenceVerdictContradicts)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			actor := ConveyorActor{ActorType: "user", ActorID: uuid.New(), Source: "rest"}
			repo := newCoordRepo()
			svc := NewConveyorService(repo)
			boardID := uuid.New()
			repo.addStatus(boardID, "ready", true)
			blockedID := repo.addStatus(boardID, "blocked", true)
			openID := repo.addStatus(boardID, "open", true)
			closedID := repo.addStatus(boardID, "done", false)
			upstreamID := repo.addTask("upstream", openID)
			downstreamID := repo.addTask("downstream", blockedID)
			expectedStatusID := blockedID
			repo.addEvidence(upstreamID, models.EvidenceVerdictSupports)
			repo.addLink(upstreamID, downstreamID, models.TaskLinkTypeBlocks)
			if tc.configure != nil {
				tc.configure(repo, upstreamID, downstreamID, openID)
			}
			if tc.name == "downstream is not blocked" {
				expectedStatusID = openID
			}

			_, err := svc.CloseTask(ctx, actor, upstreamID, CloseTaskRequest{ToStatusID: closedID, AllowDependencyAutoReady: tc.allowPolicy})
			if err != nil {
				t.Fatalf("CloseTask returned error: %v", err)
			}
			if got := repo.tasks[downstreamID].StatusID; got != expectedStatusID {
				t.Fatalf("downstream status = %s, want %s", got, expectedStatusID)
			}
			if repo.findEvent(downstreamID, "dependency.signal") == nil {
				t.Fatalf("dependency.signal event was not written")
			}
		})
	}
}

func TestWorkOrderLifecycleCoord(t *testing.T) {
	ctx := context.Background()
	actor := ConveyorActor{ActorType: "user", ActorID: uuid.New(), Source: "rest"}
	repo := newCoordRepo()
	svc := NewConveyorService(repo)
	requesterBoardID := uuid.New()
	providerBoardID := uuid.New()
	sourceStatusID := repo.addStatus(requesterBoardID, "open", true)
	providerStatusID := repo.addStatus(providerBoardID, "ready", true)
	sourceTaskID := repo.addTask("source", sourceStatusID)

	created, err := svc.CreateWorkOrder(ctx, actor, CreateWorkOrderRequest{
		SourceTaskID:             sourceTaskID,
		ProviderBoardID:          providerBoardID,
		ProviderStatusID:         providerStatusID,
		Goal:                     "Prepare integration evidence",
		Inputs:                   json.RawMessage(`{"task":"source"}`),
		AcceptanceCriteria:       json.RawMessage(`["evidence returned"]`),
		RequiredEvidenceMetadata: json.RawMessage(`{"kind":"link"}`),
		RequesterContext:         json.RawMessage(`{"team":"requester"}`),
		ProviderContext:          json.RawMessage(`{"team":"provider"}`),
	})
	if err != nil {
		t.Fatalf("CreateWorkOrder returned error: %v", err)
	}
	workOrder, err := svc.GetWorkOrder(ctx, created.EntityID)
	if err != nil {
		t.Fatalf("GetWorkOrder returned error: %v", err)
	}
	if workOrder.Status != models.WorkOrderStatusRequested {
		t.Fatalf("initial status = %s", workOrder.Status)
	}

	accepted, err := svc.AcceptWorkOrder(ctx, actor, workOrder.ID, AcceptWorkOrderRequest{TargetName: "provider task"})
	if err != nil {
		t.Fatalf("AcceptWorkOrder returned error: %v", err)
	}
	workOrder, _ = svc.GetWorkOrder(ctx, workOrder.ID)
	if workOrder.Status != models.WorkOrderStatusAccepted || workOrder.TargetTaskID == nil {
		t.Fatalf("accepted work order = %#v", workOrder)
	}
	if accepted.EntityID != *workOrder.TargetTaskID {
		t.Fatalf("accept result entity = %s, want target task %s", accepted.EntityID, *workOrder.TargetTaskID)
	}
	if !repo.hasLink(sourceTaskID, *workOrder.TargetTaskID, models.TaskLinkTypeBlocks) {
		t.Fatalf("accept did not create source/target link")
	}

	if _, err := svc.CompleteWorkOrder(ctx, actor, workOrder.ID, CompleteWorkOrderRequest{}); !errors.Is(err, ErrValidation) {
		t.Fatalf("CompleteWorkOrder without evidence error = %v, want validation", err)
	}
	evidence := repo.addEvidence(*workOrder.TargetTaskID, models.EvidenceVerdictSupports)
	if _, err := svc.CompleteWorkOrder(ctx, actor, workOrder.ID, CompleteWorkOrderRequest{ResultEvidenceID: &evidence.ID}); err != nil {
		t.Fatalf("CompleteWorkOrder returned error: %v", err)
	}
	workOrder, _ = svc.GetWorkOrder(ctx, workOrder.ID)
	if workOrder.Status != models.WorkOrderStatusCompleted || workOrder.ResultEvidenceID == nil || *workOrder.ResultEvidenceID != evidence.ID {
		t.Fatalf("completed work order = %#v", workOrder)
	}
	if repo.findEvent(sourceTaskID, "work_order.completed") == nil {
		t.Fatalf("completion event was not written for requester")
	}
	exposed, err := svc.GetWorkOrder(ctx, workOrder.ID)
	if err != nil {
		t.Fatalf("GetWorkOrder after completion returned error: %v", err)
	}
	if exposed.ResultEvidenceID == nil || *exposed.ResultEvidenceID != evidence.ID {
		t.Fatalf("result evidence was not exposed to requester: %#v", exposed.ResultEvidenceID)
	}
}

func TestWorkOrderCompletionWithWaiverCoord(t *testing.T) {
	ctx := context.Background()
	actor := ConveyorActor{ActorType: "user", ActorID: uuid.New(), Source: "rest"}
	repo, svc, workOrderID := newWorkOrderFixture(t, actor)
	waiver := "approved exception for unavailable provider evidence"
	if _, err := svc.CompleteWorkOrder(ctx, actor, workOrderID, CompleteWorkOrderRequest{EvidenceWaiver: waiver}); err != nil {
		t.Fatalf("CompleteWorkOrder waiver path returned error: %v", err)
	}
	workOrder, err := svc.GetWorkOrder(ctx, workOrderID)
	if err != nil {
		t.Fatalf("GetWorkOrder returned error: %v", err)
	}
	if workOrder.Status != models.WorkOrderStatusCompleted || workOrder.EvidenceWaiver != waiver || workOrder.ResultEvidenceID != nil {
		t.Fatalf("waived completion work order = %#v", workOrder)
	}
	event := repo.findEvent(workOrder.SourceTaskID, "work_order.completed")
	if event == nil {
		t.Fatalf("work_order.completed event was not written")
	}
	payload := decodeCoordPayload(t, event)
	if payload["evidence_waiver"] != waiver {
		t.Fatalf("completion payload waiver = %v, want %s", payload["evidence_waiver"], waiver)
	}
}

func TestWorkOrderRejectCancelFailCoord(t *testing.T) {
	ctx := context.Background()
	actor := ConveyorActor{ActorType: "user", ActorID: uuid.New(), Source: "rest"}
	for _, transition := range []struct {
		name   string
		apply  func(ConveyorService, context.Context, ConveyorActor, uuid.UUID) error
		status string
		event  string
		reason string
	}{
		{name: "reject", status: models.WorkOrderStatusRejected, event: "work_order.rejected", reason: "not feasible", apply: func(s ConveyorService, ctx context.Context, actor ConveyorActor, id uuid.UUID) error {
			_, err := s.RejectWorkOrder(ctx, actor, id, RejectWorkOrderRequest{Reason: "not feasible"})
			return err
		}},
		{name: "cancel", status: models.WorkOrderStatusCanceled, event: "work_order.canceled", reason: "superseded", apply: func(s ConveyorService, ctx context.Context, actor ConveyorActor, id uuid.UUID) error {
			_, err := s.CancelWorkOrder(ctx, actor, id, CancelWorkOrderRequest{Reason: "superseded"})
			return err
		}},
		{name: "fail", status: models.WorkOrderStatusFailed, event: "work_order.failed", reason: "blocked", apply: func(s ConveyorService, ctx context.Context, actor ConveyorActor, id uuid.UUID) error {
			_, err := s.FailWorkOrder(ctx, actor, id, FailWorkOrderRequest{Reason: "blocked"})
			return err
		}},
	} {
		t.Run(transition.name, func(t *testing.T) {
			repo, svc, workOrderID := newWorkOrderFixture(t, actor)
			if err := transition.apply(svc, ctx, actor, workOrderID); err != nil {
				t.Fatalf("transition returned error: %v", err)
			}
			if got := repo.workOrders[workOrderID].Status; got != transition.status {
				t.Fatalf("status = %s, want %s", got, transition.status)
			}
			event := repo.findEvent(repo.workOrders[workOrderID].SourceTaskID, transition.event)
			if event == nil {
				t.Fatalf("%s event was not written", transition.event)
			}
			payload := decodeCoordPayload(t, event)
			if payload["reason"] != transition.reason || payload["work_order_id"] != workOrderID.String() {
				t.Fatalf("transition payload = %#v", payload)
			}
			if event.ActorType != actor.ActorType || event.ActorID != actor.ActorID || event.Source != actor.Source {
				t.Fatalf("transition attribution = %#v", event)
			}
		})
	}
}

func TestWorkOrderModelHasNoRemoteExecutionFieldsCoord(t *testing.T) {
	forbiddenFragments := []string{
		"capability",
		"provider_tool",
		"remote_tool",
		"tool_call",
		"automation_command",
		"mcp",
		"llm",
	}
	workOrderType := reflect.TypeOf(models.WorkOrder{})
	for i := 0; i < workOrderType.NumField(); i++ {
		field := workOrderType.Field(i)
		jsonName := strings.Split(field.Tag.Get("json"), ",")[0]
		candidates := []string{strings.ToLower(field.Name), strings.ToLower(jsonName), strings.ToLower(field.Tag.Get("gorm"))}
		for _, candidate := range candidates {
			for _, fragment := range forbiddenFragments {
				if strings.Contains(candidate, fragment) {
					t.Fatalf("work order field %s exposes remote execution fragment %s", field.Name, fragment)
				}
			}
		}
	}
	assertNoRemoteExecutionCalls(t, "Conveyor.go", []string{"CreateWorkOrder", "AcceptWorkOrder", "RejectWorkOrder", "CompleteWorkOrder", "CancelWorkOrder", "FailWorkOrder", "emitDependencySignals", "shouldAutoReady", "transitionWorkOrder"})
	assertNoRemoteExecutionCalls(t, "../transport/http/Conveyor.go", []string{"CreateWorkOrder", "GetWorkOrder", "AcceptWorkOrder", "RejectWorkOrder", "CompleteWorkOrder", "CancelWorkOrder", "FailWorkOrder"})
}

func assertNoRemoteExecutionCalls(t *testing.T, path string, functionNames []string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	for _, name := range functionNames {
		block := extractFunctionBlock(string(data), name)
		if block == "" {
			t.Fatalf("function %s not found in %s", name, path)
		}
		for _, forbidden := range []string{"ProcessTaskWithLLM", "EnrichGeneratedReport", "RegisterAgentRun", "HeartbeatAgentRun", "tools/call", "ToolCall", "Mcp", "MCP", "CapabilityRoute", "ProviderTool", "RemoteTool", "AutomationCommand"} {
			if strings.Contains(block, forbidden) {
				t.Fatalf("%s in %s contains remote execution call marker %s", name, path, forbidden)
			}
		}
	}
}

func extractFunctionBlock(source string, name string) string {
	idx := strings.Index(source, "func ")
	for idx >= 0 {
		remaining := source[idx:]
		open := strings.Index(remaining, "{")
		if open < 0 {
			return ""
		}
		header := remaining[:open]
		if strings.Contains(header, name+"(") || strings.Contains(header, name+")") {
			start := idx + open
			depth := 0
			for pos := start; pos < len(source); pos++ {
				switch source[pos] {
				case '{':
					depth++
				case '}':
					depth--
					if depth == 0 {
						return source[idx : pos+1]
					}
				}
			}
			return ""
		}
		next := strings.Index(source[idx+5:], "func ")
		if next < 0 {
			return ""
		}
		idx = idx + 5 + next
	}
	return ""
}

func newWorkOrderFixture(t *testing.T, actor ConveyorActor) (*coordRepo, ConveyorService, uuid.UUID) {
	t.Helper()
	repo := newCoordRepo()
	svc := NewConveyorService(repo)
	sourceStatusID := repo.addStatus(uuid.New(), "open", true)
	providerStatusID := repo.addStatus(uuid.New(), "ready", true)
	sourceTaskID := repo.addTask("source", sourceStatusID)
	created, err := svc.CreateWorkOrder(context.Background(), actor, CreateWorkOrderRequest{SourceTaskID: sourceTaskID, ProviderBoardID: repo.statuses[providerStatusID].BoardID, ProviderStatusID: providerStatusID, Goal: "Do work"})
	if err != nil {
		t.Fatalf("CreateWorkOrder fixture error: %v", err)
	}
	return repo, svc, created.EntityID
}

type coordRepo struct {
	tasks          map[uuid.UUID]models.Task
	statuses       map[uuid.UUID]models.Status
	criteria       map[uuid.UUID][]models.AcceptanceCriterion
	evidenceByTask map[uuid.UUID][]models.Evidence
	evidenceByID   map[uuid.UUID]models.Evidence
	events         []models.ConveyorEvent
	links          []models.TaskLink
	workOrders     map[uuid.UUID]models.WorkOrder
	idempotency    map[string]models.IdempotencyRecord
	waivers        map[uuid.UUID]models.Waiver
	approvals      map[uuid.UUID]models.ApprovalRequest
}

func newCoordRepo() *coordRepo {
	return &coordRepo{tasks: map[uuid.UUID]models.Task{}, statuses: map[uuid.UUID]models.Status{}, criteria: map[uuid.UUID][]models.AcceptanceCriterion{}, evidenceByTask: map[uuid.UUID][]models.Evidence{}, evidenceByID: map[uuid.UUID]models.Evidence{}, workOrders: map[uuid.UUID]models.WorkOrder{}, idempotency: map[string]models.IdempotencyRecord{}, waivers: map[uuid.UUID]models.Waiver{}, approvals: map[uuid.UUID]models.ApprovalRequest{}}
}

func (r *coordRepo) CreateWaiver(ctx context.Context, waiver *models.Waiver) error {
	r.waivers[waiver.ID] = *waiver
	return nil
}

func (r *coordRepo) GetWaiver(ctx context.Context, id uuid.UUID) (*models.Waiver, error) {
	if waiver, ok := r.waivers[id]; ok {
		return &waiver, nil
	}
	return nil, models.ErrConveyorNotFound
}

func (r *coordRepo) CreateApprovalRequest(ctx context.Context, request *models.ApprovalRequest) error {
	r.approvals[request.ID] = *request
	return nil
}

func (r *coordRepo) GetApprovalRequest(ctx context.Context, id uuid.UUID) (*models.ApprovalRequest, error) {
	if request, ok := r.approvals[id]; ok {
		return &request, nil
	}
	return nil, models.ErrConveyorNotFound
}

func (r *coordRepo) UpdateApprovalRequest(ctx context.Context, request *models.ApprovalRequest) error {
	if _, ok := r.approvals[request.ID]; !ok {
		return models.ErrConveyorNotFound
	}
	r.approvals[request.ID] = *request
	return nil
}

func (r *coordRepo) ListApprovalRequests(ctx context.Context, workItemID uuid.UUID) ([]models.ApprovalRequest, error) {
	var out []models.ApprovalRequest
	for _, request := range r.approvals {
		if request.WorkItemID != nil && *request.WorkItemID == workItemID {
			out = append(out, request)
		}
	}
	return out, nil
}

func (r *coordRepo) ListPendingApprovals(ctx context.Context, limit int) ([]models.ApprovalRequest, error) {
	var out []models.ApprovalRequest
	for _, request := range r.approvals {
		if request.Status == models.ApprovalStatusPending {
			out = append(out, request)
		}
	}
	return out, nil
}

func (r *coordRepo) addStatus(boardID uuid.UUID, name string, open bool) uuid.UUID {
	id := uuid.New()
	r.statuses[id] = models.Status{ID: id, BoardID: boardID, Name: &name, IsOpen: &open, Deleted: ptrBool(false)}
	return id
}

func (r *coordRepo) addTask(name string, statusID uuid.UUID) uuid.UUID {
	id := uuid.New()
	now := time.Now()
	r.tasks[id] = models.Task{ID: id, Name: &name, StatusID: statusID, Deleted: ptrBool(false), CreatedAt: &now}
	return id
}

func (r *coordRepo) addEvidence(taskID uuid.UUID, verdict string) models.Evidence {
	e := models.Evidence{ID: uuid.New(), TaskID: taskID, Type: models.EvidenceTypeLink, Verdict: verdict, URI: "https://example.test/evidence", Title: "evidence", CreatedBy: uuid.New(), CreatedAt: time.Now()}
	r.evidenceByTask[taskID] = append(r.evidenceByTask[taskID], e)
	r.evidenceByID[e.ID] = e
	return e
}

func (r *coordRepo) addLink(sourceID uuid.UUID, targetID uuid.UUID, linkType string) {
	r.links = append(r.links, models.TaskLink{ID: uuid.New(), SourceTaskID: sourceID, TargetTaskID: targetID, LinkType: linkType, CreatedBy: uuid.New(), CreatedAt: time.Now()})
}

func (r *coordRepo) findEvent(taskID uuid.UUID, eventType string) *models.ConveyorEvent {
	for i := range r.events {
		if r.events[i].WorkItemID == taskID && r.events[i].Type == eventType {
			return &r.events[i]
		}
	}
	return nil
}

func (r *coordRepo) eventIndex(eventID uuid.UUID) int {
	for i := range r.events {
		if r.events[i].ID == eventID {
			return i
		}
	}
	return -1
}

func collectCoordEventTypes(events []models.ConveyorEvent) []string {
	out := make([]string, 0, len(events))
	for _, event := range events {
		out = append(out, event.Type)
	}
	return out
}

func decodeCoordPayload(t *testing.T, event *models.ConveyorEvent) map[string]any {
	t.Helper()
	payload := map[string]any{}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatalf("event payload is invalid JSON: %v", err)
	}
	return payload
}

func (r *coordRepo) hasLink(sourceID uuid.UUID, targetID uuid.UUID, linkType string) bool {
	for _, link := range r.links {
		if link.SourceTaskID == sourceID && link.TargetTaskID == targetID && link.LinkType == linkType {
			return true
		}
	}
	return false
}

func ptrBool(v bool) *bool { return &v }

func (r *coordRepo) WithTransaction(ctx context.Context, fn func(ConveyorRepository) error) error {
	return fn(r)
}
func (r *coordRepo) GetTask(ctx context.Context, id uuid.UUID) (*models.Task, error) {
	task, ok := r.tasks[id]
	if !ok {
		return nil, models.ErrConveyorNotFound
	}
	return &task, nil
}
func (r *coordRepo) ActorCanAccessTask(ctx context.Context, actorID uuid.UUID, taskID uuid.UUID) (bool, error) {
	if _, ok := r.tasks[taskID]; !ok {
		return false, models.ErrConveyorNotFound
	}
	return true, nil
}
func (r *coordRepo) GetStatus(ctx context.Context, id uuid.UUID) (*models.Status, error) {
	status, ok := r.statuses[id]
	if !ok {
		return nil, models.ErrConveyorNotFound
	}
	return &status, nil
}
func (r *coordRepo) UpdateTaskStatus(ctx context.Context, taskID uuid.UUID, statusID uuid.UUID, at time.Time) error {
	task, ok := r.tasks[taskID]
	if !ok {
		return models.ErrConveyorNotFound
	}
	task.StatusID = statusID
	task.UpdatedAt = &at
	r.tasks[taskID] = task
	return nil
}
func (r *coordRepo) CreateAcceptanceCriterion(ctx context.Context, criterion *models.AcceptanceCriterion) error {
	r.criteria[criterion.TaskID] = append(r.criteria[criterion.TaskID], *criterion)
	return nil
}
func (r *coordRepo) GetAcceptanceCriterion(ctx context.Context, id uuid.UUID) (*models.AcceptanceCriterion, error) {
	return nil, models.ErrConveyorNotFound
}
func (r *coordRepo) UpdateAcceptanceCriterionState(ctx context.Context, id uuid.UUID, state string, at time.Time) error {
	return nil
}
func (r *coordRepo) ListAcceptanceCriteria(ctx context.Context, taskID uuid.UUID) ([]models.AcceptanceCriterion, error) {
	return r.criteria[taskID], nil
}
func (r *coordRepo) CreateEvidence(ctx context.Context, evidence *models.Evidence) error {
	r.evidenceByTask[evidence.TaskID] = append(r.evidenceByTask[evidence.TaskID], *evidence)
	r.evidenceByID[evidence.ID] = *evidence
	return nil
}
func (r *coordRepo) GetEvidence(ctx context.Context, id uuid.UUID) (*models.Evidence, error) {
	evidence, ok := r.evidenceByID[id]
	if !ok {
		return nil, models.ErrConveyorNotFound
	}
	return &evidence, nil
}
func (r *coordRepo) RevokeEvidence(ctx context.Context, id uuid.UUID, actorID uuid.UUID, reason string, at time.Time) error {
	return nil
}
func (r *coordRepo) ListEvidence(ctx context.Context, taskID uuid.UUID) ([]models.Evidence, error) {
	return r.evidenceByTask[taskID], nil
}
func (r *coordRepo) CreateEvent(ctx context.Context, event *models.ConveyorEvent) error {
	r.events = append(r.events, *event)
	return nil
}
func (r *coordRepo) ListEvents(ctx context.Context, taskID uuid.UUID) ([]models.ConveyorEvent, error) {
	var out []models.ConveyorEvent
	for _, event := range r.events {
		if event.WorkItemID == taskID {
			out = append(out, event)
		}
	}
	return out, nil
}
func (r *coordRepo) CreateTaskLink(ctx context.Context, link *models.TaskLink) error {
	r.links = append(r.links, *link)
	return nil
}
func (r *coordRepo) TaskLinkExists(ctx context.Context, sourceID uuid.UUID, targetID uuid.UUID, linkType string) (bool, error) {
	return r.hasLink(sourceID, targetID, linkType), nil
}
func (r *coordRepo) ListTaskLinks(ctx context.Context, taskID uuid.UUID) ([]models.TaskLink, error) {
	var out []models.TaskLink
	for _, link := range r.links {
		if link.SourceTaskID == taskID || link.TargetTaskID == taskID {
			out = append(out, link)
		}
	}
	return out, nil
}
func (r *coordRepo) ListDownstreamDependencies(ctx context.Context, upstreamID uuid.UUID) ([]models.TaskLink, error) {
	var out []models.TaskLink
	for _, link := range r.links {
		if (link.SourceTaskID == upstreamID && link.LinkType == models.TaskLinkTypeBlocks) || (link.TargetTaskID == upstreamID && link.LinkType == models.TaskLinkTypeBlockedBy) {
			out = append(out, link)
		}
	}
	return out, nil
}
func (r *coordRepo) ListUpstreamDependencies(ctx context.Context, downstreamID uuid.UUID) ([]models.TaskLink, error) {
	var out []models.TaskLink
	for _, link := range r.links {
		if (link.TargetTaskID == downstreamID && link.LinkType == models.TaskLinkTypeBlocks) || (link.SourceTaskID == downstreamID && link.LinkType == models.TaskLinkTypeBlockedBy) {
			out = append(out, link)
		}
	}
	return out, nil
}
func (r *coordRepo) FindStatusByBoardName(ctx context.Context, boardID uuid.UUID, name string) (*models.Status, error) {
	for _, status := range r.statuses {
		if status.BoardID == boardID && status.Name != nil && *status.Name == name {
			copy := status
			return &copy, nil
		}
	}
	return nil, models.ErrConveyorNotFound
}
func (r *coordRepo) CreateWorkOrder(ctx context.Context, workOrder *models.WorkOrder) error {
	r.workOrders[workOrder.ID] = *workOrder
	return nil
}
func (r *coordRepo) GetWorkOrder(ctx context.Context, id uuid.UUID) (*models.WorkOrder, error) {
	workOrder, ok := r.workOrders[id]
	if !ok {
		return nil, models.ErrConveyorNotFound
	}
	return &workOrder, nil
}
func (r *coordRepo) UpdateWorkOrder(ctx context.Context, workOrder *models.WorkOrder) error {
	if _, ok := r.workOrders[workOrder.ID]; !ok {
		return models.ErrConveyorNotFound
	}
	r.workOrders[workOrder.ID] = *workOrder
	return nil
}
func (r *coordRepo) CreateTask(ctx context.Context, task *models.Task) error {
	r.tasks[task.ID] = *task
	return nil
}
func (r *coordRepo) CreateAgentRun(ctx context.Context, run *models.AgentRun) error { return nil }
func (r *coordRepo) GetAgentRun(ctx context.Context, id uuid.UUID) (*models.AgentRun, error) {
	return nil, models.ErrConveyorNotFound
}
func (r *coordRepo) ListAgentRuns(ctx context.Context, workItemID uuid.UUID) ([]models.AgentRun, error) {
	return nil, nil
}
func (r *coordRepo) ListStaleAgentRuns(ctx context.Context, cutoff time.Time) ([]models.AgentRun, error) {
	return nil, nil
}
func (r *coordRepo) UpdateAgentRun(ctx context.Context, run *models.AgentRun) error { return nil }
func (r *coordRepo) CreateAgentInboxItem(ctx context.Context, item *models.AgentInboxItem) error {
	return nil
}
func (r *coordRepo) ListAgentInboxItems(ctx context.Context, recipientID uuid.UUID) ([]models.AgentInboxItem, error) {
	return nil, nil
}
func (r *coordRepo) GetAgentInboxItem(ctx context.Context, id uuid.UUID) (*models.AgentInboxItem, error) {
	return nil, models.ErrConveyorNotFound
}
func (r *coordRepo) UpdateAgentInboxItemAck(ctx context.Context, itemID uuid.UUID, recipientID uuid.UUID, state string, at time.Time) error {
	return models.ErrConveyorNotFound
}
func (r *coordRepo) ListProjectReportSources(ctx context.Context, projectID uuid.UUID, start time.Time, end time.Time) ([]ports.ProjectReportSource, error) {
	return nil, nil
}
func (r *coordRepo) CreateGeneratedReport(ctx context.Context, report *models.GeneratedReport) error {
	return nil
}
func (r *coordRepo) GetGeneratedReport(ctx context.Context, id uuid.UUID) (*models.GeneratedReport, error) {
	return nil, models.ErrConveyorNotFound
}
func (r *coordRepo) UpdateGeneratedReportLLM(ctx context.Context, id uuid.UUID, draft json.RawMessage, llmError string, at time.Time) error {
	return nil
}
func (r *coordRepo) CreateForumDigest(ctx context.Context, digest *models.ForumDigest) error {
	return nil
}
func (r *coordRepo) GetForumDigest(ctx context.Context, id uuid.UUID) (*models.ForumDigest, error) {
	return nil, models.ErrConveyorNotFound
}
func (r *coordRepo) ListForumDigests(ctx context.Context, sourceType string, sourceID string) ([]models.ForumDigest, error) {
	return nil, nil
}
func (r *coordRepo) CreateForumActionCandidate(ctx context.Context, candidate *models.ForumActionCandidate) error {
	return nil
}
func (r *coordRepo) GetForumActionCandidate(ctx context.Context, id uuid.UUID) (*models.ForumActionCandidate, error) {
	return nil, models.ErrConveyorNotFound
}
func (r *coordRepo) ListForumActionCandidates(ctx context.Context, digestID uuid.UUID) ([]models.ForumActionCandidate, error) {
	return nil, nil
}
func (r *coordRepo) UpdateForumActionCandidate(ctx context.Context, candidate *models.ForumActionCandidate) error {
	return nil
}
func (r *coordRepo) GetIdempotencyRecord(ctx context.Context, actorID uuid.UUID, operation string, key string) (*models.IdempotencyRecord, error) {
	record, ok := r.idempotency[actorID.String()+operation+key]
	if !ok {
		return nil, models.ErrConveyorNotFound
	}
	return &record, nil
}
func (r *coordRepo) CreateIdempotencyRecord(ctx context.Context, record *models.IdempotencyRecord) error {
	r.idempotency[record.ActorID.String()+record.Operation+record.IdempotencyKey] = *record
	return nil
}
