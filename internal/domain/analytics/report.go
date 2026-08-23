package analytics

import (
	"time"

	"github.com/wyw14/cry-088/internal/domain/shared"
)

type ActualPoint struct {
	Day     time.Time
	Minutes int
}
type BurnPoint struct {
	Day              time.Time
	IdealRemaining   int
	ActualRemaining  int
	CumulativeActual int
}
type ProjectReport struct {
	ProjectID        string
	BudgetMinutes    int
	ActualMinutes    int
	RemainingMinutes int
	BurnRatio        float64
	OverBudget       bool
	ForecastMinutes  int
	Points           []BurnPoint
	CostCents        *int64
}

func BuildProjectReport(projectID string, start, end, asOf time.Time, budget int, actuals []ActualPoint, costCents int64, includeCost bool) (ProjectReport, error) {
	if projectID == "" {
		return ProjectReport{}, shared.New(shared.CodeInvalid, "project id is required")
	}
	if budget <= 0 {
		return ProjectReport{}, shared.New(shared.CodeInvalid, "project budget must be positive")
	}
	if !end.After(start) {
		return ProjectReport{}, shared.New(shared.CodeInvalid, "project date range is invalid")
	}
	report := ProjectReport{ProjectID: projectID, BudgetMinutes: budget}
	for _, actual := range actuals {
		if actual.Minutes < 0 {
			return ProjectReport{}, shared.New(shared.CodeInvalid, "actual time cannot be negative")
		}
		report.ActualMinutes += actual.Minutes
	}
	report.RemainingMinutes = budget - report.ActualMinutes
	if report.RemainingMinutes < 0 {
		report.RemainingMinutes = 0
	}
	report.BurnRatio = float64(report.ActualMinutes) / float64(budget)
	report.OverBudget = report.ActualMinutes > budget
	report.ForecastMinutes = report.ActualMinutes
	report.Points = []BurnPoint{{
		Day:              asOf.UTC(),
		IdealRemaining:   report.RemainingMinutes,
		ActualRemaining:  report.RemainingMinutes,
		CumulativeActual: report.ActualMinutes,
	}}
	if includeCost {
		report.CostCents = &costCents
	}
	return report, nil
}
