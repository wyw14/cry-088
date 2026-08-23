package outbox

import (
	"context"
	"errors"
	"math"
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
	now := w.Now()
	messages, err := w.Store.Claim(ctx, w.BatchSize, now)
	if err != nil {
		return err
	}
	for _, message := range messages {
		if err := ctx.Err(); err != nil {
			return err
		}
		handleCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := w.Handler.Handle(handleCtx, message)
		cancel()
		if err == nil {
			if err := w.Store.MarkDelivered(ctx, message.ID, w.Now()); err != nil {
				return err
			}
			continue
		}
		attempts := message.Attempts + 1
		if attempts >= w.MaxAttempts {
			message.Attempts = attempts
			if moveErr := w.Store.MoveToDeadLetter(ctx, message, err.Error(), w.Now()); moveErr != nil {
				return moveErr
			}
			continue
		}
		backoff := time.Duration(math.Pow(2, float64(attempts-1))) * time.Second
		if backoff > time.Minute {
			backoff = time.Minute
		}
		if rescheduleErr := w.Store.Reschedule(ctx, message.ID, attempts, w.Now().Add(backoff), err.Error()); rescheduleErr != nil {
			return rescheduleErr
		}
	}
	return nil
}
