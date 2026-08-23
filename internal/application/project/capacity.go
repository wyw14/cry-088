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
	if !command.Actor.Has(organization.PermissionManageProject) || !command.Actor.CanAccessProject(command.OrganizationID, command.ProjectID) {
		return domain.Assignment{}, shared.New(shared.CodeForbidden, "project management permission is required")
	}
	employee, err := s.Employees.Get(ctx, command.EmployeeID)
	if err != nil {
		return domain.Assignment{}, err
	}
	if employee.OrganizationID != command.OrganizationID || !employee.ActiveOn(command.ValidFrom) {
		return domain.Assignment{}, shared.New(shared.CodeForbidden, "employee is outside organization or inactive")
	}
	assignmentID, err := s.IDs.New("assignment")
	if err != nil {
		return domain.Assignment{}, err
	}
	tasks := make(map[string]bool, len(command.TaskIDs))
	for _, taskID := range command.TaskIDs {
		if taskID == "" || tasks[taskID] {
			return domain.Assignment{}, shared.New(shared.CodeInvalid, "task scope contains an empty or duplicate id")
		}
		tasks[taskID] = true
	}
	assignment := domain.Assignment{ID: assignmentID, ProjectID: command.ProjectID, EmployeeID: command.EmployeeID, Role: command.Role, ValidFrom: command.ValidFrom.UTC(), ValidUntil: command.ValidUntil.UTC(), TaskIDs: tasks, CapacityMinutes: command.CapacityMinutes, Version: 1}
	if err := assignment.Validate(); err != nil {
		return domain.Assignment{}, err
	}
	existing, err := s.Assignments.ListEmployeeMonth(ctx, command.EmployeeID, command.ValidFrom)
	if err != nil {
		return domain.Assignment{}, err
	}
	all := append(append([]domain.Assignment(nil), existing...), assignment)
	if _, err := domain.BuildCapacityPlan(command.EmployeeID, command.ValidFrom, command.AvailableMinutes, all); err != nil {
		return domain.Assignment{}, err
	}
	err = s.Transactions.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := s.Assignments.Insert(txCtx, assignment); err != nil {
			return err
		}
		eventID, err := s.IDs.New("audit")
		if err != nil {
			return err
		}
		event, err := audit.NewEvent(eventID, command.OrganizationID, command.Actor.EmployeeID, "api", "assignment", assignment.ID, "created", "project capacity allocation", command.RequestID, nil, assignment, s.Clock.Now())
		if err != nil {
			return err
		}
		return s.Audits.Append(txCtx, event)
	})
	if err != nil {
		return domain.Assignment{}, err
	}
	return assignment, nil
}
