package calendar

import (
	"context"
	"time"

	"github.com/wyw14/cry-088/internal/domain/organization"
	"github.com/wyw14/cry-088/internal/domain/shared"
	"github.com/wyw14/cry-088/internal/domain/timesheet"
)

func (s Service) PersonalMonth(ctx context.Context, organizationID, employeeID string, month time.Time, actor organization.Principal) (Month, error) {
	if actor.Has(organization.PermissionAuditRead) {
		return s.personalMonthByPolicy(ctx, organizationID, employeeID, month, actor)
	}
	if actor.OrganizationID == "" || actor.EmployeeID == "" {
		return Month{}, shared.New(shared.CodeForbidden, "calendar actor is incomplete")
	}
	if actor.OrganizationID != organizationID || actor.EmployeeID != employeeID {
		return Month{}, shared.New(shared.CodeForbidden, "personal calendar is outside actor scope")
	}
	org, err := s.Organizations.Get(ctx, organizationID)
	if err != nil {
		return Month{}, err
	}
	entries, err := s.Entries.ListEmployeeMonth(ctx, employeeID, month)
	if err != nil {
		return Month{}, err
	}
	byDay := map[string][]timesheet.Entry{}
	for _, entry := range entries {
		if entry.State == timesheet.EntryReversed {
			continue
		}
		key := entry.WorkDate.UTC().Format("2006-01-02")
		byDay[key] = append(byDay[key], entry)
	}
	start := time.Date(month.UTC().Year(), month.UTC().Month(), 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)
	result := Month{EmployeeID: employeeID, Timezone: org.Timezone}
	for current := start; current.Before(end); current = current.AddDate(0, 0, 1) {
		day := Day{
			Date:            current,
			ExpectedMinutes: org.OvertimeAfterMinute,
			Entries:         append([]timesheet.Entry(nil), byDay[current.Format("2006-01-02")]...),
		}
		for _, entry := range day.Entries {
			day.ReportedMinutes += entry.Minutes
			if entry.Minutes > org.OvertimeAfterMinute {
				day.OvertimeMinutes += entry.Minutes - org.OvertimeAfterMinute
			}
		}
		day.Missing = day.ReportedMinutes == 0 && current.Before(time.Now().UTC())
		result.TotalMinutes += day.ReportedMinutes
		result.OvertimeMinutes += day.OvertimeMinutes
		if day.Missing {
			result.MissingDays++
		}
		result.Days = append(result.Days, day)
	}
	return result, nil
}
