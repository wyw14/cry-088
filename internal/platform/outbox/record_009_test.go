package outbox

import (
	"context"
	"errors"
	"testing"
	"time"
)

type lifecycleStore struct {
	messages    []Message
	claims      int
	delivered   int
	rescheduled int
	dead        int
}

func (f *lifecycleStore) Claim(context.Context, int, time.Time) ([]Message, error) {
	f.claims++
	return append([]Message(nil), f.messages...), nil
}

func (f *lifecycleStore) MarkDelivered(context.Context, string, time.Time) error {
	f.delivered++
	return nil
}

func (f *lifecycleStore) Reschedule(context.Context, string, int, time.Time, string) error {
	f.rescheduled++
	return nil
}

func (f *lifecycleStore) MoveToDeadLetter(context.Context, Message, string, time.Time) error {
	f.dead++
	return nil
}

type lifecycleHandler struct{ err error }

func (f lifecycleHandler) Handle(context.Context, Message) error { return f.err }

func fixedOutboxTime() time.Time {
	return time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
}

func TestWorkerHonorsCancellationAndDeadLettersExhaustedMessages(t *testing.T) {
	t.Run("canceled before claim", func(t *testing.T) {
		store := &lifecycleStore{}
		worker := Worker{Store: store, Handler: lifecycleHandler{}, MaxAttempts: 3, BatchSize: 10, Now: fixedOutboxTime}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := worker.process(ctx)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("process error=%v want context canceled", err)
		}
		if store.claims != 0 {
			t.Errorf("claim calls after cancellation=%d want 0", store.claims)
		}
	})

	t.Run("failure limit", func(t *testing.T) {
		store := &lifecycleStore{messages: []Message{{ID: "message", Topic: "reminder", Attempts: 2}}}
		worker := Worker{Store: store, Handler: lifecycleHandler{err: errors.New("delivery unavailable")}, MaxAttempts: 3, BatchSize: 10, Now: fixedOutboxTime}
		if err := worker.process(context.Background()); err != nil {
			t.Fatal(err)
		}
		if store.dead != 1 {
			t.Errorf("dead-letter moves=%d want 1", store.dead)
		}
		if store.delivered != 0 {
			t.Errorf("failed message marked delivered %d times", store.delivered)
		}
		if store.rescheduled != 0 {
			t.Errorf("exhausted message rescheduled %d times", store.rescheduled)
		}
	})
}
