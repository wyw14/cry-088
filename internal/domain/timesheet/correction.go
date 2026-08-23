package timesheet

import (
	"strings"
	"time"

	"github.com/wyw14/cry-088/internal/domain/shared"
)

type CorrectionState string

const (
	CorrectionPending  CorrectionState = "pending"
	CorrectionApproved CorrectionState = "approved"
	CorrectionRejected CorrectionState = "rejected"
)

type Correction struct {
	ID                 string
	OrganizationID     string
	OriginalEntryID    string
	ReplacementEntryID string
	RequestedBy        string
	Reason             string
	State              CorrectionState
	ReviewedBy         string
	RequestedAt        time.Time
	ReviewedAt         *time.Time
	Version            int64
}

func NewCorrection(id, organizationID, originalID, replacementID, requestedBy, reason string, now time.Time) (*Correction, error) {
	reason = strings.TrimSpace(reason)
	if id == "" || organizationID == "" || originalID == "" || replacementID == "" || requestedBy == "" {
		return nil, shared.New(shared.CodeInvalid, "correction identity is incomplete")
	}
	if originalID == replacementID || len(reason) < 5 {
		return nil, shared.New(shared.CodeInvalid, "correction replacement and reason are invalid")
	}
	return &Correction{ID: id, OrganizationID: organizationID, OriginalEntryID: originalID, ReplacementEntryID: replacementID, RequestedBy: requestedBy, Reason: reason, State: CorrectionPending, RequestedAt: now.UTC(), Version: 1}, nil
}

func (c *Correction) Approve(reviewerID string, expectedVersion int64, now time.Time) error {
	if c.Version != expectedVersion {
		return shared.New(shared.CodeConflict, "correction was changed")
	}
	if c.State != CorrectionPending || reviewerID == "" || reviewerID == c.RequestedBy {
		return shared.New(shared.CodeIllegalState, "correction cannot be approved")
	}
	reviewed := now.UTC()
	c.State = CorrectionApproved
	c.ReviewedBy = reviewerID
	c.ReviewedAt = &reviewed
	c.Version++
	return nil
}
