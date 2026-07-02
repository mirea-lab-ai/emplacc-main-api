package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	models "emplacc-api/internal/domain"
	"emplacc-api/internal/service"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

func TestConveyorControllerListsAgentInboxForCurrentActor(t *testing.T) {
	e := echo.New()
	actorID := uuid.New()
	workItemID := uuid.New()
	itemID := uuid.New()
	now := time.Date(2026, 6, 22, 10, 0, 0, 0, time.UTC)
	req := httptest.NewRequest(http.MethodGet, "/api/conveyor/inbox", nil)
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)
	ctx.Set("user_id", actorID.String())
	controller := &ConveyorController{svc: fakeConveyorService{inboxItems: []service.AgentInboxItemResponse{{ID: itemID, WorkItemID: &workItemID, Kind: "question", Source: "forum", Title: "Question", Summary: "Please answer", Priority: 3, ActionRequired: true, AckState: models.AgentInboxAckStateDelivered, CreatedAt: now, UpdatedAt: now}}}}

	err := controller.ListAgentInbox(ctx)

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `[{"id":"`+itemID.String()+`","work_item_id":"`+workItemID.String()+`","kind":"question","source":"forum","title":"Question","summary":"Please answer","priority":3,"action_required":true,"ack_state":"delivered","created_at":"2026-06-22T10:00:00Z","updated_at":"2026-06-22T10:00:00Z"}]`, strings.TrimSpace(rec.Body.String()))
}

func TestConveyorControllerAcksAgentInboxItemForCurrentActor(t *testing.T) {
	e := echo.New()
	actorID := uuid.New()
	itemID := uuid.New()
	body := strings.NewReader(`{"state":"read","idempotency_key":"body-key"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/conveyor/inbox/"+itemID.String()+"/ack", body)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set("Idempotency-Key", "header-key")
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)
	ctx.Set("user_id", actorID.String())
	ctx.SetParamNames("item_id")
	ctx.SetParamValues(itemID.String())
	var ackReq service.AckAgentInboxItemRequest
	controller := &ConveyorController{svc: fakeConveyorService{inboxItem: &service.AgentInboxItemResponse{ID: itemID, Kind: "question", Source: "forum", Title: "Question", AckState: models.AgentInboxAckStateRead}, ackInboxReq: &ackReq}}

	err := controller.AckAgentInboxItem(ctx)

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "header-key", ackReq.IdempotencyKey)
	require.JSONEq(t, `{"id":"`+itemID.String()+`","kind":"question","source":"forum","title":"Question","summary":"","priority":0,"action_required":false,"ack_state":"read","created_at":"0001-01-01T00:00:00Z","updated_at":"0001-01-01T00:00:00Z"}`, strings.TrimSpace(rec.Body.String()))
}

func TestConveyorControllerRejectsInvalidAgentInboxAckPayload(t *testing.T) {
	e := echo.New()
	itemID := uuid.New()
	body := strings.NewReader(`{"state":"ignored"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/conveyor/inbox/"+itemID.String()+"/ack", body)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)
	ctx.Set("user_id", uuid.New().String())
	ctx.SetParamNames("item_id")
	ctx.SetParamValues(itemID.String())
	controller := &ConveyorController{svc: fakeConveyorService{ackInboxErr: service.ErrValidation}}

	err := controller.AckAgentInboxItem(ctx)

	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.JSONEq(t, `{"error":"validation_error"}`, rec.Body.String())
}
