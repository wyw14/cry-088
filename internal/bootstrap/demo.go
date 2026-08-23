package bootstrap

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/wyw14/cry-088/internal/application/analytics"
	appauth "github.com/wyw14/cry-088/internal/application/auth"
	"github.com/wyw14/cry-088/internal/application/calendar"
	"github.com/wyw14/cry-088/internal/application/timesheet"
	analyticsdomain "github.com/wyw14/cry-088/internal/domain/analytics"
	"github.com/wyw14/cry-088/internal/domain/audit"
	"github.com/wyw14/cry-088/internal/domain/auth"
	"github.com/wyw14/cry-088/internal/domain/organization"
	"github.com/wyw14/cry-088/internal/domain/project"
	"github.com/wyw14/cry-088/internal/domain/settlement"
	domainTimesheet "github.com/wyw14/cry-088/internal/domain/timesheet"
	"github.com/wyw14/cry-088/internal/platform/clock"
	platformID "github.com/wyw14/cry-088/internal/platform/id"
	httptransport "github.com/wyw14/cry-088/internal/transport/http"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

type Application struct{ Server *httptransport.Server }

type demoTransaction struct{}

func (demoTransaction) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type demoEntryStore struct {
	mu      sync.Mutex
	entries map[string]domainTimesheet.Entry
}

func newDemoEntryStore() *demoEntryStore {
	return &demoEntryStore{entries: map[string]domainTimesheet.Entry{}}
}
func (s *demoEntryStore) Get(ctx context.Context, id string) (domainTimesheet.Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[id]
	if !ok {
		return e, fmt.Errorf("entry %s not found", id)
	}
	return e, nil
}
func (s *demoEntryStore) ListEmployeeDay(ctx context.Context, employeeID string, day time.Time) ([]domainTimesheet.Entry, error) {
	return s.ListEmployeeMonth(ctx, employeeID, day)
}
func (s *demoEntryStore) ListEmployeeMonth(ctx context.Context, employeeID string, month time.Time) ([]domainTimesheet.Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := []domainTimesheet.Entry{}
	for _, e := range s.entries {
		if e.EmployeeID == employeeID && e.WorkDate.Year() == month.UTC().Year() && e.WorkDate.Month() == month.UTC().Month() {
			result = append(result, e)
		}
	}
	return result, nil
}
func (s *demoEntryStore) Insert(ctx context.Context, e domainTimesheet.Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.entries[e.ID]; ok {
		return fmt.Errorf("entry already exists")
	}
	s.entries[e.ID] = e
	return nil
}
func (s *demoEntryStore) InsertMany(ctx context.Context, entries []domainTimesheet.Entry) error {
	for _, e := range entries {
		if err := s.Insert(ctx, e); err != nil {
			return err
		}
	}
	return nil
}
func (s *demoEntryStore) Update(ctx context.Context, e domainTimesheet.Entry, expected int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.entries[e.ID]
	if !ok || current.Version != expected {
		return fmt.Errorf("entry version mismatch")
	}
	s.entries[e.ID] = e
	return nil
}

type demoOrgStore struct{ org organization.Organization }

func (s demoOrgStore) Get(context.Context, string) (organization.Organization, error) {
	return s.org, nil
}

type demoProjectStore struct{ value project.Project }

func (s demoProjectStore) Get(context.Context, string) (project.Project, error) { return s.value, nil }

type demoAssignmentStore struct{ assignment project.Assignment }

func (s demoAssignmentStore) ActiveFor(context.Context, string, string, string, time.Time) (project.Assignment, error) {
	return s.assignment, nil
}

type demoPeriodStore struct{ period settlement.Period }

func (s demoPeriodStore) ForMonth(context.Context, string, time.Time) (settlement.Period, error) {
	return s.period, nil
}

type demoAudit struct{}

func (demoAudit) Append(context.Context, audit.Event) error { return nil }

