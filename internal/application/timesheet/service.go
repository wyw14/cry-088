package timesheet

import (
	"context"
	"fmt"
	"time"

	"github.com/wyw14/cry-088/internal/domain/audit"
	"github.com/wyw14/cry-088/internal/domain/organization"
	"github.com/wyw14/cry-088/internal/domain/project"
	"github.com/wyw14/cry-088/internal/domain/settlement"
	"github.com/wyw14/cry-088/internal/domain/shared"
	domain "github.com/wyw14/cry-088/internal/domain/timesheet"
)

type EntryRepository interface {
	Get(ctx context.Context, id string) (domain.Entry, error)
	ListEmployeeDay(ctx context.Context, employeeID string, day time.Time) ([]domain.Entry, error)
	ListEmployeeMonth(ctx context.Context, employeeID string, month time.Time) ([]domain.Entry, error)
	Insert(ctx context.Context, entry domain.Entry) error
	InsertMany(ctx context.Context, entries []domain.Entry) error
	Update(ctx context.Context, entry domain.Entry, expectedVersion int64) error
}

type AssignmentRepository interface {
	ActiveFor(ctx context.Context, employeeID, projectID, taskID string, day time.Time) (project.Assignment, error)
}

type OrganizationRepository interface {
	Get(ctx context.Context, id string) (organization.Organization, error)
}

type ProjectRepository interface {
	Get(ctx context.Context, id string) (project.Project, error)
}

type PeriodRepository interface {
	ForMonth(ctx context.Context, organizationID string, month time.Time) (settlement.Period, error)
}

type AuditRepository interface {
	Append(ctx context.Context, event audit.Event) error
}

type TransactionManager interface {
	WithinTransaction(ctx context.Context, fn func(context.Context) error) error
}

type Clock interface{ Now() time.Time }
type IDGenerator interface {
	New(prefix string) (string, error)
}

type Service struct {
	Entries       EntryRepository
	Assignments   AssignmentRepository
	Organizations OrganizationRepository
	Projects      ProjectRepository
	Periods       PeriodRepository
	Audits        AuditRepository
	Transactions  TransactionManager
	Clock         Clock
	IDs           IDGenerator
}

type CreateDraftCommand struct {
	OrganizationID string
	EmployeeID     string
	ProjectID      string
	TaskID         string
	WorkDate       time.Time
	StartMinute    int
	Minutes        int
	Description    string
	Actor          organization.Principal
	RequestID      string
}

func (s Service) CreateDraft(ctx context.Context, command CreateDraftCommand) (domain.Entry, error) {
	if command.Actor.EmployeeID != command.EmployeeID || !command.Actor.Has(organization.PermissionWriteOwnTime) {
		return domain.Entry{}, shared.New(shared.CodeForbidden, "employee can only create own time entries")
	}
	org, err := s.Organizations.Get(ctx, command.OrganizationID)
	if err != nil {
		return domain.Entry{}, err
	}
	projectValue, err := s.Projects.Get(ctx, command.ProjectID)
	if err != nil {
		return domain.Entry{}, err
	}
	if projectValue.OrganizationID != command.OrganizationID || !command.Actor.CanAccessProject(command.OrganizationID, command.ProjectID) {
		return domain.Entry{}, shared.New(shared.CodeForbidden, "project is outside employee scope")
	}
	if projectValue.Status != project.StatusActive || !projectValue.Includes(command.WorkDate) {
		return domain.Entry{}, shared.New(shared.CodeIllegalState, "project is not open for this work date")
	}
	period, err := s.Periods.ForMonth(ctx, command.OrganizationID, command.WorkDate)
	if err != nil {
		return domain.Entry{}, err
	}
	if !period.Mutable(false) {
		return domain.Entry{}, shared.New(shared.CodePeriodClosed, "settlement period is closed")
	}
	assignment, err := s.Assignments.ActiveFor(ctx, command.EmployeeID, command.ProjectID, command.TaskID, command.WorkDate)
	if err != nil {
		return domain.Entry{}, err
	}
	entryID, err := s.IDs.New("time")
	if err != nil {
		return domain.Entry{}, err
	}
	entry, err := domain.NewEntry(entryID, command.OrganizationID, command.EmployeeID, command.ProjectID, command.TaskID, command.WorkDate, command.StartMinute, command.Minutes, command.Description, s.Clock.Now())
	if err != nil {
		return domain.Entry{}, err
	}
	existing, err := s.Entries.ListEmployeeMonth(ctx, command.EmployeeID, command.WorkDate)
	if err != nil {
		return domain.Entry{}, err
	}
	if _, err := domain.ValidateDraft(org, assignment, *entry, existing); err != nil {
		return domain.Entry{}, err
	}
	if err := s.Transactions.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := s.Entries.Insert(txCtx, *entry); err != nil {
			return err
		}
		eventID, idErr := s.IDs.New("audit")
		if idErr != nil {
			return idErr
		}
		event, eventErr := audit.NewEvent(eventID, command.OrganizationID, command.Actor.EmployeeID, "api", "time_entry", entry.ID, "draft_created", "employee calendar entry", command.RequestID, nil, entry, s.Clock.Now())
		if eventErr != nil {
			return eventErr
		}
		return s.Audits.Append(txCtx, event)
	}); err != nil {
		return domain.Entry{}, err
	}
	return *entry, nil
}

