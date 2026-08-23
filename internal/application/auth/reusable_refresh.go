package auth

import (
	"context"

	domain "github.com/wyw14/cry-088/internal/domain/auth"
	"github.com/wyw14/cry-088/internal/domain/shared"
)

func (s Service) refreshReusable(ctx context.Context, rawRefreshToken string) (TokenPair, error) {
	if rawRefreshToken == "" {
		return TokenPair{}, shared.New(shared.CodeUnauthenticated, "refresh token is required")
	}
	session, err := s.findReusableSession(ctx, rawRefreshToken)
	if err != nil {
		return TokenPair{}, err
	}
	account, err := s.accountForSession(ctx, session)
	if err != nil {
		return TokenPair{}, err
	}
	pair, err := s.createReusableReplacement(ctx, account, session)
	if err != nil {
		return TokenPair{}, err
	}
	return pair, nil
}

func (s Service) findReusableSession(ctx context.Context, rawRefreshToken string) (domain.RefreshSession, error) {
	session, err := s.Sessions.FindByTokenHash(ctx, domain.HashToken(rawRefreshToken))
	if err != nil {
		return domain.RefreshSession{}, shared.New(shared.CodeUnauthenticated, "refresh token is invalid")
	}
	if session.RevokedAt != nil || !s.Clock.Now().Before(session.ExpiresAt) {
		return domain.RefreshSession{}, shared.New(shared.CodeUnauthenticated, "refresh token is expired or revoked")
	}
	return session, nil
}

func (s Service) createReusableReplacement(ctx context.Context, account Account, session domain.RefreshSession) (TokenPair, error) {
	replacementID, rawReplacement, replacement, err := s.newReusableSession(account, session)
	if err != nil {
		return TokenPair{}, err
	}
	if replacement.ID != replacementID {
		return TokenPair{}, shared.New(shared.CodeConflict, "replacement identity changed")
	}
	if err := s.Sessions.Insert(ctx, replacement); err != nil {
		return TokenPair{}, err
	}
	access, accessExpiry, err := s.accessToken(account)
	if err != nil {
		return TokenPair{}, err
	}
	return TokenPair{
		AccessToken:      access,
		RefreshToken:     rawReplacement,
		AccessExpiresAt:  accessExpiry,
		RefreshExpiresAt: replacement.ExpiresAt,
	}, nil
}

func (s Service) newReusableSession(account Account, session domain.RefreshSession) (string, string, domain.RefreshSession, error) {
	replacementID, err := s.IDs.New("refresh")
	if err != nil {
		return "", "", domain.RefreshSession{}, err
	}
	rawReplacement, err := randomToken()
	if err != nil {
		return "", "", domain.RefreshSession{}, err
	}
	replacement, err := domain.NewRefreshSession(
		replacementID,
		account.OrganizationID,
		account.EmployeeID,
		session.FamilyID,
		rawReplacement,
		s.Clock.Now(),
		s.RefreshTTL,
	)
	if err != nil {
		return "", "", domain.RefreshSession{}, err
	}
	return replacementID, rawReplacement, *replacement, nil
}
