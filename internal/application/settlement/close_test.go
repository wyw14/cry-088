package settlement

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/wyw14/cry-088/internal/domain/audit"
	"github.com/wyw14/cry-088/internal/domain/organization"
	domain "github.com/wyw14/cry-088/internal/domain/settlement"
	"github.com/wyw14/cry-088/internal/domain/shared"
	platformClock "github.com/wyw14/cry-088/internal/platform/clock"
	"github.com/wyw14/cry-088/internal/platform/id"
)

type fakePeriodRepo struct {
	mu     sync.Mutex
	period domain.Period
}

func (r *fakePeriodRepo) GetForUpdate(_ context.Context, id string) (domain.Period, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.period.ID != id {
		return domain.Period{}, shared.New(shared.CodeNotFound, "settlement period not found")
	}
	return r.period, nil
}

func (r *fakePeriodRepo) Update(_ context.Context, period domain.Period, expectedVersion int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.period.Version != expectedVersion {
		return shared.New(shared.CodeConflict, "settlement period version mismatch")
	}
	r.period = period
	return nil
}

type fakeCostSource struct {
	lines []domain.CostLine
}

func (f fakeCostSource) ApprovedCostLines(_ context.Context, _ string, _ time.Time) ([]domain.CostLine, error) {
	return append([]domain.CostLine(nil), f.lines...), nil
}

type fakeSnapshotRepo struct {
	mu       sync.Mutex
	snapshot domain.CostSnapshot
	byPeriod map[string]domain.CostSnapshot
	inserts  int
}

func newFakeSnapshotRepo() *fakeSnapshotRepo {
	return &fakeSnapshotRepo{byPeriod: map[string]domain.CostSnapshot{}}
}

func (r *fakeSnapshotRepo) FindByPeriod(_ context.Context, periodID string) (domain.CostSnapshot, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	snapshot, ok := r.byPeriod[periodID]
	return snapshot, ok, nil
}

func (r *fakeSnapshotRepo) Insert(_ context.Context, snapshot domain.CostSnapshot) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.inserts++
	if _, exists := r.byPeriod[snapshot.PeriodID]; exists {
		return shared.New(shared.CodeConflict, "cost snapshot already exists for period")
	}
	r.byPeriod[snapshot.PeriodID] = snapshot
	return nil
}

type fakeAuditRepo struct {
	mu     sync.Mutex
	events []audit.Event
}

func (r *fakeAuditRepo) Append(_ context.Context, event audit.Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
	return nil
}

type passthroughTx struct{}

func (passthroughTx) WithinTransaction(_ context.Context, fn func(context.Context) error) error {
	return fn(context.Background())
}

// serializedTx mirrors a real PostgreSQL transaction: the body runs while a
// lock is held (the row lock acquired by FOR UPDATE), so concurrent closes
// of the same period execute one at a time. This is the guarantee the
// production TransactionManager provides and what the closeConsistent path
// relies on for idempotency under contention.
type serializedTx struct{ mu sync.Mutex }

func (t *serializedTx) WithinTransaction(_ context.Context, fn func(context.Context) error) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return fn(context.Background())
}

func newService(t *testing.T, period domain.Period, lines []domain.CostLine) (Service, *fakePeriodRepo, *fakeSnapshotRepo, *fakeAuditRepo) {
	t.Helper()
	periods := &fakePeriodRepo{period: period}
	snapshots := newFakeSnapshotRepo()
	audits := &fakeAuditRepo{}
	svc := Service{
		Periods:      periods,
		Costs:        fakeCostSource{lines: lines},
		Snapshots:    snapshots,
		Audits:       audits,
		Transactions: passthroughTx{},
		Clock:        platformClock.Fixed{Time: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)},
		IDs:          &id.Sequence{},
	}
	return svc, periods, snapshots, audits
}

func financeActor() organization.Principal {
	return organization.Principal{EmployeeID: "emp-finance", OrganizationID: "org-1", Roles: []organization.Role{organization.RoleFinance}}
}

