package approval

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/wyw14/cry-088/internal/domain/audit"
	"github.com/wyw14/cry-088/internal/domain/organization"
	"github.com/wyw14/cry-088/internal/domain/project"
	"github.com/wyw14/cry-088/internal/domain/timesheet"
	"github.com/wyw14/cry-088/internal/platform/clock"
	"github.com/wyw14/cry-088/internal/platform/id"
)

// fakeTx runs the callback immediately, matching the demo transaction manager.
type fakeTx struct{}

func (fakeTx) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type entryStore struct {
	mu      sync.Mutex
	entries map[string]timesheet.Entry
}

func newEntryStore(entries ...timesheet.Entry) *entryStore {
	store := &entryStore{entries: map[string]timesheet.Entry{}}
	for _, e := range entries {
		store.entries[e.ID] = e
	}
	return store
}

func (s *entryStore) Get(_ context.Context, id string) (timesheet.Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[id]
	if !ok {
		return e, errors.New("entry not found")
	}
	return e, nil
}

func (s *entryStore) Update(_ context.Context, e timesheet.Entry, expected int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.entries[e.ID]
	if !ok || current.Version != expected {
		return errors.New("entry version mismatch")
	}
	s.entries[e.ID] = e
	return nil
}

type projectStore struct {
	mu       sync.Mutex
	projects map[string]project.Project
}

func newProjectStore(values ...project.Project) *projectStore {
	store := &projectStore{projects: map[string]project.Project{}}
	for _, p := range values {
		store.projects[p.ID] = p
	}
	return store
}

func (s *projectStore) GetForUpdate(_ context.Context, id string) (project.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.projects[id]
	if !ok {
		return p, errors.New("project not found")
	}
	return p, nil
}

func (s *projectStore) Update(_ context.Context, p project.Project, expected int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.projects[p.ID]
	if !ok || current.Version != expected {
		return errors.New("project version mismatch")
	}
	s.projects[p.ID] = p
	return nil
}

type assignmentStore struct{ allowed bool }

func (a assignmentStore) ReviewerCanLead(context.Context, string, string, time.Time) (bool, error) {
	return a.allowed, nil
}

type auditStore struct {
	events []audit.Event
}

func (a *auditStore) Append(_ context.Context, e audit.Event) error {
	a.events = append(a.events, e)
	return nil
}

func newSubmittedEntry(id string, minutes int, now time.Time) timesheet.Entry {
	entry, err := timesheet.NewEntry(id, "org", "emp", "proj", "task", now, 0, minutes, "design review work", now)
	if err != nil {
		panic(err)
	}
	if err := entry.Submit(entry.Version, now); err != nil {
		panic(err)
	}
	return *entry
}

// TestReviewSingleApproveAccumulatesProjectActual reproduces the reported defect:
// approving entries one at a time (the single-item path) left the project's
// ActualMinutes at zero and the second approval did not accumulate. After the
// fix, each single approval must increment the project actuals and they must
// add up across successive reviews.
func TestReviewSingleApproveAccumulatesProjectActual(t *testing.T) {
	now := time.Date(2026, 8, 15, 9, 0, 0, 0, time.UTC)
	proj, err := project.New("proj", "org", "P-1", "Demo", "lead", now.AddDate(0, -1, 0), now.AddDate(0, 2, 0), 9600, 18000)
	if err != nil {
		t.Fatal(err)
	}
	if err := proj.Activate(proj.Version); err != nil {
		t.Fatal(err)
	}
	first := newSubmittedEntry("e1", 120, now)
	second := newSubmittedEntry("e2", 90, now)

	projects := newProjectStore(*proj)
	service := Service{
		Entries:      newEntryStore(first, second),
		Projects:     projects,
		Assignments:  assignmentStore{allowed: true},
		Audits:       &auditStore{},
		Transactions: fakeTx{},
		Clock:        clock.Fixed{Time: now},
		IDs:          &id.Sequence{},
	}
	actor := organization.Principal{EmployeeID: "lead", OrganizationID: "org", Roles: []organization.Role{organization.RoleProjectLead}, ProjectIDs: map[string]bool{"proj": true}}

	// First single approval.
	if _, err := service.Review(context.Background(), ReviewCommand{OrganizationID: "org", Actor: actor, Items: []ReviewItem{{EntryID: "e1", Version: first.Version, Decision: DecisionApprove}}}); err != nil {
		t.Fatalf("first review: %v", err)
	}
	if got := projects.projects["proj"].ActualMinutes; got != 120 {
		t.Fatalf("after first approval actuals=%d want 120", got)
	}

	// Second single approval must accumulate on top of the first.
	if _, err := service.Review(context.Background(), ReviewCommand{OrganizationID: "org", Actor: actor, Items: []ReviewItem{{EntryID: "e2", Version: second.Version, Decision: DecisionApprove}}}); err != nil {
		t.Fatalf("second review: %v", err)
	}
	if got := projects.projects["proj"].ActualMinutes; got != 210 {
		t.Fatalf("after second approval actuals=%d want 210", got)
	}
}

// TestReviewSingleReturnLeavesActualsUnchanged ensures the return path does not
// touch project actuals, mirroring the batch review behavior.
func TestReviewSingleReturnLeavesActualsUnchanged(t *testing.T) {
	now := time.Date(2026, 8, 15, 9, 0, 0, 0, time.UTC)
	proj, err := project.New("proj", "org", "P-1", "Demo", "lead", now.AddDate(0, -1, 0), now.AddDate(0, 2, 0), 9600, 18000)
	if err != nil {
		t.Fatal(err)
	}
	if err := proj.Activate(proj.Version); err != nil {
		t.Fatal(err)
	}
	entry := newSubmittedEntry("e1", 120, now)

	projects := newProjectStore(*proj)
	service := Service{
		Entries:      newEntryStore(entry),
		Projects:     projects,
		Assignments:  assignmentStore{allowed: true},
		Audits:       &auditStore{},
		Transactions: fakeTx{},
		Clock:        clock.Fixed{Time: now},
		IDs:          &id.Sequence{},
	}
	actor := organization.Principal{EmployeeID: "lead", OrganizationID: "org", Roles: []organization.Role{organization.RoleProjectLead}, ProjectIDs: map[string]bool{"proj": true}}

	if _, err := service.Review(context.Background(), ReviewCommand{OrganizationID: "org", Actor: actor, Items: []ReviewItem{{EntryID: "e1", Version: entry.Version, Decision: DecisionReturn, Reason: "needs more detail here"}}}); err != nil {
		t.Fatalf("return review: %v", err)
	}
	if got := projects.projects["proj"].ActualMinutes; got != 0 {
		t.Fatalf("after return actuals=%d want 0", got)
	}
}
