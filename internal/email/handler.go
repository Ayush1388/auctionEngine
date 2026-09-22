package email

import (
	"context"
	"fmt"

	"github.com/Ayush1338/auctionEngine/internal/outbox"
)

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
		return h.handleActivationEmail(event)

	default:
		return fmt.Errorf(
			"unknown outbox event type: %s",
			event.EventType,
		)
	}
}

func (h *EventHandler) handleActivationEmail(
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
		payload.To,
		payload.ActivationToken,
	); err != nil {
		return err
	}

	return nil
}
