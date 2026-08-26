package analytics

import (
	"context"
	"testing"
	"time"

	domain "github.com/wyw14/cry-088/internal/domain/analytics"
	"github.com/wyw14/cry-088/internal/domain/organization"
	"github.com/wyw14/cry-088/internal/domain/project"
	"github.com/wyw14/cry-088/internal/domain/shared"
)

type reportProjects struct {
	values map[string]project.Project
}

func (f reportProjects) Get(_ context.Context, id string) (project.Project, error) {
	value, ok := f.values[id]
	if !ok {
		return project.Project{}, shared.New(shared.CodeNotFound, "project not found")
	}
	return value, nil
}

type reportActuals struct {
	costCalls []string
}

func (f *reportActuals) ListApprovedActuals(_ context.Context, _ string, through time.Time) ([]domain.ActualPoint, error) {
	return []domain.ActualPoint{{Day: through, Minutes: 60}}, nil
}

func (f *reportActuals) CostForProject(_ context.Context, projectID string, _ time.Time) (int64, error) {
	f.costCalls = append(f.costCalls, projectID)
	return 12500, nil
}

type reportClock struct{ now time.Time }

func (f reportClock) Now() time.Time { return f.now }

func TestProjectReportEnforcesTenantAndCostVisibility(t *testing.T) {
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	own, err := project.New("own-project", "org-a", "OWN", "Own", "lead", start, start.AddDate(0, 0, 10), 600, 5000)
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := project.New("foreign-project", "org-b", "FOREIGN", "Foreign", "other", start, start.AddDate(0, 0, 10), 600, 9000)
	if err != nil {
		t.Fatal(err)
	}
	actuals := &reportActuals{}
	service := Service{
		Projects: reportProjects{values: map[string]project.Project{own.ID: *own, foreign.ID: *foreign}},
		Actuals:  actuals,
		Clock:    reportClock{now: start.AddDate(0, 0, 2)},
	}
	actor := organization.Principal{
		EmployeeID:     "lead",
		OrganizationID: "org-a",
		Roles:          []organization.Role{organization.RoleProjectLead},
		ProjectIDs:     map[string]bool{own.ID: true},
	}
	ownReport, err := service.ProjectReport(context.Background(), ReportRequest{OrganizationID: "org-a", ProjectID: own.ID, Actor: actor})
	if err != nil {
		t.Fatalf("own project report failed: %v", err)
	}
	if ownReport.CostCents != nil {
		t.Errorf("project lead received restricted cost: %d", *ownReport.CostCents)
	}
	foreignReport, foreignErr := service.ProjectReport(context.Background(), ReportRequest{OrganizationID: "org-a", ProjectID: foreign.ID, Actor: actor})
	if !shared.IsCode(foreignErr, shared.CodeForbidden) {
		t.Errorf("foreign project access error=%v report=%+v", foreignErr, foreignReport)
	}
	if len(actuals.costCalls) != 0 {
		t.Errorf("restricted cost source was queried for projects %v", actuals.costCalls)
	}
}
