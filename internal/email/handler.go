package email

import (
	"context"
	"fmt"

	"github.com/Ayush1388/auctionEngine/internal/outbox"
)

// EventHandler turns outbox events into emails. It is registered with the
// outbox Router in cmd/api/main.go for the email.activation event type.
//
// Delivery is at-least-once: if the process crashes after SMTP accepted the
// message but before the event was marked processed, the email is sent
// again on restart. For an activation link that is harmless.
type EventHandler struct {
	service *Service
}

func NewEventHandler(service *Service) *EventHandler {
	return &EventHandler{
		service: service,
	}
}

func (h *EventHandler) Handle(
	ctx context.Context,
	event outbox.Event,
) error {
	switch event.EventType {
	case EventTypeActivationEmail:
		return h.handleActivationEmail(ctx, event)

	default:
		return fmt.Errorf(
			"unknown outbox event type: %s",
			event.EventType,
		)
	}
}

func (h *EventHandler) handleActivationEmail(
	ctx context.Context,
	event outbox.Event,
) error {
	var payload ActivationEmailEvent

	if err := outbox.DecodePayload(
		event,
		&payload,
	); err != nil {
		return err
	}

	if payload.To == "" {
		return fmt.Errorf(
			"activation email recipient is empty",
		)
	}

	if payload.ActivationToken == "" {
		return fmt.Errorf(
			"activation token is empty",
		)
	}

	if err := h.service.SendActivationEmail(
		ctx,
		payload.To,
		payload.ActivationToken,
	); err != nil {
		return err
	}

	return nil
}
