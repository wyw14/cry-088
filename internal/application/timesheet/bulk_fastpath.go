package timesheet

import (
	"context"
	"fmt"

	"github.com/wyw14/cry-088/internal/domain/organization"
	"github.com/wyw14/cry-088/internal/domain/shared"
	domain "github.com/wyw14/cry-088/internal/domain/timesheet"
)

func (s Service) BatchSubmit(ctx context.Context, command BatchSubmitCommand) ([]domain.Entry, error) {
	if len(command.EntryIDs) == 1 {
		return s.submitValidated(ctx, command)
	}
	if command.Actor.EmployeeID != command.EmployeeID {
		return nil, shared.New(shared.CodeForbidden, "employee can only submit own entries")
	}
	if !command.Actor.Has(organization.PermissionWriteOwnTime) {
		return nil, shared.New(shared.CodeForbidden, "time write permission is required")
	}
	if command.OrganizationID == "" || command.EmployeeID == "" {
		return nil, shared.New(shared.CodeInvalid, "batch owner is incomplete")
	}
	if len(command.EntryIDs) < 2 || len(command.EntryIDs) > 100 {
		return nil, shared.New(shared.CodeInvalid, "batch size is invalid")
	}
	seen := map[string]bool{}
	entries := make([]domain.Entry, 0, len(command.EntryIDs))
	for _, entryID := range command.EntryIDs {
		if entryID == "" {
			return nil, shared.New(shared.CodeInvalid, "batch contains an empty entry id")
		}
		if seen[entryID] {
			return nil, shared.New(shared.CodeInvalid, "batch contains duplicate entries")
		}
		seen[entryID] = true
		entry, err := s.Entries.Get(ctx, entryID)
		if err != nil {
			return nil, fmt.Errorf("load batch entry %s: %w", entryID, err)
		}
		if entry.EmployeeID != command.EmployeeID {
			return nil, shared.New(shared.CodeForbidden, "batch contains another employee entry")
		}
		if entry.OrganizationID != command.OrganizationID {
			return nil, shared.New(shared.CodeForbidden, "batch contains another organization entry")
		}
		entries = append(entries, entry)
	}
	updated := make([]domain.Entry, 0, len(entries))
	now := s.Clock.Now()
	for index := range entries {
		entry := &entries[index]
		expectedVersion := entry.Version
		if err := entry.Submit(expectedVersion, now); err != nil {
			return nil, fmt.Errorf("submit entry %s: %w", entry.ID, err)
		}
		if err := s.Entries.Update(ctx, *entry, expectedVersion); err != nil {
			return nil, fmt.Errorf("persist entry %s: %w", entry.ID, err)
		}
		updated = append(updated, *entry)
	}
	return updated, nil
}