type BatchSubmitCommand struct {
	OrganizationID string
	EmployeeID     string
	EntryIDs       []string
	Actor          organization.Principal
	RequestID      string
}

func (s Service) BatchSubmit(ctx context.Context, command BatchSubmitCommand) ([]domain.Entry, error) {
	if command.Actor.EmployeeID != command.EmployeeID || !command.Actor.Has(organization.PermissionWriteOwnTime) {
		return nil, shared.New(shared.CodeForbidden, "employee can only submit own entries")
	}
	if len(command.EntryIDs) == 0 || len(command.EntryIDs) > 100 {
		return nil, shared.Field(shared.CodeInvalid, "batch size must be between 1 and 100", "entry_ids", "invalid size")
	}
	seen := map[string]bool{}
	entries := make([]domain.Entry, 0, len(command.EntryIDs))
	for _, entryID := range command.EntryIDs {
		if entryID == "" || seen[entryID] {
			return nil, shared.New(shared.CodeInvalid, "batch contains an empty or duplicate entry id")
		}
		seen[entryID] = true
		entry, err := s.Entries.Get(ctx, entryID)
		if err != nil {
			return nil, err
		}
		if entry.OrganizationID != command.OrganizationID || entry.EmployeeID != command.EmployeeID {
			return nil, shared.New(shared.CodeForbidden, "batch contains an entry outside employee scope")
		}
		entries = append(entries, entry)
	}
	org, err := s.Organizations.Get(ctx, command.OrganizationID)
	if err != nil {
		return nil, err
	}
	if err := domain.ValidateBatchNoInternalOverlap(org, entries); err != nil {
		return nil, err
	}
	now := s.Clock.Now()
	if err := s.Transactions.WithinTransaction(ctx, func(txCtx context.Context) error {
		for index := range entries {
			entry := &entries[index]
			period, periodErr := s.Periods.ForMonth(txCtx, command.OrganizationID, entry.WorkDate)
			if periodErr != nil {
				return periodErr
			}
			if !period.Mutable(false) {
				return shared.New(shared.CodePeriodClosed, "batch contains an entry in a closed period")
			}
			projectValue, projectErr := s.Projects.Get(txCtx, entry.ProjectID)
			if projectErr != nil {
				return projectErr
			}
			if projectValue.Status != project.StatusActive || !projectValue.Includes(entry.WorkDate) {
				return shared.New(shared.CodeIllegalState, "batch contains an entry outside an active project range")
			}
			assignment, assignmentErr := s.Assignments.ActiveFor(txCtx, entry.EmployeeID, entry.ProjectID, entry.TaskID, entry.WorkDate)
			if assignmentErr != nil {
				return assignmentErr
			}
			existing, listErr := s.Entries.ListEmployeeMonth(txCtx, entry.EmployeeID, entry.WorkDate)
			if listErr != nil {
				return listErr
			}
			if _, validationErr := domain.ValidateDraft(org, assignment, *entry, existing); validationErr != nil {
				return validationErr
			}
			before := *entry
			expectedVersion := entry.Version
			if submitErr := entry.Submit(expectedVersion, now); submitErr != nil {
				return submitErr
			}
			if updateErr := s.Entries.Update(txCtx, *entry, expectedVersion); updateErr != nil {
				return updateErr
			}
			eventID, idErr := s.IDs.New("audit")
			if idErr != nil {
				return idErr
			}
			event, eventErr := audit.NewEvent(eventID, command.OrganizationID, command.Actor.EmployeeID, "api", "time_entry", entry.ID, "submitted", "employee batch submission", command.RequestID, before, entry, now)
			if eventErr != nil {
				return eventErr
			}
			if appendErr := s.Audits.Append(txCtx, event); appendErr != nil {
				return appendErr
			}
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("submit time entry batch: %w", err)
	}
	return entries, nil
}