func TestCloseIdempotentOnRepeatedCalls(t *testing.T) {
	period, err := domain.NewPeriod("period-1", "org-1", time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("new period: %v", err)
	}
	lines := []domain.CostLine{
		{EmployeeID: "emp-a", ProjectID: "proj-1", Minutes: 60, RateCents: 100, AmountCents: 100},
	}
	svc, periods, snapshots, audits := newService(t, *period, lines)
	cmd := CloseCommand{OrganizationID: "org-1", PeriodID: "period-1", Actor: financeActor(), RequestID: "req-1"}

	first, err := svc.Close(context.Background(), cmd)
	if err != nil {
		t.Fatalf("first close: %v", err)
	}
	if snapshots.inserts != 1 {
		t.Fatalf("expected 1 snapshot after first close, got %d", snapshots.inserts)
	}
	if len(audits.events) != 1 {
		t.Fatalf("expected 1 audit event after first close, got %d", len(audits.events))
	}
	if got := periods.period.State; got != domain.PeriodClosed {
		t.Fatalf("expected period closed after first close, got %q", got)
	}

	// Second close: a double-click / retry. Must be idempotent.
	second, err := svc.Close(context.Background(), cmd)
	if err != nil {
		t.Fatalf("second close: %v", err)
	}
	if snapshots.inserts != 1 {
		t.Fatalf("expected still 1 snapshot after second close, got %d", snapshots.inserts)
	}
	if len(audits.events) != 1 {
		t.Fatalf("expected still 1 audit event after second close, got %d", len(audits.events))
	}
	if second.ID != first.ID || second.TotalCents != first.TotalCents || second.Checksum != first.Checksum {
		t.Fatalf("second close returned a different snapshot: first=%+v second=%+v", first, second)
	}
	if got := periods.period.State; got != domain.PeriodClosed {
		t.Fatalf("expected period still closed after second close, got %q", got)
	}
}

func TestCloseConsistentAcrossConcurrentRequests(t *testing.T) {
	period, err := domain.NewPeriod("period-2", "org-1", time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("new period: %v", err)
	}
	lines := []domain.CostLine{
		{EmployeeID: "emp-a", ProjectID: "proj-1", Minutes: 120, RateCents: 200, AmountCents: 400},
	}
	svc, periods, snapshots, audits := newService(t, *period, lines)
	svc.Transactions = &serializedTx{}
	cmd := CloseCommand{OrganizationID: "org-1", PeriodID: "period-2", Actor: financeActor(), RequestID: "req-2"}

	var wg sync.WaitGroup
	const n = 8
	results := make([]domain.CostSnapshot, n)
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = svc.Close(context.Background(), cmd)
		}(i)
	}
	wg.Wait()

	success := 0
	var reference domain.CostSnapshot
	for i := 0; i < n; i++ {
		if errs[i] == nil {
			success++
			if success == 1 {
				reference = results[i]
			} else if results[i].ID != reference.ID {
				t.Fatalf("concurrent close returned different snapshot ids: %s vs %s", reference.ID, results[i].ID)
			}
		}
	}
	if success == 0 {
		t.Fatalf("all concurrent closes failed: %v", errs[0])
	}
	if snapshots.inserts != 1 {
		t.Fatalf("expected exactly 1 snapshot after concurrent closes, got %d", snapshots.inserts)
	}
	if len(audits.events) != 1 {
		t.Fatalf("expected exactly 1 audit event after concurrent closes, got %d", len(audits.events))
	}
	if got := periods.period.State; got != domain.PeriodClosed {
		t.Fatalf("expected period closed after concurrent closes, got %q", got)
	}
}

func TestCloseRespectsOptimisticVersion(t *testing.T) {
	period, err := domain.NewPeriod("period-3", "org-1", time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("new period: %v", err)
	}
	svc, _, snapshots, _ := newService(t, *period, nil)
	cmd := CloseCommand{OrganizationID: "org-1", PeriodID: "period-3", ExpectedVersion: 999, Actor: financeActor(), RequestID: "req-3"}

	if _, err := svc.Close(context.Background(), cmd); err == nil {
		t.Fatalf("expected conflict for stale version, got nil")
	} else if !shared.IsCode(err, shared.CodeConflict) {
		t.Fatalf("expected CONFLICT for stale version, got %v", err)
	}
	if snapshots.inserts != 0 {
		t.Fatalf("expected no snapshot on stale version, got %d", snapshots.inserts)
	}
}

func TestCloseForbiddenWithoutPermission(t *testing.T) {
	period, err := domain.NewPeriod("period-4", "org-1", time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("new period: %v", err)
	}
	svc, _, snapshots, _ := newService(t, *period, nil)
	cmd := CloseCommand{OrganizationID: "org-1", PeriodID: "period-4", Actor: organization.Principal{EmployeeID: "emp-x", OrganizationID: "org-1", Roles: []organization.Role{organization.RoleEmployee}}, RequestID: "req-4"}

	if _, err := svc.Close(context.Background(), cmd); err == nil {
		t.Fatalf("expected forbidden without close permission, got nil")
	} else if !shared.IsCode(err, shared.CodeForbidden) {
		t.Fatalf("expected FORBIDDEN without close permission, got %v", err)
	}
	if snapshots.inserts != 0 {
		t.Fatalf("expected no snapshot when forbidden, got %d", snapshots.inserts)
	}
}
