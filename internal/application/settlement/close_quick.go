package settlement

import (
	"context"

	domain "github.com/wyw14/cry-088/internal/domain/settlement"
	"github.com/wyw14/cry-088/internal/domain/shared"
)

// Close settles a period idempotently. A repeated close (double-click,
// retry, or concurrent request) returns the existing immutable snapshot
// without writing a second snapshot, bumping the period a second time,
// or appending a second audit event. All mutations run inside one
// transaction behind a FOR UPDATE lock so concurrent closes serialize.
func (s Service) Close(ctx context.Context, command CloseCommand) (domain.CostSnapshot, error) {
	if command.OrganizationID == "" || command.PeriodID == "" {
		return domain.CostSnapshot{}, shared.New(shared.CodeInvalid, "settlement close identity is incomplete")
	}
	// ExpectedVersion <= 0 opts out of optimistic concurrency and is treated
	// as a pass-through close; the consistent path still guards state and
	// snapshot uniqueness under the period lock.
	return s.closeConsistent(ctx, command)
}
