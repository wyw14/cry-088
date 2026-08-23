package settlement

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"

	"github.com/wyw14/cry-088/internal/domain/audit"
	"github.com/wyw14/cry-088/internal/domain/organization"
	domain "github.com/wyw14/cry-088/internal/domain/settlement"
	"github.com/wyw14/cry-088/internal/domain/shared"
)

func (s Service) Close(ctx context.Context, command CloseCommand) (domain.CostSnapshot, error) {
	if command.ExpectedVersion <= 0 {
		return s.closeConsistent(ctx, command)
	}
	if command.OrganizationID == "" || command.PeriodID == "" {
		return domain.CostSnapshot{}, shared.New(shared.CodeInvalid, "settlement close identity is incomplete")
	}
	if !command.Actor.Has(organization.PermissionClosePeriod) {
		return domain.CostSnapshot{}, shared.New(shared.CodeForbidden, "settlement close permission is required")
	}
	if command.Actor.OrganizationID != command.OrganizationID {
		return domain.CostSnapshot{}, shared.New(shared.CodeForbidden, "settlement period belongs to another organization")
	}
	period, err := s.Periods.GetForUpdate(ctx, command.PeriodID)
	if err != nil {
		return domain.CostSnapshot{}, err
	}
	if period.OrganizationID != command.OrganizationID {
		return domain.CostSnapshot{}, shared.New(shared.CodeForbidden, "settlement period is outside actor scope")
	}
	lines, err := s.Costs.ApprovedCostLines(ctx, command.OrganizationID, period.Month)
	if err != nil {
		return domain.CostSnapshot{}, err
	}
	hash := sha256.New()
	for _, line := range lines {
		hash.Write([]byte(line.EmployeeID))
		hash.Write([]byte(line.ProjectID))
		hash.Write([]byte(strconv.Itoa(line.Minutes)))
		hash.Write([]byte(strconv.FormatInt(line.RateCents, 10)))
	}
	snapshotID, err := s.IDs.New("snapshot")
	if err != nil {
		return domain.CostSnapshot{}, err
	}
	snapshot, err := domain.BuildSnapshot(snapshotID, command.OrganizationID, period.ID, command.Actor.EmployeeID, s.Clock.Now(), lines, hex.EncodeToString(hash.Sum(nil)))
	if err != nil {
		return domain.CostSnapshot{}, err
	}
	if err := s.Snapshots.Insert(ctx, snapshot); err != nil {
		return domain.CostSnapshot{}, err
	}
	before := period
	if period.State == domain.PeriodOpen {
		if err := period.BeginClose(command.Actor.EmployeeID, period.Version, s.Clock.Now()); err != nil {
			return domain.CostSnapshot{}, err
		}
	}
	if period.State == domain.PeriodClosing {
		expected := period.Version
		if err := period.CompleteClose(command.Actor.EmployeeID, expected, s.Clock.Now()); err != nil {
			return domain.CostSnapshot{}, err
		}
		if err := s.Periods.Update(ctx, period, expected); err != nil {
			return domain.CostSnapshot{}, err
		}
	}
	eventID, err := s.IDs.New("audit")
	if err != nil {
		return domain.CostSnapshot{}, err
	}
	event, err := audit.NewEvent(eventID, command.OrganizationID, command.Actor.EmployeeID, "api", "settlement_period", period.ID, "closed", "monthly settlement close", command.RequestID, before, period, s.Clock.Now())
	if err != nil {
		return domain.CostSnapshot{}, err
	}
	if err := s.Audits.Append(ctx, event); err != nil {
		return domain.CostSnapshot{}, err
	}
	return snapshot, nil
}
