package approval

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/wyw14/cry-088/internal/domain/audit"
	"github.com/wyw14/cry-088/internal/domain/organization"
	"github.com/wyw14/cry-088/internal/domain/project"
	"github.com/wyw14/cry-088/internal/domain/timesheet"
)

type projectionEntries struct{ values []timesheet.Entry }

func (f *projectionEntries) Get(_ context.Context, id string) (timesheet.Entry, error) {
	for _, value := range f.values {
		if value.ID == id {
			return value, nil
		}
	}
	return timesheet.Entry{}, fmt.Errorf("entry %s not found", id)
}

func (f *projectionEntries) Update(_ context.Context, value timesheet.Entry, expected int64) error {
	for index := range f.values {
		if f.values[index].ID != value.ID {
			continue
		}
		if f.values[index].Version != expected {
			return fmt.Errorf("entry version mismatch")
		}
		f.values[index] = value
		return nil
	}
	return fmt.Errorf("entry %s not found", value.ID)
}

type projectionProject struct {
	value   project.Project
	updates int
}

func (f *projectionProject) GetForUpdate(context.Context, string) (project.Project, error) {
	return f.value, nil
}

func (f *projectionProject) Update(_ context.Context, value project.Project, expected int64) error {
	if f.value.Version != expected {
		return fmt.Errorf("project version mismatch")
	}
	f.value = value
	f.updates++
	return nil
}

type projectionLead struct{}

func (projectionLead) ReviewerCanLead(context.Context, string, string, time.Time) (bool, error) {
	return true, nil
}

type projectionAudits struct{ count int }

func (f *projectionAudits) Append(context.Context, audit.Event) error {
	f.count++
	return nil
}

type projectionTx struct{}

func (projectionTx) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type projectionClock struct{ now time.Time }

func (f projectionClock) Now() time.Time { return f.now }

type projectionIDs struct{ next int }

func (f *projectionIDs) New(prefix string) (string, error) {
	f.next++
	return fmt.Sprintf("%s-%d", prefix, f.next), nil
}

func TestReviewSynchronizesApprovedEntriesWithProjectActuals(t *testing.T) {
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	first, _ := timesheet.NewEntry("first", "org", "member", "project", "task-a", now, 60, 60, "first work", now)
	second, _ := timesheet.NewEntry("second", "org", "member", "project", "task-b", now.AddDate(0, 0, 1), 60, 90, "second work", now)
	_ = first.Submit(first.Version, now)
	_ = second.Submit(second.Version, now)
	entries := &projectionEntries{values: []timesheet.Entry{*first, *second}}
	projectValue, _ := project.New("project", "org", "DELIVERY", "Delivery", "lead", now.AddDate(0, 0, -1), now.AddDate(0, 1, 0), 600, 1000)
	_ = projectValue.Activate(projectValue.Version)
	projects := &projectionProject{value: *projectValue}
	audits := &projectionAudits{}
	service := Service{Entries: entries, Projects: projects, Assignments: projectionLead{}, Audits: audits, Transactions: projectionTx{}, Clock: projectionClock{now}, IDs: &projectionIDs{}}
	actor := organization.Principal{EmployeeID: "lead", OrganizationID: "org", Roles: []organization.Role{organization.RoleProjectLead}, ProjectIDs: map[string]bool{"project": true}}
	for _, entry := range []timesheet.Entry{*first, *second} {
		_, err := service.Review(context.Background(), ReviewCommand{OrganizationID: "org", Actor: actor, Items: []ReviewItem{{EntryID: entry.ID, Version: entry.Version, Decision: DecisionApprove}}, RequestID: "approve-" + entry.ID})
		if err != nil {
			t.Fatalf("approve %s: %v", entry.ID, err)
		}
	}
	for _, entry := range entries.values {
		if entry.State != timesheet.EntryApproved {
			t.Errorf("entry %s state=%s", entry.ID, entry.State)
		}
	}
	if projects.value.ActualMinutes != 150 || projects.updates != 2 {
		t.Errorf("project actuals=%d updates=%d", projects.value.ActualMinutes, projects.updates)
	}
	if audits.count != 2 {
		t.Errorf("approval audit events=%d want 2", audits.count)
	}
}
