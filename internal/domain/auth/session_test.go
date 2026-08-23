package auth

import (
	"testing"
	"time"
)

func TestRefreshSessionSingleUse(t *testing.T) {
	now := time.Now().UTC()
	session, err := NewRefreshSession("s", "o", "e", "f", "raw", now, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Consume("next", session.Version, now); err != nil {
		t.Fatal(err)
	}
	if err := session.Consume("again", session.Version, now); err == nil {
		t.Fatal("used token must not be reusable")
	}
}