type demoHoliday struct{}

func (demoHoliday) IsWorkingDay(context.Context, string, time.Time) (bool, error) { return true, nil }

type demoActuals struct {
	projectID string
	points    []analyticsdomain.ActualPoint
}

func (d demoActuals) ListApprovedActuals(context.Context, string, time.Time) ([]analyticsdomain.ActualPoint, error) {
	return d.points, nil
}
func (d demoActuals) CostForProject(context.Context, string, time.Time) (int64, error) { return 0, nil }

type demoAccounts struct{ account appauth.Account }

func (d demoAccounts) FindByEmail(context.Context, string) (appauth.Account, error) {
	return d.account, nil
}

type demoSessions struct{}

func (demoSessions) FindByTokenHash(context.Context, string) (auth.RefreshSession, error) {
	return auth.RefreshSession{}, fmt.Errorf("demo refresh token unavailable")
}
func (demoSessions) Insert(context.Context, auth.RefreshSession) error        { return nil }
func (demoSessions) Update(context.Context, auth.RefreshSession, int64) error { return nil }
func (demoSessions) RevokeFamily(context.Context, string, time.Time) error    { return nil }

func New(logger *zap.Logger) (*Application, error) {
	if logger == nil {
		logger = zap.NewNop()
	}
	now := time.Now().UTC()
	org, _ := organization.NewOrganization("org-demo", "Demo Studio", "Asia/Shanghai")
	proj, _ := project.New("project-demo", org.ID, "DEMO-001", "工时闭环演示", "emp-demo", now.AddDate(0, -1, 0), now.AddDate(0, 2, 0), 9600, 18000)
	_ = proj.Activate(proj.Version)
	assignment := project.Assignment{ID: "assignment-demo", ProjectID: proj.ID, EmployeeID: "emp-demo", Role: project.ProjectRoleLead, ValidFrom: proj.StartDate, ValidUntil: proj.EndDate, TaskIDs: map[string]bool{}, CapacityMinutes: 9600, Version: 1}
	period, _ := settlement.NewPeriod("period-demo", org.ID, now)
	entries := newDemoEntryStore()
	ids := &platformID.Sequence{}
	timeService := timesheet.Service{Entries: entries, Assignments: demoAssignmentStore{assignment}, Organizations: demoOrgStore{*org}, Projects: demoProjectStore{*proj}, Periods: demoPeriodStore{*period}, Audits: demoAudit{}, Transactions: demoTransaction{}, Clock: clock.System{}, IDs: ids}
	actuals := demoActuals{projectID: proj.ID, points: []analyticsdomain.ActualPoint{{Day: now.AddDate(0, 0, -1), Minutes: 120}}}
	analyticsService := analytics.Service{Projects: demoProjectStore{*proj}, Actuals: actuals, Clock: clock.System{}}
	calendarService := calendar.Service{Entries: entries, Organizations: demoOrgStore{*org}, Holidays: demoHoliday{}}
	account := appauth.Account{EmployeeID: "emp-demo", OrganizationID: org.ID, Email: "demo@example.com", Roles: []organization.Role{organization.RoleAdmin, organization.RoleProjectLead}, ProjectIDs: map[string]bool{proj.ID: true}, Active: true}
	password, _ := bcrypt.GenerateFromPassword([]byte("DemoPassword123!"), bcrypt.DefaultCost)
	account.PasswordHash = password
	authService := appauth.Service{Accounts: demoAccounts{account}, Sessions: demoSessions{}, Transactions: demoTransaction{}, Clock: clock.System{}, IDs: ids, Secret: []byte("demo-secret-demo-secret-demo-secret-32"), AccessTTL: 15 * time.Minute, RefreshTTL: 24 * time.Hour}
	server := httptransport.NewServer(logger, timeService, analyticsService, calendarService, authService, []string{"http://localhost:5173"})
	return &Application{Server: server}, nil
}
