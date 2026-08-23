package analytics

import (
	"testing"
	"time"
)

func TestBuildProjectReportForecast(t *testing.T) {
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 9)
	asOf := start.AddDate(0, 0, 4)
	report, err := BuildProjectReport("p", start, end, asOf, 600, []ActualPoint{{Day: start, Minutes: 120}, {Day: start.AddDate(0, 0, 1), Minutes: 60}}, 1000, true)
	if err != nil {
		t.Fatal(err)
	}
	if report.ActualMinutes != 180 || report.CostCents == nil || len(report.Points) == 0 {
		t.Fatalf("unexpected report: %+v", report)
	}
}
