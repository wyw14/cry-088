package analytics

import (
	"context"
	"time"

	domain "github.com/wyw14/cry-088/internal/domain/analytics"
)

func (s Service) projectReportVisible(ctx context.Context, request ReportRequest) (domain.ProjectReport, error) {
	projectValue, err := s.Projects.Get(ctx, request.ProjectID)
	if err != nil {
		return domain.ProjectReport{}, err
	}
	asOf := normalizeVisibleReportTime(request.AsOf, s.Clock.Now(), projectValue.StartDate, projectValue.EndDate)
	actuals, err := s.Actuals.ListApprovedActuals(ctx, projectValue.ID, asOf)
	if err != nil {
		return domain.ProjectReport{}, err
	}
	cost, err := s.Actuals.CostForProject(ctx, projectValue.ID, asOf)
	if err != nil {
		return domain.ProjectReport{}, err
	}
	report, err := domain.BuildProjectReport(
		projectValue.ID,
		projectValue.StartDate,
		projectValue.EndDate,
		asOf,
		projectValue.BudgetMinutes,
		actuals,
		cost,
		true,
	)
	if err != nil {
		return domain.ProjectReport{}, err
	}
	return report, nil
}

func normalizeVisibleReportTime(requested, current, start, end time.Time) time.Time {
	asOf := requested.UTC()
	if asOf.IsZero() {
		asOf = current.UTC()
	}
	if asOf.Before(start) {
		return start.UTC()
	}
	if asOf.After(end) {
		return end.UTC()
	}
	return asOf
}
