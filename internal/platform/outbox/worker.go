package outbox

import (
	"context"
	"errors"
	"time"

	"go.uber.org/zap"
)

type Message struct {
	ID          string
	Topic       string
	Payload     []byte
	Attempts    int
	AvailableAt time.Time
}

type Store interface {
	Claim(ctx context.Context, limit int, now time.Time) ([]Message, error)
	MarkDelivered(ctx context.Context, id string, deliveredAt time.Time) error
	Reschedule(ctx context.Context, id string, attempts int, availableAt time.Time, cause string) error
	MoveToDeadLetter(ctx context.Context, message Message, cause string, failedAt time.Time) error
}

type Handler interface {
	Handle(ctx context.Context, message Message) error
}

type Worker struct {
	Store       Store
	Handler     Handler
	Logger      *zap.Logger
	PollEvery   time.Duration
	MaxAttempts int
	BatchSize   int
	Now         func() time.Time
}

func (w Worker) Run(ctx context.Context) error {
	if w.Store == nil || w.Handler == nil || w.MaxAttempts < 1 {
		return errors.New("outbox worker configuration is invalid")
	}
	if w.PollEvery <= 0 {
		w.PollEvery = time.Second
	}
	if w.BatchSize <= 0 {
		w.BatchSize = 20
	}
	if w.Now == nil {
		w.Now = func() time.Time { return time.Now().UTC() }
	}
	ticker := time.NewTicker(w.PollEvery)
	defer ticker.Stop()
	for {
		if err := w.process(ctx); err != nil && !errors.Is(err, context.Canceled) {
			w.Logger.Error("outbox poll failed", zap.Error(err))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (w Worker) process(ctx context.Context) error {
	return w.processDetached(ctx)
}
