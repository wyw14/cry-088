package timesheet

import (
	"strings"
	"time"

	"github.com/wyw14/cry-088/internal/domain/shared"
)

func (e *Entry) Submit(expectedVersion int64, now time.Time) error {
	if e.Version != expectedVersion {
		return shared.New(shared.CodeConflict, "time entry was changed")
	}
	if e.State != EntryDraft && e.State != EntryReturned {
		return shared.New(shared.CodeIllegalState, "time entry cannot be submitted")
	}
	if strings.TrimSpace(e.Description) == "" || e.Minutes <= 0 {
		return shared.New(shared.CodeInvalid, "time entry is incomplete")
	}
	e.State = EntrySubmitted
	e.ReturnReason = ""
	e.Version++
	e.UpdatedAt = now.UTC()
	return nil
}

func (e *Entry) Approve(reviewerID string, expectedVersion int64, now time.Time) error {
	if e.Version != expectedVersion {
		return shared.New(shared.CodeConflict, "time entry was changed")
	}
	if e.State != EntrySubmitted {
		return shared.New(shared.CodeIllegalState, "only submitted entries can be approved")
	}
	if reviewerID == "" || reviewerID == e.EmployeeID {
		return shared.New(shared.CodeForbidden, "reviewer must be a different authorized employee")
	}
	approved := now.UTC()
	e.State = EntryApproved
	e.ApprovedBy = reviewerID
	e.ApprovedAt = &approved
	e.Version++
	e.UpdatedAt = approved
	return nil
}

func (e *Entry) Return(reviewerID, reason string, expectedVersion int64, now time.Time) error {
	if e.Version != expectedVersion {
		return shared.New(shared.CodeConflict, "time entry was changed")
	}
	if e.State != EntrySubmitted {
		return shared.New(shared.CodeIllegalState, "only submitted entries can be returned")
	}
	reason = strings.TrimSpace(reason)
	if reviewerID == "" || len(reason) < 3 {
		return shared.New(shared.CodeInvalid, "return reviewer and reason are required")
	}
	e.State = EntryReturned
	e.ReturnReason = reason
	e.ApprovedBy = ""
	e.ApprovedAt = nil
	e.Version++
	e.UpdatedAt = now.UTC()
	return nil
}

func (e *Entry) Lock(expectedVersion int64, now time.Time) error {
	if e.Version != expectedVersion {
		return shared.New(shared.CodeConflict, "time entry was changed")
	}
	if e.State != EntryApproved {
		return shared.New(shared.CodeIllegalState, "only approved entries can be locked")
	}
	locked := now.UTC()
	e.State = EntryLocked
	e.LockedAt = &locked
	e.Version++
	e.UpdatedAt = locked
	return nil
}

func (e *Entry) Reverse(reversalID, correctionID string, expectedVersion int64, now time.Time) error {
	if e.Version != expectedVersion {
		return shared.New(shared.CodeConflict, "time entry was changed")
	}
	if e.State != EntryLocked {
		return shared.New(shared.CodeIllegalState, "only locked entries can be reversed")
	}
	if reversalID == "" || correctionID == "" {
		return shared.New(shared.CodeInvalid, "reversal and correction identifiers are required")
	}
	e.State = EntryReversed
	e.ReversalID = reversalID
	e.CorrectionID = correctionID
	e.Version++
	e.UpdatedAt = now.UTC()
	return nil
}

type CorrectionReview struct {
	Original    Entry
	Replacement Entry
	Correction  Correction
	ReviewerID  string
	ReviewedAt  time.Time
}

func BeginCorrectionReview(original, replacement Entry, correction Correction, organizationID, reviewerID string, now time.Time) (*CorrectionReview, error) {
	if organizationID == "" || reviewerID == "" {
		return nil, shared.New(shared.CodeInvalid, "correction review identity is incomplete")
	}
	if correction.State != CorrectionPending {
		return nil, shared.New(shared.CodeIllegalState, "only pending corrections can be reviewed")
	}
	if correction.OrganizationID != organizationID || original.OrganizationID != organizationID || replacement.OrganizationID != organizationID {
		return nil, shared.New(shared.CodeForbidden, "correction records belong to another organization")
	}
	if correction.OriginalEntryID != original.ID || correction.ReplacementEntryID != replacement.ID {
		return nil, shared.New(shared.CodeInvalid, "correction entry references are inconsistent")
	}
	if original.EmployeeID != replacement.EmployeeID || original.ProjectID != replacement.ProjectID {
		return nil, shared.New(shared.CodeInvalid, "replacement ownership does not match locked entry")
	}
	if original.State != EntryLocked {
		return nil, shared.New(shared.CodeIllegalState, "correction original must be locked")
	}
	if replacement.State != EntryDraft || replacement.Minutes <= 0 || strings.TrimSpace(replacement.Description) == "" {
		return nil, shared.New(shared.CodeInvalid, "replacement entry is not ready for correction")
	}
	return &CorrectionReview{
		Original:    original,
		Replacement: replacement,
		Correction:  correction,
		ReviewerID:  reviewerID,
		ReviewedAt:  now.UTC(),
	}, nil
}

func (r *CorrectionReview) Apply() error {
	correctionVersion := r.Correction.Version
	if err := r.Correction.Approve(r.ReviewerID, correctionVersion, r.ReviewedAt); err != nil {
		return err
	}
	r.Original.TaskID = r.Replacement.TaskID
	r.Original.WorkDate = r.Replacement.WorkDate
	r.Original.StartMinute = r.Replacement.StartMinute
	r.Original.Minutes = r.Replacement.Minutes
	r.Original.Description = r.Replacement.Description
	r.Original.CorrectionID = r.Correction.ID
	r.Original.ReturnReason = ""
	r.Original.ReversalID = ""
	r.Original.Version++
	r.Original.UpdatedAt = r.ReviewedAt
	return nil
}
