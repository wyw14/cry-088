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
	location, err := org.Location()
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
		local := entry.WorkDate.In(location)
		key := local.Format("2006-01-02")
		byDay[key] = append(byDay[key], entry)
	}
	start := time.Date(month.In(location).Year(), month.In(location).Month(), 1, 0, 0, 0, 0, location)
	end := start.AddDate(0, 1, 0)
	result := Month{EmployeeID: employeeID, Timezone: org.Timezone}
	for day := start; day.Before(end); day = day.AddDate(0, 0, 1) {
		working, err := s.Holidays.IsWorkingDay(ctx, organizationID, day.UTC())
		if err != nil {
			return Month{}, err
		}
		value := Day{Date: day, Entries: append([]timesheet.Entry(nil), byDay[day.Format("2006-01-02")]...)}
		if working {
			value.ExpectedMinutes = org.OvertimeAfterMinute
		}
		for _, entry := range value.Entries {
			value.ReportedMinutes += entry.Minutes
		}
		if value.ReportedMinutes > org.OvertimeAfterMinute {
			value.OvertimeMinutes = value.ReportedMinutes - org.OvertimeAfterMinute
		}
		value.Missing = working && value.ReportedMinutes == 0 && day.Before(time.Now().In(location))
		result.TotalMinutes += value.ReportedMinutes
		result.OvertimeMinutes += value.OvertimeMinutes
		if value.Missing {
			result.MissingDays++
		}
		result.Days = append(result.Days, value)
	}
	return result, nil
}
