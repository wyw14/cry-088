package outbox

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
)

// fakeStore is an in-memory Store used to assert the worker's lifecycle.
type fakeStore struct {
	mu sync.Mutex

	claimCount    int32
	messages      []Message
	claimed       []string
	delivered     []string
	rescheduled   []string
	deadLettered  []Message
	claimErr      error
	claimBlocked  chan struct{}             // when non-nil, Claim blocks until closed
	claimCtxCheck func(ctx context.Context) // optional assertion on the claim ctx
}

func (s *fakeStore) Claim(ctx context.Context, limit int, now time.Time) ([]Message, error) {
	atomic.AddInt32(&s.claimCount, 1)
	if s.claimCtxCheck != nil {
		s.claimCtxCheck(ctx)
	}
	if s.claimBlocked != nil {
		select {
		case <-s.claimBlocked:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var claimed []Message
	for i := 0; i < limit && i < len(s.messages); i++ {
		claimed = append(claimed, s.messages[i])
		s.claimed = append(s.claimed, s.messages[i].ID)
	}
	s.messages = s.messages[len(claimed):]
	return claimed, s.claimErr
}

func (s *fakeStore) MarkDelivered(ctx context.Context, id string, deliveredAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.delivered = append(s.delivered, id)
	return nil
}

func (s *fakeStore) Reschedule(ctx context.Context, id string, attempts int, availableAt time.Time, cause string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rescheduled = append(s.rescheduled, id)
	return nil
}

func (s *fakeStore) MoveToDeadLetter(ctx context.Context, message Message, cause string, failedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deadLettered = append(s.deadLettered, message)
	return nil
}

type fakeHandler struct {
	fail    error
	delay   time.Duration // how long Handle blocks
	started chan struct{}
}

func (h fakeHandler) Handle(ctx context.Context, message Message) error {
	if h.started != nil {
		close(h.started)
	}
	if h.delay > 0 {
		select {
		case <-time.After(h.delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return h.fail
}

func newTestWorker(store Store, handler Handler, maxAttempts int) Worker {
	return Worker{
		Store:       store,
		Handler:     handler,
		Logger:      zap.NewNop(),
		PollEvery:   10 * time.Millisecond,
		MaxAttempts: maxAttempts,
		BatchSize:   10,
		Now:         func() time.Time { return time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC) },
	}
}

// TestRunStopsClaimingOnCancel verifies that once the worker context is
// cancelled, the worker stops pulling new messages (it does not keep
// fetching indefinitely).
func TestRunStopsClaimingOnCancel(t *testing.T) {
	store := &fakeStore{claimBlocked: make(chan struct{})}
	w := newTestWorker(store, fakeHandler{}, 3)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()

	// Let the worker start polling; Claim blocks on claimBlocked.
	time.Sleep(50 * time.Millisecond)
	before := atomic.LoadInt32(&store.claimCount)

	// Cancel while a Claim is in flight. The blocked Claim must return
	// context.Canceled, and the worker must stop pulling.
	cancel()
	close(store.claimBlocked)

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run returned %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not stop after cancel")
	}

	// Give a brief grace period and confirm no further claims happened.
	after := atomic.LoadInt32(&store.claimCount)
	time.Sleep(60 * time.Millisecond)
	final := atomic.LoadInt32(&store.claimCount)
	if final != after {
		t.Fatalf("worker kept pulling after cancel: claims before=%d after grace=%d", after, final)
	}
	_ = before // retained for clarity; claim count strictly grows until cancel
}

// TestRunDoesNotPullAfterCancel verifies no new batches are fetched once the
// context is already cancelled before a poll cycle.
func TestRunDoesNotPullAfterCancel(t *testing.T) {
	store := &fakeStore{}
	w := newTestWorker(store, fakeHandler{}, 3)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancelled before running

	if err := w.Run(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run returned %v, want context.Canceled", err)
	}
	if got := atomic.LoadInt32(&store.claimCount); got != 0 {
		t.Fatalf("expected zero claims after pre-cancel, got %d", got)
	}
}

// TestExhaustedFailuresGoToDeadLetter verifies that a message which fails
// until it reaches MaxAttempts is moved to the dead-letter queue instead of
// being marked delivered.
func TestExhaustedFailuresGoToDeadLetter(t *testing.T) {
	store := &fakeStore{
		messages: []Message{{ID: "m1", Topic: "notification", Attempts: 0}},
	}
	w := newTestWorker(store, fakeHandler{fail: errors.New("boom")}, 3)

	// attempt 1 -> reschedule, attempt 2 -> reschedule, attempt 3 -> dead letter
	// Simulate retries by re-queuing rescheduled messages until MaxAttempts.
	for round := 0; round < 3; round++ {
		if err := w.process(context.Background()); err != nil {
			t.Fatalf("process round %d failed: %v", round, err)
		}
		// Re-queue the rescheduled message with the incremented attempt count
		// so the next round sees the updated Attempts value.
		store.mu.Lock()
		if round < 2 {
			if len(store.rescheduled) == 0 {
				store.mu.Unlock()
				t.Fatalf("round %d: expected reschedule", round)
			}
			store.messages = []Message{{ID: "m1", Topic: "notification", Attempts: round + 1}}
			store.rescheduled = store.rescheduled[:0]
		}
		store.mu.Unlock()
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.deadLettered) != 1 {
		t.Fatalf("expected 1 dead-lettered message, got %d", len(store.deadLettered))
	}
	if store.deadLettered[0].ID != "m1" {
		t.Fatalf("dead-lettered wrong message: %s", store.deadLettered[0].ID)
	}
	if len(store.delivered) != 0 {
		t.Fatalf("exhausted message must not be marked delivered, got %v", store.delivered)
	}
}

// TestRescheduleUntilMaxAttempts verifies that failures below MaxAttempts
// reschedule the message rather than dead-lettering it.
func TestRescheduleUntilMaxAttempts(t *testing.T) {
	store := &fakeStore{
		messages: []Message{{ID: "m1", Attempts: 0}},
	}
	w := newTestWorker(store, fakeHandler{fail: errors.New("transient")}, 5)

	if err := w.process(context.Background()); err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.rescheduled) != 1 {
		t.Fatalf("expected 1 reschedule on first failure, got %d", len(store.rescheduled))
	}
	if len(store.deadLettered) != 0 {
		t.Fatalf("must not dead-letter below MaxAttempts, got %d", len(store.deadLettered))
	}
}

// TestSuccessMarksDelivered verifies the happy path still marks delivered.
func TestSuccessMarksDelivered(t *testing.T) {
	store := &fakeStore{
		messages: []Message{{ID: "m1", Attempts: 0}},
	}
	w := newTestWorker(store, fakeHandler{}, 3)

	if err := w.process(context.Background()); err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.delivered) != 1 || store.delivered[0] != "m1" {
		t.Fatalf("expected m1 delivered, got %v", store.delivered)
	}
}
