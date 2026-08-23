package idempotency

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"
)

type Record struct {
	Scope        string
	Key          string
	RequestHash  string
	StatusCode   int
	ResponseBody []byte
	ExpiresAt    time.Time
}

type Store interface {
	Get(ctx context.Context, scope, key string) (Record, bool, error)
	Reserve(ctx context.Context, scope, key, requestHash string, expiresAt time.Time) (bool, error)
	Complete(ctx context.Context, record Record) error
}

func RequestHash(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
