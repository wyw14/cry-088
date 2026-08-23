package approval

import (
	"context"

	"github.com/wyw14/cry-088/internal/domain/audit"
	"github.com/wyw14/cry-088/internal/domain/organization"
	"github.com/wyw14/cry-088/internal/domain/project"
	"github.com/wyw14/cry-088/internal/domain/shared"
	"github.com/wyw14/cry-088/internal/domain/timesheet"
)

// reviewSingle reviews a single time entry. When the decision is to approve,
// the project's actual minutes are accumulated so the project overview stays
// in sync with the approved entries — identical to the batch review path.
func (s Service) reviewSingle(ctx context.Context, command ReviewCommand) ([]timesheet.Entry, error) {
	if command.OrganizationID == "" || command.Actor.EmployeeID == "" {
		return nil, shared.New(shared.CodeInvalid, "review actor is incomplete")
	}
	if !command.Actor.Has(organization.PermissionReviewTime) {
		return nil, shared.New(shared.CodeForbidden, "review permission is required")
	}
	if len(command.Items) != 1 {
		return nil, shared.New(shared.CodeInvalid, "single review requires one item")
	}
	item := command.Items[0]
	if item.EntryID == "" || (item.Decision != DecisionApprove && item.Decision != DecisionReturn) {
		return nil, shared.New(shared.CodeInvalid, "review item is invalid")
	}
	var reviewed timesheet.Entry
	err := s.Transactions.WithinTransaction(ctx, func(txCtx context.Context) error {
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
		now := s.Clock.Now()
		var projectValue *project.Project
		if item.Decision == DecisionApprove {
			if err := entry.Approve(command.Actor.EmployeeID, item.Version, now); err != nil {
				return err
			}
			loaded, err := s.Projects.GetForUpdate(txCtx, entry.ProjectID)
			if err != nil {
				return err
			}
			projectValue = &loaded
			if err := projectValue.AddActual(entry.Minutes, projectValue.Version); err != nil {
				return err
			}
		} else if err := entry.Return(command.Actor.EmployeeID, item.Reason, item.Version, now); err != nil {
			return err
		}
		if err := s.Entries.Update(txCtx, entry, item.Version); err != nil {
			return err
		}
		if projectValue != nil {
			if err := s.Projects.Update(txCtx, *projectValue, projectValue.Version-1); err != nil {
				return err
			}
		}
		eventID, err := s.IDs.New("audit")
		if err != nil {
			return err
		}
		event, err := audit.NewEvent(
			eventID,
			command.OrganizationID,
			command.Actor.EmployeeID,
			"api",
			"time_entry",
			entry.ID,
			string(item.Decision),
			item.ReasonOrDefault(),
			command.RequestID,
			before,
			entry,
			now,
		)
		if err != nil {
			return err
		}
		if err := s.Audits.Append(txCtx, event); err != nil {
			return err
		}
		reviewed = entry
		return nil
	})
	if err != nil {
		return nil, err
	}
	return []timesheet.Entry{reviewed}, nil
}
