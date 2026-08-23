package settlement

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"time"

	"github.com/wyw14/cry-088/internal/domain/audit"
	"github.com/wyw14/cry-088/internal/domain/organization"
	domain "github.com/wyw14/cry-088/internal/domain/settlement"
	"github.com/wyw14/cry-088/internal/domain/shared"
)

type PeriodRepository interface {
	GetForUpdate(ctx context.Context, id string) (domain.Period, error)
	Update(ctx context.Context, period domain.Period, expectedVersion int64) error
}

type CostSource interface {
	ApprovedCostLines(ctx context.Context, organizationID string, month time.Time) ([]domain.CostLine, error)
}

type SnapshotRepository interface {
	FindByPeriod(ctx context.Context, periodID string) (domain.CostSnapshot, bool, error)
	Insert(ctx context.Context, snapshot domain.CostSnapshot) error
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
	Periods      PeriodRepository
	Costs        CostSource
	Snapshots    SnapshotRepository
	Audits       AuditRepository
	Transactions TransactionManager
	Clock        Clock
	IDs          IDGenerator
}

type CloseCommand struct {
	OrganizationID  string
	PeriodID        string
	ExpectedVersion int64
	Actor           organization.Principal
	RequestID       string
}

func (s Service) closeConsistent(ctx context.Context, command CloseCommand) (domain.CostSnapshot, error) {
	if !command.Actor.Has(organization.PermissionClosePeriod) || command.Actor.OrganizationID != command.OrganizationID {
		return domain.CostSnapshot{}, shared.New(shared.CodeForbidden, "settlement close permission is required")
	}
	var result domain.CostSnapshot
	err := s.Transactions.WithinTransaction(ctx, func(txCtx context.Context) error {
		period, err := s.Periods.GetForUpdate(txCtx, command.PeriodID)
		if err != nil {
			return err
		}
		if period.OrganizationID != command.OrganizationID {
			return shared.New(shared.CodeForbidden, "settlement period is outside organization")
		}
		// Re-check the snapshot under the period row lock so that a concurrent
		// close returns the existing snapshot instead of producing a duplicate.
		if existing, found, err := s.Snapshots.FindByPeriod(txCtx, period.ID); err != nil {
			return err
		} else if found {
			result = existing
			return nil
		}
		if period.State == domain.PeriodClosed {
			return shared.New(shared.CodeConflict, "closed period has no immutable cost snapshot")
		}
		if command.ExpectedVersion > 0 && period.Version != command.ExpectedVersion {
			return shared.New(shared.CodeConflict, "settlement period was changed")
		}
		// heldVersion is the version currently held by the database row; the
		// optimistic-concurrency predicate for Update must match it, not the
		// in-memory value that BeginClose/CompleteClose mutate.
		heldVersion := period.Version
		before := period
		if period.State == domain.PeriodOpen {
			if err := period.BeginClose(command.Actor.EmployeeID, heldVersion, s.Clock.Now()); err != nil {
				return err
			}
		}
		lines, err := s.Costs.ApprovedCostLines(txCtx, command.OrganizationID, period.Month)
		if err != nil {
			return err
		}
		sort.Slice(lines, func(i, j int) bool {
			if lines[i].ProjectID == lines[j].ProjectID {
				return lines[i].EmployeeID < lines[j].EmployeeID
			}
			return lines[i].ProjectID < lines[j].ProjectID
		})
		hash := sha256.New()
		for _, line := range lines {
			hash.Write([]byte(line.ProjectID))
			hash.Write([]byte{0})
			hash.Write([]byte(line.EmployeeID))
			hash.Write([]byte{0})
			hash.Write([]byte(strconv.Itoa(line.Minutes)))
			hash.Write([]byte{0})
			hash.Write([]byte(strconv.FormatInt(line.RateCents, 10)))
		}
		snapshotID, err := s.IDs.New("snapshot")
		if err != nil {
			return err
		}
		snapshot, err := domain.BuildSnapshot(snapshotID, command.OrganizationID, period.ID, command.Actor.EmployeeID, s.Clock.Now(), lines, hex.EncodeToString(hash.Sum(nil)))
		if err != nil {
			return err
		}
		if err := s.Snapshots.Insert(txCtx, snapshot); err != nil {
			return err
		}
		// CompleteClose validates against the current in-memory version
		// (already incremented by BeginClose when transitioning from open).
		if err := period.CompleteClose(command.Actor.EmployeeID, period.Version, s.Clock.Now()); err != nil {
			return err
		}
		if err := s.Periods.Update(txCtx, period, heldVersion); err != nil {
			return err
		}
		eventID, err := s.IDs.New("audit")
		if err != nil {
			return err
		}
		event, err := audit.NewEvent(eventID, command.OrganizationID, command.Actor.EmployeeID, "api", "settlement_period", period.ID, "closed", "monthly settlement close", command.RequestID, before, period, s.Clock.Now())
		if err != nil {
			return err
		}
		if err := s.Audits.Append(txCtx, event); err != nil {
			return err
		}
		result = snapshot
		return nil
	})
	if err != nil {
		return domain.CostSnapshot{}, err
	}
	return result, nil
}
