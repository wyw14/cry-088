package audit

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/wyw14/cry-088/internal/domain/shared"
)

type Event struct {
	ID             string
	OrganizationID string
	ActorID        string
	Source         string
	EntityType     string
	EntityID       string
	Action         string
	Before         json.RawMessage
	After          json.RawMessage
	Reason         string
	OccurredAt     time.Time
	RequestID      string
}

func NewEvent(id, organizationID, actorID, source, entityType, entityID, action, reason, requestID string, before, after any, now time.Time) (Event, error) {
	if id == "" || organizationID == "" || actorID == "" || source == "" || entityType == "" || entityID == "" || action == "" {
		return Event{}, shared.New(shared.CodeInvalid, "audit event identity is incomplete")
	}
	if strings.TrimSpace(reason) == "" {
		return Event{}, shared.Field(shared.CodeInvalid, "audit reason is required", "reason", "required")
	}
	beforeJSON, err := json.Marshal(before)
	if err != nil {
		return Event{}, shared.Wrap(shared.CodeInvalid, "cannot encode audit before state", err)
	}
	afterJSON, err := json.Marshal(after)
	if err != nil {
		return Event{}, shared.Wrap(shared.CodeInvalid, "cannot encode audit after state", err)
	}
	return Event{ID: id, OrganizationID: organizationID, ActorID: actorID, Source: source, EntityType: entityType, EntityID: entityID, Action: action, Before: beforeJSON, After: afterJSON, Reason: strings.TrimSpace(reason), OccurredAt: now.UTC(), RequestID: requestID}, nil
}
