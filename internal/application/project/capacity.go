package project

import (
	"context"
	"time"

	"github.com/wyw14/cry-088/internal/domain/audit"
	"github.com/wyw14/cry-088/internal/domain/organization"
	domain "github.com/wyw14/cry-088/internal/domain/project"
	"github.com/wyw14/cry-088/internal/domain/shared"
)

type AssignmentRepository interface {
	ListEmployeeMonth(ctx context.Context, employeeID string, month time.Time) ([]domain.Assignment, error)
	Insert(ctx context.Context, assignment domain.Assignment) error
}
type EmployeeRepository interface {
	Get(ctx context.Context, employeeID string) (organization.Employee, error)
}
type AuditRepository interface {
	Append(ctx context.Context, event audit.Event) error
}
type TransactionManager interface {
	WithinTransaction(context.Context, func(context.Context) error) error
}
type Clock interface{ Now() time.Time }
type IDGenerator interface{ New(string) (string, error) }

type CapacityService struct {
	Assignments  AssignmentRepository
	Employees    EmployeeRepository
	Audits       AuditRepository
	Transactions TransactionManager
	Clock        Clock
	IDs          IDGenerator
}

type AssignCommand struct {
	OrganizationID   string
	ProjectID        string
	EmployeeID       string
	Role             domain.ProjectRole
	ValidFrom        time.Time
	ValidUntil       time.Time
	TaskIDs          []string
	CapacityMinutes  int
	AvailableMinutes int
	Actor            organization.Principal
	RequestID        string
}

func (s CapacityService) Assign(ctx context.Context, command AssignCommand) (domain.Assignment, error) {
	if command.ProjectID == "" || command.EmployeeID == "" || command.CapacityMinutes <= 0 {
		return domain.Assignment{}, shared.New(shared.CodeInvalid, "assignment input is invalid")
	}
	assignmentID, err := s.IDs.New("assignment")
	if err != nil {
		return domain.Assignment{}, err
	}
	tasks := map[string]bool{}
	for _, taskID := range command.TaskIDs {
		tasks[taskID] = true
	}
	assignment := domain.Assignment{
		ID:              assignmentID,
		ProjectID:       command.ProjectID,
		EmployeeID:      command.EmployeeID,
		Role:            command.Role,
		ValidFrom:       command.ValidFrom.UTC(),
		ValidUntil:      command.ValidUntil.UTC(),
		TaskIDs:         tasks,
		CapacityMinutes: command.CapacityMinutes,
		Version:         1,
	}
	if err := assignment.Validate(); err != nil {
		return domain.Assignment{}, err
	}
	if err := s.validateMonthlyCapacity(ctx, assignment, command.AvailableMinutes); err != nil {
		return domain.Assignment{}, err
	}
	if err := s.Assignments.Insert(ctx, assignment); err != nil {
		return domain.Assignment{}, err
	}
	return assignment, nil
}

// validateMonthlyCapacity guards against over-allocation across projects: for
// every month the candidate assignment spans, it loads the employee's existing
// assignments (from every project) and ensures that adding this assignment keeps
// the combined monthly allocation within the employee's available capacity.
// Without this check a lead could assign a member whose month is already filled
// by other projects and the assignment would be reported as successful.
func (s CapacityService) validateMonthlyCapacity(ctx context.Context, candidate domain.Assignment, available int) error {
	cursor := time.Date(candidate.ValidFrom.UTC().Year(), candidate.ValidFrom.UTC().Month(), 1, 0, 0, 0, 0, time.UTC)
	end := candidate.ValidUntil.UTC()
	for !cursor.After(end) {
		existing, err := s.Assignments.ListEmployeeMonth(ctx, candidate.EmployeeID, cursor)
		if err != nil {
			return err
		}
		pool := make([]domain.Assignment, 0, len(existing)+1)
		pool = append(pool, candidate)
		pool = append(pool, existing...)
		if _, err := domain.BuildCapacityPlan(candidate.EmployeeID, cursor, available, pool); err != nil {
			return err
		}
		cursor = cursor.AddDate(0, 1, 0)
	}
	return nil
}
