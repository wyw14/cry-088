package project

import (
	"time"

	"github.com/wyw14/cry-088/internal/domain/shared"
)

type ProjectRole string

const (
	ProjectRoleMember ProjectRole = "member"
	ProjectRoleLead   ProjectRole = "lead"
	ProjectRoleViewer ProjectRole = "viewer"
)

type Assignment struct {
	ID              string
	ProjectID       string
	EmployeeID      string
	Role            ProjectRole
	ValidFrom       time.Time
	ValidUntil      time.Time
	TaskIDs         map[string]bool
	CapacityMinutes int
	Version         int64
}

func (a Assignment) Validate() error {
	if a.ID == "" || a.ProjectID == "" || a.EmployeeID == "" {
		return shared.New(shared.CodeInvalid, "assignment identity is incomplete")
	}
	if a.ValidUntil.Before(a.ValidFrom) {
		return shared.Field(shared.CodeInvalid, "assignment date range is invalid", "valid_until", "before valid_from")
	}
	if a.CapacityMinutes <= 0 {
		return shared.Field(shared.CodeInvalid, "assignment capacity must be positive", "capacity_minutes", "must be positive")
	}
	return nil
}

func (a Assignment) Allows(day time.Time, taskID string) bool {
	day = day.UTC()
	if day.Before(a.ValidFrom.UTC()) || day.After(a.ValidUntil.UTC()) {
		return false
	}
	if len(a.TaskIDs) == 0 {
		return true
	}
	return a.TaskIDs[taskID]
}

type CapacityPlan struct {
	EmployeeID       string
	Month            time.Time
	AvailableMinutes int
	AllocatedMinutes int
}

func BuildCapacityPlan(employeeID string, month time.Time, available int, assignments []Assignment) (CapacityPlan, error) {
	if employeeID == "" || available < 0 {
		return CapacityPlan{}, shared.New(shared.CodeInvalid, "capacity plan input is invalid")
	}
	start := time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0).Add(-time.Nanosecond)
	plan := CapacityPlan{EmployeeID: employeeID, Month: start, AvailableMinutes: available}
	seen := map[string]bool{}
	for _, assignment := range assignments {
		if assignment.EmployeeID != employeeID || assignment.ValidUntil.Before(start) || assignment.ValidFrom.After(end) {
			continue
		}
		if seen[assignment.ID] {
			continue
		}
		seen[assignment.ID] = true
		plan.AllocatedMinutes += assignment.CapacityMinutes
	}
	if plan.AllocatedMinutes > plan.AvailableMinutes {
		return plan, shared.New(shared.CodeCapacity, "employee allocation exceeds monthly capacity")
	}
	return plan, nil
}
