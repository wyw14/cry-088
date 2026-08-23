package organization

import (
	"strings"
	"time"

	"github.com/wyw14/cry-088/internal/domain/shared"
)

type Organization struct {
	ID                  string
	Name                string
	Timezone            string
	MaxDailyMinutes     int
	OvertimeAfterMinute int
	AllowOverlap        bool
	Version             int64
}

func NewOrganization(id, name, timezone string) (*Organization, error) {
	id = strings.TrimSpace(id)
	name = strings.TrimSpace(name)
	if id == "" || name == "" {
		return nil, shared.New(shared.CodeInvalid, "organization id and name are required")
	}
	if _, err := time.LoadLocation(timezone); err != nil {
		return nil, shared.Field(shared.CodeInvalid, "invalid organization timezone", "timezone", timezone)
	}
	return &Organization{
		ID: id, Name: name, Timezone: timezone,
		MaxDailyMinutes: 720, OvertimeAfterMinute: 480, Version: 1,
	}, nil
}

func (o Organization) Location() (*time.Location, error) {
	loc, err := time.LoadLocation(o.Timezone)
	if err != nil {
		return nil, shared.Wrap(shared.CodeInvalid, "invalid configured timezone", err)
	}
	return loc, nil
}

func (o *Organization) ChangeTimeRules(maxDaily, overtimeAfter int, allowOverlap bool, expectedVersion int64) error {
	if expectedVersion != o.Version {
		return shared.New(shared.CodeConflict, "organization rules were changed")
	}
	if maxDaily < 60 || maxDaily > 1440 {
		return shared.Field(shared.CodeInvalid, "daily limit must be between 60 and 1440 minutes", "max_daily_minutes", "out of range")
	}
	if overtimeAfter < 0 || overtimeAfter > maxDaily {
		return shared.Field(shared.CodeInvalid, "overtime threshold exceeds daily limit", "overtime_after_minutes", "out of range")
	}
	o.MaxDailyMinutes = maxDaily
	o.OvertimeAfterMinute = overtimeAfter
	o.AllowOverlap = allowOverlap
	o.Version++
	return nil
}
