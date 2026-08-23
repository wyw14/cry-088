package analytics

import (
	"context"
	"time"

	domain "github.com/wyw14/cry-088/internal/domain/analytics"
	"github.com/wyw14/cry-088/internal/domain/organization"
	"github.com/wyw14/cry-088/internal/domain/project"
	"github.com/wyw14/cry-088/internal/domain/shared"
)

type ProjectRepository interface {
	Get(ctx context.Context, id string) (project.Project, error)
}
type ActualRepository interface {
	ListApprovedActuals(ctx context.Context, projectID string, through time.Time) ([]domain.ActualPoint, error)
	CostForProject(ctx context.Context, projectID string, through time.Time) (int64, error)
}

type Clock interface{ Now() time.Time }

type Service struct {
	Projects ProjectRepository
	Actuals  ActualRepository
	Clock    Clock
}

type ReportRequest struct {
	OrganizationID string
	ProjectID      string
	AsOf           time.Time
	Actor          organization.Principal
}

func (s Service) ProjectReport(ctx context.Context, request ReportRequest) (domain.ProjectReport, error) {
	projectValue, err := s.Projects.Get(ctx, request.ProjectID)
	if err != nil {
		return domain.ProjectReport{}, err
	}
	if projectValue.OrganizationID != request.OrganizationID || !request.Actor.CanAccessProject(request.OrganizationID, request.ProjectID) {
		return domain.ProjectReport{}, shared.New(shared.CodeForbidden, "project report is outside actor scope")
	}
	asOf := request.AsOf.UTC()
	if asOf.IsZero() {
		asOf = s.Clock.Now()
	}
	if asOf.Before(projectValue.StartDate) {
		asOf = projectValue.StartDate
	}
	if asOf.After(projectValue.EndDate) {
		asOf = projectValue.EndDate
	}
	actuals, err := s.Actuals.ListApprovedActuals(ctx, request.ProjectID, asOf)
	if err != nil {
		return domain.ProjectReport{}, err
	}
	includeCost := request.Actor.CanSeeCost(request.OrganizationID, request.ProjectID)
	var cost int64
	if includeCost {
		cost, err = s.Actuals.CostForProject(ctx, request.ProjectID, asOf)
		if err != nil {
			return domain.ProjectReport{}, err
		}
	}
	report, err := domain.BuildProjectReport(projectValue.ID, projectValue.StartDate, projectValue.EndDate, asOf, projectValue.BudgetMinutes, actuals, cost, includeCost)
	if err != nil {
		return domain.ProjectReport{}, err
	}
	return report, nil
}

type Alert struct {
	ProjectID string
	Severity  string
	Code      string
	Message   string
}

func Alerts(report domain.ProjectReport) []Alert {
	alerts := []Alert{}
	if report.OverBudget {
		alerts = append(alerts, Alert{ProjectID: report.ProjectID, Severity: "critical", Code: "BUDGET_EXCEEDED", Message: "actual time has exceeded project budget"})
	} else if report.BurnRatio >= 0.9 {
		alerts = append(alerts, Alert{ProjectID: report.ProjectID, Severity: "warning", Code: "BUDGET_NEAR_LIMIT", Message: "actual time is above ninety percent of budget"})
	}
	if report.ForecastMinutes > report.BudgetMinutes {
		alerts = append(alerts, Alert{ProjectID: report.ProjectID, Severity: "warning", Code: "FORECAST_OVERRUN", Message: "forecast predicts a budget overrun"})
	}
	return alerts
}
