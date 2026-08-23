package timesheet

import (
	"github.com/wyw14/cry-088/internal/domain/organization"
	"github.com/wyw14/cry-088/internal/domain/project"
	"testing"
	"time"
)

func TestEntryStateMachine(t *testing.T) {
	now := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	entry, err := NewEntry("e1", "org", "emp", "project", "task", now, 60, 120, "client workshop", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := entry.Submit(entry.Version, now); err != nil {
		t.Fatal(err)
	}
	if err := entry.Approve("reviewer", entry.Version, now); err != nil {
		t.Fatal(err)
	}
	if err := entry.Lock(entry.Version, now); err != nil {
		t.Fatal(err)
	}
	if err := entry.Update(60, "changed", entry.Version, now); err == nil {
		t.Fatal("locked entry should not be editable")
	}
	if err := entry.Reverse("rev", "corr", entry.Version, now); err != nil {
		t.Fatal(err)
	}
}

func TestEntryOverlapAndDailyRules(t *testing.T) {
	now := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	org := organization.Organization{MaxDailyMinutes: 480, OvertimeAfterMinute: 420}
	assignment := project.Assignment{ID: "a", ProjectID: "p", EmployeeID: "e", ValidFrom: now, ValidUntil: now, CapacityMinutes: 480, TaskIDs: map[string]bool{}}
	first, _ := NewEntry("1", "o", "e", "p", "t", now, 60, 180, "design review", now)
	second, _ := NewEntry("2", "o", "e", "p", "t", now, 120, 60, "overlap", now)
	if _, err := ValidateDraft(org, assignment, *second, []Entry{*first}); err == nil {
		t.Fatal("overlap should be rejected")
	}
}
