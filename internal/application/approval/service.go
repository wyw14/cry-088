package approval

import (
	"context"
	"time"

	"github.com/wyw14/cry-088/internal/domain/audit"
	"github.com/wyw14/cry-088/internal/domain/organization"
	"github.com/wyw14/cry-088/internal/domain/project"
	"github.com/wyw14/cry-088/internal/domain/shared"
	"github.com/wyw14/cry-088/internal/domain/timesheet"
)

type EntryRepository interface {
	Get(ctx context.Context, id string) (timesheet.Entry, error)
	Update(ctx context.Context, entry timesheet.Entry, expectedVersion int64) error
}

type ProjectRepository interface {
	GetForUpdate(ctx context.Context, id string) (project.Project, error)
	Update(ctx context.Context, project project.Project, expectedVersion int64) error
}

type AssignmentRepository interface {
	ReviewerCanLead(ctx context.Context, reviewerID, projectID string, day time.Time) (bool, error)
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
	Projects     ProjectRepository
	Assignments  AssignmentRepository
	Audits       AuditRepository
	Transactions TransactionManager
	Clock        Clock
	IDs          IDGenerator
}

type Decision string

const (
	DecisionApprove Decision = "approve"
	DecisionReturn  Decision = "return"
)

type ReviewItem struct {
	EntryID  string
	Version  int64
	Decision Decision
	Reason   string
}

type ReviewCommand struct {
	OrganizationID string
	Actor          organization.Principal
	Items          []ReviewItem
	RequestID      string
}

func (s Service) Review(ctx context.Context, command ReviewCommand) ([]timesheet.Entry, error) {
	if len(command.Items) == 1 {
		return s.reviewSingle(ctx, command)
	}
	if !command.Actor.Has(organization.PermissionReviewTime) {
		return nil, shared.New(shared.CodeForbidden, "review permission is required")
	}
	if len(command.Items) == 0 || len(command.Items) > 100 {
		return nil, shared.New(shared.CodeInvalid, "review batch size is invalid")
	}
	seen := map[string]bool{}
	for _, item := range command.Items {
		if item.EntryID == "" || seen[item.EntryID] {
			return nil, shared.New(shared.CodeInvalid, "review batch contains duplicate entries")
		}
		if item.Decision != DecisionApprove && item.Decision != DecisionReturn {
			return nil, shared.New(shared.CodeInvalid, "review decision is invalid")
		}
		seen[item.EntryID] = true
	}
	updated := make([]timesheet.Entry, 0, len(command.Items))
	now := s.Clock.Now()
	err := s.Transactions.WithinTransaction(ctx, func(txCtx context.Context) error {
		projectCache := map[string]*project.Project{}
		projectVersions := map[string]int64{}
		for _, item := range command.Items {
			entry, err := s.Entries.Get(txCtx, item.EntryID)
			if err != nil {
				return err
			}
			if entry.OrganizationID != command.OrganizationID || !command.Actor.CanAccessProject(command.OrganizationID, entry.ProjectID) {
				return shared.New(shared.CodeForbidden, "review entry is outside actor scope")
			}
			allowed, err := s.Assignments.ReviewerCanLead(txCtx, command.Actor.EmployeeID, entry.ProjectID, entry.WorkDate)
			if err != nil {
				return err
			}
			if !allowed {
				return shared.New(shared.CodeForbidden, "actor was not project lead on the work date")
			}
			before := entry
			if item.Decision == DecisionApprove {
				if err := entry.Approve(command.Actor.EmployeeID, item.Version, now); err != nil {
					return err
				}
				projectValue := projectCache[entry.ProjectID]
				if projectValue == nil {
					loaded, err := s.Projects.GetForUpdate(txCtx, entry.ProjectID)
					if err != nil {
						return err
					}
					projectVersions[entry.ProjectID] = loaded.Version
					projectValue = &loaded
					projectCache[entry.ProjectID] = projectValue
				}
				if err := projectValue.AddActual(entry.Minutes, projectValue.Version); err != nil {
					return err
				}
			} else {
				if err := entry.Return(command.Actor.EmployeeID, item.Reason, item.Version, now); err != nil {
					return err
				}
			}
			if err := s.Entries.Update(txCtx, entry, item.Version); err != nil {
				return err
			}
			eventID, err := s.IDs.New("audit")
			if err != nil {
				return err
			}
			event, err := audit.NewEvent(eventID, command.OrganizationID, command.Actor.EmployeeID, "api", "time_entry", entry.ID, string(item.Decision), item.ReasonOrDefault(), command.RequestID, before, entry, now)
			if err != nil {
				return err
			}
			if err := s.Audits.Append(txCtx, event); err != nil {
				return err
			}
			updated = append(updated, entry)
		}
		for projectID, projectValue := range projectCache {
			if err := s.Projects.Update(txCtx, *projectValue, projectVersions[projectID]); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func (i ReviewItem) ReasonOrDefault() string {
	if i.Reason != "" {
		return i.Reason
	}
	return "project lead approval"
}
