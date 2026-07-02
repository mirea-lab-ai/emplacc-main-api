package httpapi

import (
	"context"
	"net/http"

	"emplacc-api/internal/service"

	"github.com/labstack/echo/v4"
)

type conveyorInboxService interface {
	ListAgentInbox(ctx context.Context, actor service.ConveyorActor) ([]service.AgentInboxItemResponse, error)
	AckAgentInboxItem(ctx context.Context, actor service.ConveyorActor, req service.AckAgentInboxItemRequest) (*service.AgentInboxItemResponse, error)
}

type conveyorInboxAckRequest struct {
	State          string `json:"state"`
	IdempotencyKey string `json:"idempotency_key" example:"inbox-ack-1"`
}

// ListAgentInbox godoc
// @Summary List current actor Conveyor inbox items
// @Tags conveyor
// @Produce json
// @Success 200 {array} service.AgentInboxItemResponse
// @Failure 400 {object} conveyorErrorResponse
// @Router /api/conveyor/inbox [get]
// @Security BearerAuth
func (c *ConveyorController) ListAgentInbox(ctx echo.Context) error {
	actor, err := actorFromContext(ctx)
	if err != nil {
		return conveyorError(ctx, err)
	}
	svc, ok := c.svc.(conveyorInboxService)
	if !ok {
		return conveyorError(ctx, service.ErrValidation)
	}
	items, err := svc.ListAgentInbox(ctx.Request().Context(), actor)
	if err != nil {
		return conveyorError(ctx, err)
	}
	return ctx.JSON(http.StatusOK, items)
}

// AckAgentInboxItem godoc
// @Summary Ack a current actor Conveyor inbox item
// @Tags conveyor
// @Accept json
// @Produce json
// @Param item_id path string true "Inbox item ID"
// @Param input body conveyorInboxAckRequest true "Ack state"
// @Success 200 {object} service.AgentInboxItemResponse
// @Failure 400 {object} conveyorErrorResponse
// @Router /api/conveyor/inbox/{item_id}/ack [post]
// @Security BearerAuth
func (c *ConveyorController) AckAgentInboxItem(ctx echo.Context) error {
	itemID, err := parseUUIDParam(ctx, "item_id")
	if err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	actor, err := actorFromContext(ctx)
	if err != nil {
		return conveyorError(ctx, err)
	}
	var req conveyorInboxAckRequest
	if err := ctx.Bind(&req); err != nil {
		return conveyorError(ctx, service.ErrValidation)
	}
	svc, ok := c.svc.(conveyorInboxService)
	if !ok {
		return conveyorError(ctx, service.ErrValidation)
	}
	item, err := svc.AckAgentInboxItem(ctx.Request().Context(), actor, service.AckAgentInboxItemRequest{ItemID: itemID, State: req.State, IdempotencyKey: idempotencyKey(ctx, req.IdempotencyKey)})
	if err != nil {
		return conveyorError(ctx, err)
	}
	return ctx.JSON(http.StatusOK, item)
}
