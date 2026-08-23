package correction

import (
	"context"
	"time"

	"github.com/wyw14/cry-088/internal/domain/audit"
	"github.com/wyw14/cry-088/internal/domain/organization"
	"github.com/wyw14/cry-088/internal/domain/project"
	"github.com/wyw14/cry-088/internal/domain/settlement"
	"github.com/wyw14/cry-088/internal/domain/shared"
	"github.com/wyw14/cry-088/internal/domain/timesheet"
)

type EntryRepository interface {
	Get(ctx context.Context, id string) (timesheet.Entry, error)
	Insert(ctx context.Context, entry timesheet.Entry) error
	Update(ctx context.Context, entry timesheet.Entry, expectedVersion int64) error
}

type CorrectionRepository interface {
	Insert(ctx context.Context, correction timesheet.Correction) error
	Update(ctx context.Context, correction timesheet.Correction, expectedVersion int64) error
}

type PeriodRepository interface {
	ForMonth(ctx context.Context, organizationID string, month time.Time) (settlement.Period, error)
}

type AssignmentRepository interface {
	ActiveFor(ctx context.Context, employeeID, projectID, taskID string, day time.Time) (project.Assignment, error)
}

type AuditRepository interface {
	Append(ctx context.Context, event audit.Event) error
}
type TransactionManager interface {
	WithinTransaction(context.Context, func(context.Context) error) error
}
type Clock interface{ Now() time.Time }
type IDGenerator interface{ New(string) (string, error) }

type Service struct {
	Entries      EntryRepository
	Corrections  CorrectionRepository
	Periods      PeriodRepository
	Assignments  AssignmentRepository
	Audits       AuditRepository
	Transactions TransactionManager
	Clock        Clock
	IDs          IDGenerator
}

type RequestCommand struct {
	OrganizationID  string
	OriginalEntryID string
	TaskID          string
	StartMinute     int
	Minutes         int
	Description     string
	Reason          string
	Actor           organization.Principal
	RequestID       string
}

type RequestResult struct {
	Correction  timesheet.Correction
	Replacement timesheet.Entry
}

func (s Service) Request(ctx context.Context, command RequestCommand) (RequestResult, error) {
	original, err := s.Entries.Get(ctx, command.OriginalEntryID)
	if err != nil {
		return RequestResult{}, err
	}
	if original.OrganizationID != command.OrganizationID || original.EmployeeID != command.Actor.EmployeeID {
		return RequestResult{}, shared.New(shared.CodeForbidden, "employee can only correct own time entry")
	}
	if original.State != timesheet.EntryLocked {
		return RequestResult{}, shared.New(shared.CodeIllegalState, "only locked entries require correction")
	}
	period, err := s.Periods.ForMonth(ctx, command.OrganizationID, original.WorkDate)
	if err != nil {
		return RequestResult{}, err
	}
	if period.State == settlement.PeriodClosed && !command.Actor.Has(organization.PermissionClosePeriod) {
		return RequestResult{}, shared.New(shared.CodePeriodClosed, "closed periods can only be corrected by finance")
	}
	assignment, err := s.Assignments.ActiveFor(ctx, original.EmployeeID, original.ProjectID, command.TaskID, original.WorkDate)
	if err != nil {
		return RequestResult{}, err
	}
	if !assignment.Allows(original.WorkDate, command.TaskID) {
		return RequestResult{}, shared.New(shared.CodeForbidden, "replacement task is outside assignment scope")
	}
	replacementID, err := s.IDs.New("time")
	if err != nil {
		return RequestResult{}, err
	}
	correctionID, err := s.IDs.New("correction")
	if err != nil {
		return RequestResult{}, err
	}
	replacement, err := timesheet.NewEntry(replacementID, command.OrganizationID, original.EmployeeID, original.ProjectID, command.TaskID, original.WorkDate, command.StartMinute, command.Minutes, command.Description, s.Clock.Now())
	if err != nil {
		return RequestResult{}, err
	}
	replacement.CorrectionID = correctionID
	correction, err := timesheet.NewCorrection(correctionID, command.OrganizationID, original.ID, replacement.ID, command.Actor.EmployeeID, command.Reason, s.Clock.Now())
	if err != nil {
		return RequestResult{}, err
	}
	err = s.Transactions.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := s.Entries.Insert(txCtx, *replacement); err != nil {
			return err
		}
		if err := s.Corrections.Insert(txCtx, *correction); err != nil {
			return err
		}
		eventID, err := s.IDs.New("audit")
		if err != nil {
			return err
		}
		event, err := audit.NewEvent(eventID, command.OrganizationID, command.Actor.EmployeeID, "api", "correction", correction.ID, "requested", correction.Reason, command.RequestID, original, replacement, s.Clock.Now())
		if err != nil {
			return err
		}
		return s.Audits.Append(txCtx, event)
	})
	if err != nil {
		return RequestResult{}, err
	}
	return RequestResult{Correction: *correction, Replacement: *replacement}, nil
}

type ApproveCommand struct {
	OrganizationID string
	Correction     timesheet.Correction
	Actor          organization.Principal
	RequestID      string
}

func (s Service) Approve(ctx context.Context, command ApproveCommand) (RequestResult, error) {
	if !command.Actor.Has(organization.PermissionReviewTime) {
		return RequestResult{}, shared.New(shared.CodeForbidden, "correction review permission is required")
	}
	original, err := s.Entries.Get(ctx, command.Correction.OriginalEntryID)
	if err != nil {
		return RequestResult{}, err
	}
	replacement, err := s.Entries.Get(ctx, command.Correction.ReplacementEntryID)
	if err != nil {
		return RequestResult{}, err
	}
	if original.OrganizationID != command.OrganizationID || replacement.OrganizationID != command.OrganizationID || !command.Actor.CanAccessProject(command.OrganizationID, original.ProjectID) {
		return RequestResult{}, shared.New(shared.CodeForbidden, "correction is outside reviewer scope")
	}
	now := s.Clock.Now()
	correctionValue := command.Correction
	err = s.Transactions.WithinTransaction(ctx, func(txCtx context.Context) error {
		correctionVersion := correctionValue.Version
		if err := correctionValue.Approve(command.Actor.EmployeeID, correctionVersion, now); err != nil {
			return err
		}
		originalVersion := original.Version
		reversalID, err := s.IDs.New("reversal")
		if err != nil {
			return err
		}
		if err := original.Reverse(reversalID, correctionValue.ID, originalVersion, now); err != nil {
			return err
		}
		replacementVersion := replacement.Version
		if err := replacement.Submit(replacementVersion, now); err != nil {
			return err
		}
		if err := replacement.Approve(command.Actor.EmployeeID, replacement.Version, now); err != nil {
			return err
		}
		if err := replacement.Lock(replacement.Version, now); err != nil {
			return err
		}
		if err := s.Entries.Update(txCtx, original, originalVersion); err != nil {
			return err
		}
		if err := s.Entries.Update(txCtx, replacement, replacementVersion); err != nil {
			return err
		}
		if err := s.Corrections.Update(txCtx, correctionValue, correctionVersion); err != nil {
			return err
		}
		eventID, err := s.IDs.New("audit")
		if err != nil {
			return err
		}
		event, err := audit.NewEvent(eventID, command.OrganizationID, command.Actor.EmployeeID, "api", "correction", correctionValue.ID, "approved", correctionValue.Reason, command.RequestID, original.ID, replacement.ID, now)
		if err != nil {
			return err
		}
		return s.Audits.Append(txCtx, event)
	})
	if err != nil {
		return RequestResult{}, err
	}
	return RequestResult{Correction: correctionValue, Replacement: replacement}, nil
}
