package settlement

import (
	"time"

	"github.com/wyw14/cry-088/internal/domain/shared"
)

type PeriodState string

const (
	PeriodOpen    PeriodState = "open"
	PeriodClosing PeriodState = "closing"
	PeriodClosed  PeriodState = "closed"
)

type Period struct {
	ID             string
	OrganizationID string
	Month          time.Time
	State          PeriodState
	ClosingBy      string
	ClosedBy       string
	ClosingAt      *time.Time
	ClosedAt       *time.Time
	Version        int64
}

func NewPeriod(id, organizationID string, month time.Time) (*Period, error) {
	if id == "" || organizationID == "" {
		return nil, shared.New(shared.CodeInvalid, "settlement period identity is incomplete")
	}
	month = time.Date(month.UTC().Year(), month.UTC().Month(), 1, 0, 0, 0, 0, time.UTC)
	return &Period{ID: id, OrganizationID: organizationID, Month: month, State: PeriodOpen, Version: 1}, nil
}

func (p Period) Contains(day time.Time) bool {
	return p.Month.Year() == day.UTC().Year() && p.Month.Month() == day.UTC().Month()
}

func (p Period) Mutable(privileged bool) bool {
	return p.State == PeriodOpen || (privileged && p.State == PeriodClosing)
}

func (p *Period) BeginClose(actorID string, expectedVersion int64, now time.Time) error {
	if p.Version != expectedVersion {
		return shared.New(shared.CodeConflict, "settlement period was changed")
	}
	if p.State != PeriodOpen || actorID == "" {
		return shared.New(shared.CodeIllegalState, "settlement period cannot begin closing")
	}
	closing := now.UTC()
	p.State = PeriodClosing
	p.ClosingBy = actorID
	p.ClosingAt = &closing
	p.Version++
	return nil
}

func (p *Period) CompleteClose(actorID string, expectedVersion int64, now time.Time) error {
	if p.Version != expectedVersion {
		return shared.New(shared.CodeConflict, "settlement period was changed")
	}
	if p.State != PeriodClosing || actorID == "" {
		return shared.New(shared.CodeIllegalState, "settlement period is not closing")
	}
	closed := now.UTC()
	p.State = PeriodClosed
	p.ClosedBy = actorID
	p.ClosedAt = &closed
	p.Version++
	return nil
}
