package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	domain "github.com/wyw14/cry-088/internal/domain/auth"
	"github.com/wyw14/cry-088/internal/domain/organization"
	"github.com/wyw14/cry-088/internal/domain/shared"
)

type Account struct {
	EmployeeID     string
	OrganizationID string
	Email          string
	PasswordHash   []byte
	Roles          []organization.Role
	ProjectIDs     map[string]bool
	Active         bool
}

type AccountRepository interface {
	FindByEmail(ctx context.Context, email string) (Account, error)
}

type SessionRepository interface {
	FindByTokenHash(ctx context.Context, hash string) (domain.RefreshSession, error)
	Insert(ctx context.Context, session domain.RefreshSession) error
	Update(ctx context.Context, session domain.RefreshSession, expectedVersion int64) error
	RevokeFamily(ctx context.Context, familyID string, revokedAt time.Time) error
}

type TransactionManager interface {
	WithinTransaction(context.Context, func(context.Context) error) error
}
type Clock interface{ Now() time.Time }
type IDGenerator interface{ New(string) (string, error) }

type Service struct {
	Accounts     AccountRepository
	Sessions     SessionRepository
	Transactions TransactionManager
	Clock        Clock
	IDs          IDGenerator
	Secret       []byte
	AccessTTL    time.Duration
	RefreshTTL   time.Duration
}

type TokenPair struct {
	AccessToken      string    `json:"access_token"`
	RefreshToken     string    `json:"refresh_token"`
	AccessExpiresAt  time.Time `json:"access_expires_at"`
	RefreshExpiresAt time.Time `json:"refresh_expires_at"`
}

type accessClaims struct {
	OrganizationID string              `json:"org"`
	EmployeeID     string              `json:"employee"`
	Roles          []organization.Role `json:"roles"`
	ProjectIDs     []string            `json:"projects"`
	jwt.RegisteredClaims
}

func (s Service) Login(ctx context.Context, email, password string) (TokenPair, error) {
	account, err := s.Accounts.FindByEmail(ctx, email)
	if err != nil {
		return TokenPair{}, shared.New(shared.CodeUnauthenticated, "email or password is invalid")
	}
	if !account.Active || bcrypt.CompareHashAndPassword(account.PasswordHash, []byte(password)) != nil {
		return TokenPair{}, shared.New(shared.CodeUnauthenticated, "email or password is invalid")
	}
	familyID, err := s.IDs.New("family")
	if err != nil {
		return TokenPair{}, err
	}
	return s.issue(ctx, account, familyID)
}

func (s Service) Refresh(ctx context.Context, rawRefreshToken string) (TokenPair, error) {
	return s.refreshReusable(ctx, rawRefreshToken)
}

func (s Service) issue(ctx context.Context, account Account, familyID string) (TokenPair, error) {
	refreshID, err := s.IDs.New("refresh")
	if err != nil {
		return TokenPair{}, err
	}
	rawRefresh, err := randomToken()
	if err != nil {
		return TokenPair{}, err
	}
	session, err := domain.NewRefreshSession(refreshID, account.OrganizationID, account.EmployeeID, familyID, rawRefresh, s.Clock.Now(), s.RefreshTTL)
	if err != nil {
		return TokenPair{}, err
	}
	access, accessExpiry, err := s.accessToken(account)
	if err != nil {
		return TokenPair{}, err
	}
	if err := s.Sessions.Insert(ctx, *session); err != nil {
		return TokenPair{}, err
	}
	return TokenPair{AccessToken: access, RefreshToken: rawRefresh, AccessExpiresAt: accessExpiry, RefreshExpiresAt: session.ExpiresAt}, nil
}

func (s Service) accessToken(account Account) (string, time.Time, error) {
	now := s.Clock.Now()
	expires := now.Add(s.AccessTTL)
	projects := make([]string, 0, len(account.ProjectIDs))
	for projectID, allowed := range account.ProjectIDs {
		if allowed {
			projects = append(projects, projectID)
		}
	}
	claims := accessClaims{OrganizationID: account.OrganizationID, EmployeeID: account.EmployeeID, Roles: append([]organization.Role(nil), account.Roles...), ProjectIDs: projects, RegisteredClaims: jwt.RegisteredClaims{Subject: account.EmployeeID, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(expires), Issuer: "cry-088"}}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.Secret)
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, expires, nil
}

func (s Service) ParseAccess(raw string) (organization.Principal, error) {
	claims := &accessClaims{}
	token, err := jwt.ParseWithClaims(raw, claims, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, shared.New(shared.CodeUnauthenticated, "access token signing method is invalid")
		}
		return s.Secret, nil
	}, jwt.WithIssuer("cry-088"), jwt.WithExpirationRequired())
	if err != nil || !token.Valid {
		return organization.Principal{}, shared.New(shared.CodeUnauthenticated, "access token is invalid")
	}
	projects := make(map[string]bool, len(claims.ProjectIDs))
	for _, id := range claims.ProjectIDs {
		projects[id] = true
	}
	return organization.Principal{EmployeeID: claims.EmployeeID, OrganizationID: claims.OrganizationID, Roles: append([]organization.Role(nil), claims.Roles...), ProjectIDs: projects}, nil
}

func (s Service) accountForSession(ctx context.Context, session domain.RefreshSession) (Account, error) {
	account, err := s.Accounts.FindByEmail(ctx, session.EmployeeID+"@internal.invalid")
	if err != nil {
		return Account{}, shared.Wrap(shared.CodeUnauthenticated, "refresh account is unavailable", err)
	}
	if !account.Active || account.EmployeeID != session.EmployeeID || account.OrganizationID != session.OrganizationID {
		return Account{}, shared.New(shared.CodeUnauthenticated, "refresh account is inactive")
	}
	return account, nil
}

func randomToken() (string, error) {
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}
