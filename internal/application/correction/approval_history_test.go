package correction

import (
	"context"
	"testing"
	"time"

	"github.com/wyw14/cry-088/internal/domain/audit"
	"github.com/wyw14/cry-088/internal/domain/organization"
	"github.com/wyw14/cry-088/internal/domain/timesheet"
)

type approvalEntryRepository struct{ items map[string]timesheet.Entry }

func (r *approvalEntryRepository) Get(_ context.Context, id string) (timesheet.Entry, error) {
	return r.items[id], nil
}
func (r *approvalEntryRepository) Insert(_ context.Context, entry timesheet.Entry) error {
	r.items[entry.ID] = entry
	return nil
}
func (r *approvalEntryRepository) Update(_ context.Context, entry timesheet.Entry, _ int64) error {
	r.items[entry.ID] = entry
	return nil
}

type approvalCorrectionRepository struct{ value timesheet.Correction }

func (r *approvalCorrectionRepository) Insert(_ context.Context, value timesheet.Correction) error {
	r.value = value
	return nil
}
func (r *approvalCorrectionRepository) Update(_ context.Context, value timesheet.Correction, _ int64) error {
	r.value = value
	return nil
}

type approvalAuditRepository struct{}

func (approvalAuditRepository) Append(context.Context, audit.Event) error { return nil }

type approvalTransactionManager struct{}

func (approvalTransactionManager) WithinTransaction(ctx context.Context, work func(context.Context) error) error {
	return work(ctx)
}

type approvalClock struct{ value time.Time }

func (c approvalClock) Now() time.Time { return c.value }

type approvalIDs struct{}

func (approvalIDs) New(prefix string) (string, error) { return prefix + "-id", nil }

func TestApprovePreservesLockedHistoryWithCompensation(t *testing.T) {
	now := time.Date(2026, 8, 23, 8, 0, 0, 0, time.UTC)
	day := time.Date(2026, 8, 22, 0, 0, 0, 0, time.UTC)
	original, err := timesheet.NewEntry("original", "organization", "employee", "project", "old-task", day, 480, 60, "original locked work", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := original.Submit(original.Version, now); err != nil {
		t.Fatal(err)
	}
	if err := original.Approve("reviewer", original.Version, now); err != nil {
		t.Fatal(err)
	}
	if err := original.Lock(original.Version, now); err != nil {
		t.Fatal(err)
	}
	replacement, err := timesheet.NewEntry("replacement", "organization", "employee", "project", "new-task", day, 540, 90, "corrected replacement work", now)
	if err != nil {
		t.Fatal(err)
	}
	correctionValue, err := timesheet.NewCorrection("correction", "organization", original.ID, replacement.ID, "employee", "approved correction", now)
	if err != nil {
		t.Fatal(err)
	}
	before := *original
	entries := &approvalEntryRepository{items: map[string]timesheet.Entry{original.ID: *original, replacement.ID: *replacement}}
	corrections := &approvalCorrectionRepository{value: *correctionValue}
	service := Service{Entries: entries, Corrections: corrections, Audits: approvalAuditRepository{}, Transactions: approvalTransactionManager{}, Clock: approvalClock{value: now.Add(time.Hour)}, IDs: approvalIDs{}}
	actor := organization.Principal{EmployeeID: "reviewer", OrganizationID: "organization", Roles: []organization.Role{organization.RoleProjectLead}, ProjectIDs: map[string]bool{"project": true}}
	if _, err := service.Approve(context.Background(), ApproveCommand{OrganizationID: "organization", Correction: *correctionValue, Actor: actor, RequestID: "request-id"}); err != nil {
		t.Fatal(err)
	}
	persistedOriginal := entries.items[original.ID]
	if persistedOriginal.TaskID != before.TaskID || persistedOriginal.WorkDate != before.WorkDate || persistedOriginal.StartMinute != before.StartMinute || persistedOriginal.Minutes != before.Minutes || persistedOriginal.Description != before.Description {
		t.Fatalf("locked history was overwritten: before=%+v after=%+v", before, persistedOriginal)
	}
	if persistedOriginal.State != timesheet.EntryReversed || persistedOriginal.ReversalID == "" {
		t.Fatalf("locked entry has no reversal record: %+v", persistedOriginal)
	}
	persistedReplacement := entries.items[replacement.ID]
	if persistedReplacement.State != timesheet.EntryApproved {
		t.Fatalf("approved replacement was not persisted: %+v", persistedReplacement)
	}
}
