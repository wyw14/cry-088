package calendar

import (
	"context"
	"testing"
	"time"

	"github.com/wyw14/cry-088/internal/domain/organization"
	"github.com/wyw14/cry-088/internal/domain/timesheet"
)

type calendarEntries struct{ values []timesheet.Entry }

func (f calendarEntries) ListEmployeeMonth(context.Context, string, time.Time) ([]timesheet.Entry, error) {
	return append([]timesheet.Entry(nil), f.values...), nil
}

type calendarOrganizations struct{ value organization.Organization }

func (f calendarOrganizations) Get(context.Context, string) (organization.Organization, error) {
	return f.value, nil
}

type calendarHolidays struct{ location *time.Location }

func (f calendarHolidays) IsWorkingDay(_ context.Context, _ string, day time.Time) (bool, error) {
	local := day.In(f.location)
	if local.Year() == 2026 && local.Month() == time.August && local.Day() == 3 {
		return false, nil
	}
	weekday := local.Weekday()
	return weekday != time.Saturday && weekday != time.Sunday, nil
}

func TestPersonalMonthUsesTimezoneHolidayAndDailyOvertime(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	stored := timesheet.Entry{ID: "entry", OrganizationID: "org", EmployeeID: "emp", ProjectID: "project", TaskID: "task", WorkDate: time.Date(2026, 7, 31, 16, 0, 0, 0, time.UTC), StartMinute: 480, Minutes: 600, Description: "release support", State: timesheet.EntryApproved, Version: 1}
	service := Service{Entries: calendarEntries{[]timesheet.Entry{stored}}, Organizations: calendarOrganizations{organization.Organization{ID: "org", Timezone: "Asia/Shanghai", MaxDailyMinutes: 720, OvertimeAfterMinute: 480}}, Holidays: calendarHolidays{location}}
	actor := organization.Principal{EmployeeID: "emp", OrganizationID: "org", Roles: []organization.Role{organization.RoleEmployee}}
	result, err := service.PersonalMonth(context.Background(), "org", "emp", time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), actor)
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalMinutes != 600 || result.OvertimeMinutes != 120 {
		t.Fatalf("totals=%d overtime=%d", result.TotalMinutes, result.OvertimeMinutes)
	}
	var first, holiday Day
	for _, day := range result.Days {
		if day.Date.Day() == 1 {
			first = day
		}
		if day.Date.Day() == 3 {
			holiday = day
		}
	}
	if first.ReportedMinutes != 600 {
		t.Fatalf("timezone entry landed on wrong day: %+v", first)
	}
	if holiday.ExpectedMinutes != 0 || holiday.Missing {
		t.Fatalf("calendar override was ignored: %+v", holiday)
	}
}
