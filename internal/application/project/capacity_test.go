package project

import (
	"context"
	"testing"
	"time"

	"github.com/wyw14/cry-088/internal/domain/audit"
	"github.com/wyw14/cry-088/internal/domain/organization"
	domain "github.com/wyw14/cry-088/internal/domain/project"
	"github.com/wyw14/cry-088/internal/domain/shared"
	"github.com/wyw14/cry-088/internal/platform/clock"
	platformID "github.com/wyw14/cry-088/internal/platform/id"
)

type stubAssignments struct {
	existing   []domain.Assignment
	inserted   []domain.Assignment
	listCalled int
}

func (s *stubAssignments) ListEmployeeMonth(_ context.Context, _ string, _ time.Time) ([]domain.Assignment, error) {
	s.listCalled++
	return s.existing, nil
}

func (s *stubAssignments) Insert(_ context.Context, a domain.Assignment) error {
	s.inserted = append(s.inserted, a)
	return nil
}

type noAudits struct{}

func (noAudits) Append(context.Context, audit.Event) error { return nil }

type noTx struct{}

func (noTx) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

func newService(existing []domain.Assignment) (CapacityService, *stubAssignments) {
	store := &stubAssignments{existing: existing}
	return CapacityService{
		Assignments:  store,
		Employees:    nil,
		Audits:       noAudits{},
		Transactions: noTx{},
		Clock:        clock.Fixed{Time: time.Date(2026, 8, 23, 0, 0, 0, 0, time.UTC)},
		IDs:          &platformID.Sequence{},
	}, store
}

func TestAssignRejectsCrossProjectMonthlyOvercommit(t *testing.T) {
	// Member's month is already filled to capacity by another project.
	jan := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	janEnd := time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)
	const available = 9600
	filled := domain.Assignment{
		ID: "a-filled", ProjectID: "project-a", EmployeeID: "emp-1",
		Role: domain.ProjectRoleMember, ValidFrom: jan, ValidUntil: janEnd,
		TaskIDs: map[string]bool{}, CapacityMinutes: available, Version: 1,
	}
	service, store := newService([]domain.Assignment{filled})

	// A lead assigns the same member to a new project in the same month.
	_, err := service.Assign(context.Background(), AssignCommand{
		ProjectID: "project-b", EmployeeID: "emp-1",
		Role: domain.ProjectRoleMember, ValidFrom: jan, ValidUntil: janEnd,
		CapacityMinutes: 4800, AvailableMinutes: available,
		Actor: organization.Principal{EmployeeID: "lead-1"},
	})
	if err == nil {
		t.Fatal("expected capacity error when month is already filled by other projects, got nil")
	}
	if !shared.IsCode(err, shared.CodeCapacity) {
		t.Fatalf("expected CAPACITY_EXCEEDED error, got %v", err)
	}
	if len(store.inserted) != 0 {
		t.Fatalf("expected no assignment to be inserted on overcommit, got %d", len(store.inserted))
	}
}

func TestAssignAllowsCapacityWithinRemainingHeadroom(t *testing.T) {
	jan := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	janEnd := time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)
	const available = 9600
	existing := domain.Assignment{
		ID: "a-existing", ProjectID: "project-a", EmployeeID: "emp-1",
		Role: domain.ProjectRoleMember, ValidFrom: jan, ValidUntil: janEnd,
		TaskIDs: map[string]bool{}, CapacityMinutes: 4800, Version: 1,
	}
	service, store := newService([]domain.Assignment{existing})

	assignment, err := service.Assign(context.Background(), AssignCommand{
		ProjectID: "project-b", EmployeeID: "emp-1",
		Role: domain.ProjectRoleMember, ValidFrom: jan, ValidUntil: janEnd,
		CapacityMinutes: 4800, AvailableMinutes: available,
		Actor: organization.Principal{EmployeeID: "lead-1"},
	})
	if err != nil {
		t.Fatalf("expected success within headroom, got %v", err)
	}
	if len(store.inserted) != 1 || store.inserted[0].ID != assignment.ID {
		t.Fatalf("expected assignment to be inserted once, got %v", store.inserted)
	}
}

func TestAssignChecksEveryMonthSpannedByLongAssignment(t *testing.T) {
	// Existing assignment consumes the full capacity of the LAST month that the
	// new (multi-month) assignment spans. Earlier months must pass before the
	// loop reaches the overcommitted final month, proving each month is checked.
	jan := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	marEnd := time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC)
	const available = 9600
	mar := domain.Assignment{
		ID: "a-mar", ProjectID: "project-a", EmployeeID: "emp-1",
		Role: domain.ProjectRoleMember,
		ValidFrom: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
		ValidUntil: time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC),
		TaskIDs: map[string]bool{}, CapacityMinutes: available, Version: 1,
	}
	service, store := newService([]domain.Assignment{mar})

	_, err := service.Assign(context.Background(), AssignCommand{
		ProjectID: "project-b", EmployeeID: "emp-1",
		Role: domain.ProjectRoleMember, ValidFrom: jan, ValidUntil: marEnd,
		CapacityMinutes: 4800, AvailableMinutes: available,
		Actor: organization.Principal{EmployeeID: "lead-1"},
	})
	if err == nil {
		t.Fatal("expected capacity error for the overcommitted March month, got nil")
	}
	if !shared.IsCode(err, shared.CodeCapacity) {
		t.Fatalf("expected CAPACITY_EXCEEDED error, got %v", err)
	}
	if len(store.inserted) != 0 {
		t.Fatalf("expected no assignment to be inserted on overcommit, got %d", len(store.inserted))
	}
	// The validator walks every month of the span: Jan and Feb pass, March
	// fails, so all three months were checked before rejecting.
	if store.listCalled != 3 {
		t.Fatalf("expected capacity checked for each of the 3 spanned months, got %d calls", store.listCalled)
	}
}
