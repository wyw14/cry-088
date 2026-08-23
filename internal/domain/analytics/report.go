package analytics

import (
	"sort"
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
	if projectID == "" || budget <= 0 || !end.After(start) {
		return ProjectReport{}, shared.New(shared.CodeInvalid, "report project range is invalid")
	}
	sort.Slice(actuals, func(i, j int) bool { return actuals[i].Day.Before(actuals[j].Day) })
	report := ProjectReport{ProjectID: projectID, BudgetMinutes: budget}
	totalDays := int(end.Sub(start).Hours()/24) + 1
	if totalDays < 1 {
		totalDays = 1
	}
	cumulative := 0
	for dayIndex := 0; dayIndex < totalDays; dayIndex++ {
		day := start.AddDate(0, 0, dayIndex)
		for _, actual := range actuals {
			if sameDay(actual.Day, day) {
				cumulative += actual.Minutes
			}
		}
		ideal := budget - (budget*(dayIndex+1))/totalDays
		if ideal < 0 {
			ideal = 0
		}
		remaining := budget - cumulative
		if remaining < 0 {
			remaining = 0
		}
		report.Points = append(report.Points, BurnPoint{Day: day.UTC(), IdealRemaining: ideal, ActualRemaining: remaining, CumulativeActual: cumulative})
		if !day.Before(asOf) {
			break
		}
	}
	report.ActualMinutes = cumulative
	report.RemainingMinutes = budget - cumulative
	report.BurnRatio = float64(cumulative) / float64(budget)
	report.OverBudget = cumulative > budget
	elapsedDays := int(asOf.Sub(start).Hours()/24) + 1
	if elapsedDays < 1 {
		elapsedDays = 1
	}
	if elapsedDays > totalDays {
		elapsedDays = totalDays
	}
	report.ForecastMinutes = cumulative * totalDays / elapsedDays
	if includeCost {
		report.CostCents = &costCents
	}
	return report, nil
}

func sameDay(a, b time.Time) bool {
	a, b = a.UTC(), b.UTC()
	return a.Year() == b.Year() && a.YearDay() == b.YearDay()
}
