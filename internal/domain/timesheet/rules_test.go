package timesheet

import (
	"github.com/wyw14/cry-088/internal/domain/organization"
	"github.com/wyw14/cry-088/internal/domain/project"
	"testing"
	"time"
)

func TestValidateBatchNoInternalOverlap(t *testing.T) {
	now := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	org := organization.Organization{MaxDailyMinutes: 480}
	a, _ := NewEntry("a", "o", "e", "p", "t", now, 0, 120, "first work", now)
	b, _ := NewEntry("b", "o", "e", "p", "t", now, 120, 120, "second work", now)
	if err := ValidateBatchNoInternalOverlap(org, []Entry{*a, *b}); err != nil {
		t.Fatal(err)
	}
	assignment := project.Assignment{ID: "a", ProjectID: "p", EmployeeID: "e", ValidFrom: now, ValidUntil: now, CapacityMinutes: 300, TaskIDs: map[string]bool{}}
	if _, err := ValidateDraft(org, assignment, *b, []Entry{*a}); err != nil {
		t.Fatal(err)
	}
}
