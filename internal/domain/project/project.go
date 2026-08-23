package project

import (
	"strings"
	"time"

	"github.com/wyw14/cry-088/internal/domain/shared"
)

type Status string

const (
	StatusPlanning  Status = "planning"
	StatusActive    Status = "active"
	StatusPaused    Status = "paused"
	StatusCompleted Status = "completed"
	StatusArchived  Status = "archived"
)

type Project struct {
	ID               string
	OrganizationID   string
	Code             string
	Name             string
	OwnerID          string
	StartDate        time.Time
	EndDate          time.Time
	BudgetMinutes    int
	DefaultCostCents int64
	ActualMinutes    int
	Status           Status
	Version          int64
}

func New(id, organizationID, code, name, ownerID string, start, end time.Time, budgetMinutes int, costCents int64) (*Project, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(organizationID) == "" || strings.TrimSpace(ownerID) == "" {
		return nil, shared.New(shared.CodeInvalid, "project identity is incomplete")
	}
	if !end.After(start) {
		return nil, shared.Field(shared.CodeInvalid, "project end must be after start", "end_date", "invalid range")
	}
	if budgetMinutes <= 0 || costCents < 0 {
		return nil, shared.New(shared.CodeInvalid, "project budget and cost rate are invalid")
	}
	return &Project{ID: id, OrganizationID: organizationID, Code: strings.ToUpper(code), Name: strings.TrimSpace(name), OwnerID: ownerID, StartDate: start.UTC(), EndDate: end.UTC(), BudgetMinutes: budgetMinutes, DefaultCostCents: costCents, Status: StatusPlanning, Version: 1}, nil
}

func (p *Project) Activate(expectedVersion int64) error {
	if p.Version != expectedVersion {
		return shared.New(shared.CodeConflict, "project was changed")
	}
	if p.Status != StatusPlanning && p.Status != StatusPaused {
		return shared.New(shared.CodeIllegalState, "project cannot be activated from current state")
	}
	p.Status = StatusActive
	p.Version++
	return nil
}

func (p *Project) AddActual(minutes int, expectedVersion int64) error {
	if p.Version != expectedVersion {
		return shared.New(shared.CodeConflict, "project actuals were changed")
	}
	if minutes <= 0 || p.ActualMinutes+minutes < 0 {
		return shared.New(shared.CodeInvalid, "actual minutes delta is invalid")
	}
	p.ActualMinutes += minutes
	p.Version++
	return nil
}

func (p Project) Includes(day time.Time) bool {
	d := day.UTC()
	return !d.Before(p.StartDate) && !d.After(p.EndDate)
}

func (p Project) BurnRatio() float64 {
	if p.BudgetMinutes == 0 {
		return 0
	}
	return float64(p.ActualMinutes) / float64(p.BudgetMinutes)
}
