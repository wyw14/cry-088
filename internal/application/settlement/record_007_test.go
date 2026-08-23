package settlement

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/wyw14/cry-088/internal/domain/audit"
	"github.com/wyw14/cry-088/internal/domain/organization"
	domain "github.com/wyw14/cry-088/internal/domain/settlement"
)

type closePeriods struct {
	value   domain.Period
	updates int
}

func (f *closePeriods) GetForUpdate(context.Context, string) (domain.Period, error) {
	return f.value, nil
}
func (f *closePeriods) Update(_ context.Context, value domain.Period, _ int64) error {
	f.value = value
	f.updates++
	return nil
}

type closeCosts struct{ calls int }

func (f *closeCosts) ApprovedCostLines(context.Context, string, time.Time) ([]domain.CostLine, error) {
	f.calls++
	minutes := 60
	if f.calls > 1 {
		minutes = 180
	}
	line, _ := domain.NewCostLine("emp", "project", minutes, 6000)
	return []domain.CostLine{line}, nil
}

type closeSnapshots struct{ values []domain.CostSnapshot }

func (f *closeSnapshots) FindByPeriod(_ context.Context, periodID string) (domain.CostSnapshot, bool, error) {
	for _, value := range f.values {
		if value.PeriodID == periodID {
			return value, true, nil
		}
	}
	return domain.CostSnapshot{}, false, nil
}
func (f *closeSnapshots) Insert(_ context.Context, value domain.CostSnapshot) error {
	f.values = append(f.values, value)
	return nil
}

type closeAudits struct{ count int }

func (f *closeAudits) Append(context.Context, audit.Event) error {
	f.count++
	return nil
}

type closeTx struct{}

func (closeTx) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type closeClock struct{ now time.Time }

func (f closeClock) Now() time.Time { return f.now }

type closeIDs struct{ next int }

func (f *closeIDs) New(prefix string) (string, error) {
	f.next++
	return fmt.Sprintf("%s_%d", prefix, f.next), nil
}

func TestClosePeriodReplayReturnsSingleImmutableSnapshot(t *testing.T) {
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	period, _ := domain.NewPeriod("period", "org", now)
	periods := &closePeriods{value: *period}
	costs := &closeCosts{}
	snapshots := &closeSnapshots{}
	audits := &closeAudits{}
	service := Service{Periods: periods, Costs: costs, Snapshots: snapshots, Audits: audits, Transactions: closeTx{}, Clock: closeClock{now}, IDs: &closeIDs{}}
	actor := organization.Principal{EmployeeID: "finance", OrganizationID: "org", Roles: []organization.Role{organization.RoleFinance}}
	command := CloseCommand{OrganizationID: "org", PeriodID: "period", ExpectedVersion: 1, Actor: actor, RequestID: "request"}
	first, err := service.Close(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Close(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || first.Checksum != second.Checksum || first.TotalCents != second.TotalCents {
		t.Errorf("replay changed immutable snapshot: first=%+v second=%+v", first, second)
	}
	if len(snapshots.values) != 1 {
		t.Errorf("snapshot inserts=%d want 1", len(snapshots.values))
	}
	if costs.calls != 1 {
		t.Errorf("cost source calls=%d want 1", costs.calls)
	}
	if periods.value.State != domain.PeriodClosed || periods.updates != 1 {
		t.Errorf("period state=%s updates=%d", periods.value.State, periods.updates)
	}
	if audits.count != 1 {
		t.Errorf("close audit events=%d want 1", audits.count)
	}
}
