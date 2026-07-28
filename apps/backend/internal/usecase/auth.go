package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/observability"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/port"
	"github.com/google/uuid"
)

type Auth struct {
	Repo       port.Repository
	Passwords  port.PasswordHasher
	Tokens     port.TokenManager
	Sessions   port.SessionIndex
	Limiter    port.RateLimiter
	RateLimit  int
	RateWindow time.Duration
}

type Authentication struct {
	Principal domain.Principal
	TokenHash []byte
}

func (a Auth) Login(ctx context.Context, input domain.LoginInput) (domain.LoginResult, error) {
	defer observability.StartSegment(ctx, "Usecase.Auth.Login")()
	input.Username = domain.NormalizeUsername(input.Username)
	if input.Username == "" || input.Password == "" {
		return domain.LoginResult{}, domain.Validation("Username dan kata sandi wajib diisi", nil)
	}
	key := input.IPAddress + ":" + input.Username
	if allowed, err := a.Limiter.Allow(ctx, key, a.RateLimit, a.RateWindow); err == nil && !allowed {
		return domain.LoginResult{}, domain.NewError(domain.CodeRateLimited, "Terlalu banyak percobaan masuk. Coba lagi nanti.")
	}

	user, err := a.Repo.UserForLogin(ctx, input.Username)
	if err != nil {
		return domain.LoginResult{}, domain.NewError(domain.CodeInvalidCredentials, "Username atau kata sandi salah")
	}
	ok, verifyErr := a.Passwords.Verify(input.Password, user.PasswordHash)
	if verifyErr != nil || !ok || !user.IsActive || user.DeletedAt != nil {
		return domain.LoginResult{}, domain.NewError(domain.CodeInvalidCredentials, "Username atau kata sandi salah")
	}

	raw, tokenHash, err := a.Tokens.New()
	if err != nil {
		return domain.LoginResult{}, domain.WrapInternal(err, "issue session token")
	}
	var terminalID *uuid.UUID
	if input.InstallationID != nil {
		terminalID, err = a.Repo.TerminalIDByInstallation(ctx, *input.InstallationID)
		if err != nil {
			return domain.LoginResult{}, err
		}
	}
	principal, err := a.Repo.CreateSession(ctx, user.ID, terminalID, tokenHash)
	if err != nil {
		return domain.LoginResult{}, err
	}
	a.Sessions.Set(ctx, tokenHash, principal.SessionID)
	return domain.LoginResult{Token: raw, Principal: principal}, nil
}

func (a Auth) Authenticate(ctx context.Context, rawToken string) (Authentication, error) {
	defer observability.StartSegment(ctx, "Usecase.Auth.Authenticate")()
	tokenHash, err := a.Tokens.Hash(strings.TrimSpace(rawToken))
	if err != nil {
		return Authentication{}, domain.NewError(domain.CodeUnauthorized, "Sesi tidak valid")
	}
	if sessionID, ok := a.Sessions.Get(ctx, tokenHash); ok {
		principal, repoErr := a.Repo.PrincipalBySession(ctx, sessionID, tokenHash)
		if repoErr == nil {
			return Authentication{Principal: principal, TokenHash: tokenHash}, nil
		}
		a.Sessions.Delete(ctx, tokenHash)
	}
	principal, err := a.Repo.PrincipalByTokenHash(ctx, tokenHash)
	if err != nil {
		return Authentication{}, domain.NewError(domain.CodeUnauthorized, "Sesi tidak valid atau telah dicabut")
	}
	a.Sessions.Set(ctx, tokenHash, principal.SessionID)
	return Authentication{Principal: principal, TokenHash: tokenHash}, nil
}

func (a Auth) Logout(ctx context.Context, principal domain.Principal) error {
	defer observability.StartSegment(ctx, "Usecase.Auth.Logout")()
	return a.Repo.RevokeSession(ctx, principal.SessionID, principal.UserID, "logout")
}

func (a Auth) ChangePassword(ctx context.Context, principal domain.Principal, current, next string) error {
	defer observability.StartSegment(ctx, "Usecase.Auth.ChangePassword")()
	if err := domain.ValidatePassword(next); err != nil {
		return err
	}
	if current == next {
		return domain.Validation("Kata sandi baru harus berbeda", map[string]any{"field": "newPassword"})
	}
	user, err := a.Repo.UserForLogin(ctx, principal.Username)
	if err != nil {
		return domain.NewError(domain.CodeUnauthorized, "Pengguna tidak ditemukan")
	}
	ok, err := a.Passwords.Verify(current, user.PasswordHash)
	if err != nil || !ok {
		return domain.Validation("Kata sandi saat ini salah", map[string]any{"field": "currentPassword"})
	}
	hash, err := a.Passwords.Hash(next)
	if err != nil {
		return domain.WrapInternal(err, "hash changed password")
	}
	if err := a.Repo.ChangeOwnPassword(ctx, principal, hash); err != nil {
		return err
	}
	return nil
}

func RequireReady(principal domain.Principal) error {
	if principal.MustChangePassword {
		return domain.NewError(domain.CodePasswordChange, "Ganti kata sandi sementara sebelum melanjutkan")
	}
	return nil
}

func RequireSuperadmin(principal domain.Principal) error {
	if err := RequireReady(principal); err != nil {
		return err
	}
	if !principal.IsSuperadmin() {
		return domain.NewError(domain.CodeForbidden, "Tindakan ini hanya tersedia untuk superadmin")
	}
	return nil
}

func RequireTerminal(principal domain.Principal) error {
	if err := RequireReady(principal); err != nil {
		return err
	}
	if principal.TerminalID == nil {
		return domain.NewError(domain.CodeForbidden, "Daftarkan terminal ini sebelum membuat perubahan lokal")
	}
	return nil
}

func BearerToken(header string) (string, error) {
	scheme, value, ok := strings.Cut(strings.TrimSpace(header), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("missing bearer token")
	}
	return strings.TrimSpace(value), nil
}
