package usecase

import (
	"context"
	"strings"
	"time"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/observability"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/port"
)

const sandboxResetConfirmation = "RESET SANDBOX"

// Sandbox coordinates the control-plane lifecycle for the one shared Sandbox
// generation. Business mutations remain scoped by the immutable session-bound
// data space in their respective use cases and repositories.
type Sandbox struct {
	Repo          port.Repository
	Clock         port.Clock
	Enabled       bool
	QRISAmount    int64
	RetentionDays int
}

func (s Sandbox) Status(ctx context.Context, principal domain.Principal) (domain.SandboxStatus, error) {
	defer observability.StartSegment(ctx, "Usecase.Sandbox.Status")()
	if err := RequireReady(principal); err != nil {
		return domain.SandboxStatus{}, err
	}
	status := domain.SandboxStatus{
		Enabled:       s.Enabled,
		DataMode:      domain.DataModeSandbox,
		RetentionDays: s.retentionDays(),
		QrisAmount:    s.qrisAmount(),
	}
	space, err := s.Repo.ActiveDataSpace(ctx, domain.DataModeSandbox)
	if err != nil {
		// A disabled feature may legitimately never have been activated. Once a
		// generation exists, keep returning its identity so clients can detect a
		// rollback without pretending the retained Sandbox data disappeared.
		if !s.Enabled && domain.IsCode(err, domain.CodeNotFound) {
			return status, nil
		}
		return domain.SandboxStatus{}, err
	}
	status.DataSpaceID = &space.ID
	status.Generation = &space.Generation
	return status, nil
}

// Initialize creates the first shared Sandbox generation only after the
// deployment explicitly enables the feature. The repository operation is
// advisory-locked and idempotent across concurrent API replicas.
func (s Sandbox) Initialize(ctx context.Context) (domain.DataSpace, error) {
	defer observability.StartSegment(ctx, "Usecase.Sandbox.Initialize")()
	if !s.Enabled {
		return domain.DataSpace{}, nil
	}
	return s.Repo.EnsureSandbox(ctx)
}

func (s Sandbox) Reset(
	ctx context.Context,
	principal domain.Principal,
	input domain.ResetSandboxInput,
) (domain.SandboxResetResult, error) {
	defer observability.StartSegment(ctx, "Usecase.Sandbox.Reset")()
	if err := RequireProduction(principal); err != nil {
		return domain.SandboxResetResult{}, err
	}
	if err := RequireSuperadmin(principal); err != nil {
		return domain.SandboxResetResult{}, err
	}
	if !s.Enabled {
		return domain.SandboxResetResult{}, domain.NewError(
			domain.CodeForbidden,
			"Mode Sandbox sedang dinonaktifkan",
		)
	}
	if input.ExpectedGeneration < 1 {
		return domain.SandboxResetResult{}, domain.Validation(
			"Generasi Sandbox tidak valid",
			map[string]any{"field": "expectedGeneration"},
		)
	}
	if strings.TrimSpace(input.Confirmation) != sandboxResetConfirmation {
		return domain.SandboxResetResult{}, domain.Validation(
			"Ketik RESET SANDBOX untuk mengonfirmasi reset",
			map[string]any{"field": "confirmation"},
		)
	}
	return s.Repo.ResetSandbox(
		ctx,
		principal,
		input.ExpectedGeneration,
		time.Duration(s.retentionDays())*24*time.Hour,
	)
}

func (s Sandbox) Cleanup(ctx context.Context) (domain.SandboxCleanupResult, error) {
	defer observability.StartSegment(ctx, "Usecase.Sandbox.Cleanup")()
	return s.Repo.CleanupExpiredSandboxes(ctx, s.now())
}

func (s Sandbox) now() time.Time {
	if s.Clock == nil {
		return time.Now().UTC()
	}
	return s.Clock.Now()
}

func (s Sandbox) retentionDays() int {
	if s.RetentionDays < 1 {
		return 30
	}
	return s.RetentionDays
}

func (s Sandbox) qrisAmount() int64 {
	// The amount is a deliberate safety invariant, not a freely configurable
	// price. Config validation rejects every other value; this fallback keeps
	// directly constructed services safe as well.
	if s.QRISAmount != domain.SandboxQRISPaymentAmount {
		return domain.SandboxQRISPaymentAmount
	}
	return s.QRISAmount
}
