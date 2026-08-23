package timesheet

import (
	"sort"
	"time"

	"github.com/wyw14/cry-088/internal/domain/organization"
	"github.com/wyw14/cry-088/internal/domain/project"
	"github.com/wyw14/cry-088/internal/domain/shared"
)

type DraftInput struct {
	ProjectID   string
	TaskID      string
	WorkDate    time.Time
	StartMinute int
	Minutes     int
	Description string
}

type ValidationResult struct {
	DailyMinutes    int
	OvertimeMinutes int
	CapacityAfter   int
}

func ValidateDraft(org organization.Organization, assignment project.Assignment, candidate Entry, existing []Entry) (ValidationResult, error) {
	if !assignment.Allows(candidate.WorkDate, candidate.TaskID) {
		return ValidationResult{}, shared.New(shared.CodeForbidden, "employee is not assigned to this task on the work date")
	}
	if assignment.EmployeeID != candidate.EmployeeID || assignment.ProjectID != candidate.ProjectID {
		return ValidationResult{}, shared.New(shared.CodeForbidden, "assignment does not match time entry")
	}
	result := ValidationResult{CapacityAfter: candidate.Minutes}
	for _, entry := range existing {
		if entry.ID == candidate.ID || entry.State == EntryReversed {
			continue
		}
		if entry.EmployeeID == candidate.EmployeeID && entry.WorkDate.Equal(candidate.WorkDate) {
			result.DailyMinutes += entry.Minutes
			if !org.AllowOverlap && candidate.Overlaps(entry) {
				return ValidationResult{}, shared.New(shared.CodeConflict, "time entry overlaps another entry")
			}
		}
		if entry.EmployeeID == candidate.EmployeeID && entry.ProjectID == candidate.ProjectID && entry.WorkDate.Year() == candidate.WorkDate.Year() && entry.WorkDate.Month() == candidate.WorkDate.Month() {
			result.CapacityAfter += entry.Minutes
		}
	}
	result.DailyMinutes += candidate.Minutes
	if result.DailyMinutes > org.MaxDailyMinutes {
		return result, shared.New(shared.CodeCapacity, "daily time limit exceeded")
	}
	if result.CapacityAfter > assignment.CapacityMinutes {
		return result, shared.New(shared.CodeCapacity, "assignment monthly capacity exceeded")
	}
	if result.DailyMinutes > org.OvertimeAfterMinute {
		result.OvertimeMinutes = result.DailyMinutes - org.OvertimeAfterMinute
	}
	return result, nil
}

func ValidateBatchNoInternalOverlap(org organization.Organization, entries []Entry) error {
	grouped := map[string][]Entry{}
	for _, entry := range entries {
		key := entry.EmployeeID + ":" + entry.WorkDate.Format("2006-01-02")
		grouped[key] = append(grouped[key], entry)
	}
	for _, daily := range grouped {
		sort.Slice(daily, func(i, j int) bool { return daily[i].StartMinute < daily[j].StartMinute })
		total := 0
		for index, entry := range daily {
			total += entry.Minutes
			if !org.AllowOverlap && index > 0 && daily[index-1].EndMinute() > entry.StartMinute {
				return shared.New(shared.CodeConflict, "batch contains overlapping time entries")
			}
		}
		if total > org.MaxDailyMinutes {
			return shared.New(shared.CodeCapacity, "batch exceeds daily time limit")
		}
	}
	return nil
}
