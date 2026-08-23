package approval

import (
	"context"
	"time"

	"github.com/wyw14/cry-088/internal/domain/audit"
	"github.com/wyw14/cry-088/internal/domain/organization"
	"github.com/wyw14/cry-088/internal/domain/shared"
	"github.com/wyw14/cry-088/internal/domain/timesheet"
)

type LockRepository interface {
	ListApprovedForPeriod(ctx context.Context, organizationID string, month time.Time) ([]timesheet.Entry, error)
	Update(ctx context.Context, entry timesheet.Entry, expectedVersion int64) error
}

type LockService struct {
	Entries      LockRepository
	Audits       AuditRepository
	Transactions TransactionManager
	Clock        Clock
	IDs          IDGenerator
}

func (s LockService) LockApproved(ctx context.Context, organizationID string, month time.Time, actor organization.Principal, requestID string) ([]timesheet.Entry, error) {
	if !actor.Has(organization.PermissionClosePeriod) || actor.OrganizationID != organizationID {
		return nil, shared.New(shared.CodeForbidden, "period close permission is required")
	}
	entries, err := s.Entries.ListApprovedForPeriod(ctx, organizationID, month)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return []timesheet.Entry{}, nil
	}
	now := s.Clock.Now()
	err = s.Transactions.WithinTransaction(ctx, func(txCtx context.Context) error {
		for index := range entries {
			entry := &entries[index]
			before := *entry
			expectedVersion := entry.Version
			if err := entry.Lock(expectedVersion, now); err != nil {
				return err
			}
			if err := s.Entries.Update(txCtx, *entry, expectedVersion); err != nil {
				return err
			}
			eventID, err := s.IDs.New("audit")
			if err != nil {
				return err
			}
			event, err := audit.NewEvent(eventID, organizationID, actor.EmployeeID, "settlement", "time_entry", entry.ID, "locked", "settlement period lock", requestID, before, entry, now)
			if err != nil {
				return err
			}
			if err := s.Audits.Append(txCtx, event); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return entries, nil
}
