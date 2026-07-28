package usecase

import (
	"context"
	"crypto/ed25519"
	"strings"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/observability"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/port"
	"github.com/google/uuid"
)

type Terminals struct {
	Repo port.Repository
}

func (t Terminals) Enroll(ctx context.Context, principal domain.Principal, input domain.EnrollTerminalInput) (domain.Terminal, error) {
	defer observability.StartSegment(ctx, "Usecase.Terminals.Enroll")()
	if err := RequireReady(principal); err != nil {
		return domain.Terminal{}, err
	}
	input.InstallationID = strings.TrimSpace(input.InstallationID)
	input.Name = strings.TrimSpace(input.Name)
	if len(input.InstallationID) < 8 || len(input.InstallationID) > 200 || input.Name == "" || len(input.Name) > 120 || len(input.PublicKey) != ed25519.PublicKeySize {
		return domain.Terminal{}, domain.Validation("Data terminal atau kunci Ed25519 tidak valid", nil)
	}
	return t.Repo.EnrollTerminal(ctx, principal, input)
}

func (t Terminals) Current(ctx context.Context, principal domain.Principal) (domain.Terminal, error) {
	defer observability.StartSegment(ctx, "Usecase.Terminals.Current")()
	if err := RequireReady(principal); err != nil {
		return domain.Terminal{}, err
	}
	if principal.TerminalID == nil {
		return domain.Terminal{}, domain.NewError(domain.CodeNotFound, "Sesi belum terikat ke terminal")
	}
	return t.Repo.GetTerminal(ctx, *principal.TerminalID)
}

func (t Terminals) Revoke(ctx context.Context, principal domain.Principal, id uuid.UUID) (domain.Terminal, error) {
	defer observability.StartSegment(ctx, "Usecase.Terminals.Revoke")()
	if err := RequireSuperadmin(principal); err != nil {
		return domain.Terminal{}, err
	}
	return t.Repo.RevokeTerminal(ctx, principal, id)
}
