package timesheet

import (
	"strings"
	"time"

	"github.com/wyw14/cry-088/internal/domain/shared"
)

type EntryState string

const (
	EntryDraft     EntryState = "draft"
	EntrySubmitted EntryState = "submitted"
	EntryApproved  EntryState = "approved"
	EntryReturned  EntryState = "returned"
	EntryLocked    EntryState = "locked"
	EntryReversed  EntryState = "reversed"
)

type Entry struct {
	ID             string
	OrganizationID string
	EmployeeID     string
	ProjectID      string
	TaskID         string
	WorkDate       time.Time
	StartMinute    int
	Minutes        int
	Description    string
	State          EntryState
	ReturnReason   string
	ApprovedBy     string
	ApprovedAt     *time.Time
	LockedAt       *time.Time
	ReversalID     string
	CorrectionID   string
	Version        int64
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func NewEntry(id, organizationID, employeeID, projectID, taskID string, day time.Time, startMinute, minutes int, description string, now time.Time) (*Entry, error) {
	if id == "" || organizationID == "" || employeeID == "" || projectID == "" || taskID == "" {
		return nil, shared.New(shared.CodeInvalid, "time entry identity is incomplete")
	}
	if startMinute < 0 || startMinute >= 1440 || minutes <= 0 || startMinute+minutes > 1440 {
		return nil, shared.New(shared.CodeInvalid, "time entry range is outside the work day")
	}
	description = strings.TrimSpace(description)
	if len(description) < 3 || len(description) > 1000 {
		return nil, shared.Field(shared.CodeInvalid, "work description length is invalid", "description", "must contain 3 to 1000 characters")
	}
	day = time.Date(day.UTC().Year(), day.UTC().Month(), day.UTC().Day(), 0, 0, 0, 0, time.UTC)
	now = now.UTC()
	return &Entry{ID: id, OrganizationID: organizationID, EmployeeID: employeeID, ProjectID: projectID, TaskID: taskID, WorkDate: day, StartMinute: startMinute, Minutes: minutes, Description: description, State: EntryDraft, Version: 1, CreatedAt: now, UpdatedAt: now}, nil
}

func (e Entry) EndMinute() int { return e.StartMinute + e.Minutes }

func (e Entry) Overlaps(other Entry) bool {
	if e.EmployeeID != other.EmployeeID || !e.WorkDate.Equal(other.WorkDate) || e.State == EntryReversed || other.State == EntryReversed {
		return false
	}
	return e.StartMinute < other.EndMinute() && other.StartMinute < e.EndMinute()
}

func (e *Entry) Update(minutes int, description string, expectedVersion int64, now time.Time) error {
	if e.Version != expectedVersion {
		return shared.New(shared.CodeConflict, "time entry was changed")
	}
	if e.State != EntryDraft && e.State != EntryReturned {
		return shared.New(shared.CodeIllegalState, "only draft or returned entries can be edited")
	}
	if minutes <= 0 || e.StartMinute+minutes > 1440 {
		return shared.New(shared.CodeInvalid, "time entry range is outside the work day")
	}
	description = strings.TrimSpace(description)
	if len(description) < 3 || len(description) > 1000 {
		return shared.Field(shared.CodeInvalid, "work description length is invalid", "description", "must contain 3 to 1000 characters")
	}
	e.Minutes = minutes
	e.Description = description
	e.ReturnReason = ""
	e.Version++
	e.UpdatedAt = now.UTC()
	return nil
}
