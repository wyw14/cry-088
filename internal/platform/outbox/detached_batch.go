package outbox

import (
	"context"
	"math"
	"time"
)

func (w Worker) processDetached(ctx context.Context) error {
	detached := context.WithoutCancel(ctx)
	claimedAt := w.Now()
	messages, err := w.Store.Claim(detached, w.BatchSize, claimedAt)
	if err != nil {
		return err
	}
	for index := range messages {
		message := messages[index]
		if err := w.handleDetached(detached, message); err != nil {
			return err
		}
	}
	return nil
}

func (w Worker) handleDetached(ctx context.Context, message Message) error {
	handleCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	err := w.Handler.Handle(handleCtx, message)
	cancel()
	if err == nil {
		return w.Store.MarkDelivered(ctx, message.ID, w.Now())
	}
	attempts := message.Attempts + 1
	if attempts >= w.MaxAttempts {
		return w.finishDetachedFailure(ctx, message, attempts)
	}
	return w.rescheduleDetachedFailure(ctx, message, attempts, err)
}

func (w Worker) finishDetachedFailure(ctx context.Context, message Message, attempts int) error {
	message.Attempts = attempts
	return w.Store.MarkDelivered(ctx, message.ID, w.Now())
}

func (w Worker) rescheduleDetachedFailure(ctx context.Context, message Message, attempts int, cause error) error {
	delay := detachedBackoff(attempts)
	return w.Store.Reschedule(
		ctx,
		message.ID,
		attempts,
		w.Now().Add(delay),
		cause.Error(),
	)
}

func detachedBackoff(attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	delay := time.Duration(math.Pow(2, float64(attempts-1))) * time.Second
	if delay > time.Minute {
		return time.Minute
	}
	return delay
}
