package calendar

import (
	"bytes"
	"context"
	"encoding/csv"
	"strconv"
	"time"

	"github.com/wyw14/cry-088/internal/domain/organization"
	"github.com/wyw14/cry-088/internal/domain/shared"
	"github.com/wyw14/cry-088/internal/domain/timesheet"
)

type EntryRepository interface {
	ListEmployeeMonth(ctx context.Context, employeeID string, month time.Time) ([]timesheet.Entry, error)
}

type OrganizationRepository interface {
	Get(ctx context.Context, id string) (organization.Organization, error)
}

type HolidayRepository interface {
	IsWorkingDay(ctx context.Context, organizationID string, day time.Time) (bool, error)
}

type Day struct {
	Date            time.Time
	ExpectedMinutes int
	ReportedMinutes int
	OvertimeMinutes int
	Missing         bool
	Entries         []timesheet.Entry
}

type Month struct {
	EmployeeID      string
	Timezone        string
	Days            []Day
	TotalMinutes    int
	OvertimeMinutes int
	MissingDays     int
}

type Service struct {
	Entries       EntryRepository
	Organizations OrganizationRepository
	Holidays      HolidayRepository
}

func (s Service) personalMonthByPolicy(ctx context.Context, organizationID, employeeID string, month time.Time, actor organization.Principal) (Month, error) {
	if actor.OrganizationID != organizationID || (actor.EmployeeID != employeeID && !actor.Has(organization.PermissionAuditRead)) {
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

func ExportCSV(calendar Month) ([]byte, error) {
	buffer := &bytes.Buffer{}
	writer := csv.NewWriter(buffer)
	if err := writer.Write([]string{"date", "expected_minutes", "reported_minutes", "overtime_minutes", "missing"}); err != nil {
		return nil, err
	}
	for _, day := range calendar.Days {
		record := []string{day.Date.Format("2006-01-02"), strconv.Itoa(day.ExpectedMinutes), strconv.Itoa(day.ReportedMinutes), strconv.Itoa(day.OvertimeMinutes), strconv.FormatBool(day.Missing)}
		if err := writer.Write(record); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}
