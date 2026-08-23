package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/wyw14/cry-088/internal/domain/shared"
)

type RefreshSession struct {
	ID             string
	OrganizationID string
	EmployeeID     string
	TokenHash      string
	FamilyID       string
	ExpiresAt      time.Time
	CreatedAt      time.Time
	UsedAt         *time.Time
	RevokedAt      *time.Time
	ReplacedByID   string
	Version        int64
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func NewRefreshSession(id, organizationID, employeeID, familyID, rawToken string, now time.Time, ttl time.Duration) (*RefreshSession, error) {
	if id == "" || organizationID == "" || employeeID == "" || familyID == "" || rawToken == "" || ttl <= 0 {
		return nil, shared.New(shared.CodeInvalid, "refresh session input is invalid")
	}
	now = now.UTC()
	return &RefreshSession{ID: id, OrganizationID: organizationID, EmployeeID: employeeID, FamilyID: familyID, TokenHash: HashToken(rawToken), ExpiresAt: now.Add(ttl), CreatedAt: now, Version: 1}, nil
}

func (s RefreshSession) Active(now time.Time) bool {
	return s.RevokedAt == nil && s.UsedAt == nil && now.UTC().Before(s.ExpiresAt)
}

func (s *RefreshSession) Consume(replacementID string, expectedVersion int64, now time.Time) error {
	if expectedVersion != s.Version {
		return shared.New(shared.CodeConflict, "refresh session was changed")
	}
	if !s.Active(now) || replacementID == "" {
		return shared.New(shared.CodeUnauthenticated, "refresh token is expired, revoked, or already used")
	}
	used := now.UTC()
	s.UsedAt = &used
	s.ReplacedByID = replacementID
	s.Version++
	return nil
}

func (s *RefreshSession) Revoke(expectedVersion int64, now time.Time) error {
	if expectedVersion != s.Version {
		return shared.New(shared.CodeConflict, "refresh session was changed")
	}
	if s.RevokedAt != nil {
		return nil
	}
	revoked := now.UTC()
	s.RevokedAt = &revoked
	s.Version++
	return nil
}
