package project

import (
	"context"
	"testing"
	"time"

	"github.com/wyw14/cry-088/internal/domain/audit"
	"github.com/wyw14/cry-088/internal/domain/organization"
	domain "github.com/wyw14/cry-088/internal/domain/project"
	"github.com/wyw14/cry-088/internal/domain/shared"
)

type capacityAssignments struct {
	existing []domain.Assignment
	inserted []domain.Assignment
}

func (f *capacityAssignments) ListEmployeeMonth(context.Context, string, time.Time) ([]domain.Assignment, error) {
	return append([]domain.Assignment(nil), f.existing...), nil
}
func (f *capacityAssignments) Insert(_ context.Context, value domain.Assignment) error {
	f.inserted = append(f.inserted, value)
	return nil
}

type capacityEmployees struct{ employee organization.Employee }

func (f capacityEmployees) Get(context.Context, string) (organization.Employee, error) {
	return f.employee, nil
}

type capacityAudits struct{}

func (capacityAudits) Append(context.Context, audit.Event) error { return nil }

type capacityTx struct{}

func (capacityTx) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type capacityClock struct{ now time.Time }

func (f capacityClock) Now() time.Time { return f.now }

type capacityIDs struct{}

func (capacityIDs) New(prefix string) (string, error) { return prefix + "_new", nil }

func TestAssignRejectsAggregateCapacityOverflow(t *testing.T) {
	now := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	assignments := &capacityAssignments{existing: []domain.Assignment{{
		ID: "existing", ProjectID: "p-old", EmployeeID: "emp", Role: domain.ProjectRoleMember,
		ValidFrom: now, ValidUntil: now.AddDate(0, 1, 0), CapacityMinutes: 420, Version: 1,
	}}}
	service := CapacityService{
		Assignments: assignments,
		Employees:   capacityEmployees{organization.Employee{ID: "emp", OrganizationID: "org", Status: organization.EmployeeActive, HiredAt: now.AddDate(-1, 0, 0)}},
		Audits:      capacityAudits{}, Transactions: capacityTx{}, Clock: capacityClock{now}, IDs: capacityIDs{},
	}
	actor := organization.Principal{EmployeeID: "lead", OrganizationID: "org", Roles: []organization.Role{organization.RoleProjectLead}, ProjectIDs: map[string]bool{"p-new": true}}
	_, err := service.Assign(context.Background(), AssignCommand{
		OrganizationID: "org", ProjectID: "p-new", EmployeeID: "emp", Role: domain.ProjectRoleMember,
		ValidFrom: now, ValidUntil: now.AddDate(0, 1, 0), CapacityMinutes: 120, AvailableMinutes: 480,
		Actor: actor, RequestID: "request-1",
	})
	if !shared.IsCode(err, shared.CodeCapacity) {
		t.Fatalf("expected capacity error, got %v", err)
	}
	if len(assignments.inserted) != 0 {
		t.Fatalf("overflow assignment was persisted: %d", len(assignments.inserted))
	}
}
