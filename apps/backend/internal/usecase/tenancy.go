package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/observability"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/port"
	"github.com/google/uuid"
)

type Tenancy struct {
	Repo                port.TenancyRepository
	Passwords           port.PasswordHasher
	ProvisioningEnabled bool
}

func RequirePlatform(p domain.Principal) error {
	return RequireManagement(p)
}

func (t Tenancy) Contexts(ctx context.Context, p domain.Principal) (domain.AvailableContexts, error) {
	defer observability.StartSegment(ctx, "Usecase.Tenancy.Contexts")()
	result, err := t.Repo.AvailableContexts(ctx, p.UserID)
	result.TenantProvisioningEnabled = t.ProvisioningEnabled
	return result, err
}

func (t Tenancy) Create(ctx context.Context, p domain.Principal, input domain.CreateTenantInput) (domain.CreateTenantResult, error) {
	defer observability.StartSegment(ctx, "Usecase.Tenancy.Create")()
	if err := RequirePlatform(p); err != nil {
		return domain.CreateTenantResult{}, err
	}
	if !t.ProvisioningEnabled {
		return domain.CreateTenantResult{}, domain.NewError(domain.CodeForbidden, "Pendaftaran bisnis baru belum diaktifkan")
	}
	input.Name = strings.TrimSpace(input.Name)
	input.Slug = strings.ToLower(strings.TrimSpace(input.Slug))
	if input.Name == "" || len(input.Name) > 160 || !regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,62}$`).MatchString(input.Slug) {
		return domain.CreateTenantResult{}, domain.Validation("Nama dan kode bisnis tidak valid", nil)
	}
	return t.Repo.CreateTenant(ctx, p, input, nil)
}

func (t Tenancy) Invite(ctx context.Context, p domain.Principal, tenantID uuid.UUID, role domain.Role, owner bool) (domain.Invitation, error) {
	defer observability.StartSegment(ctx, "Usecase.Tenancy.Invite")()
	return domain.Invitation{}, domain.NewError("INVITATIONS_REMOVED", "Undangan tidak lagi digunakan. Semua akun aktif dapat memilih setiap bisnis.")
}

func (t Tenancy) Accept(ctx context.Context, p domain.Principal, code string) (domain.TenantContext, error) {
	defer observability.StartSegment(ctx, "Usecase.Tenancy.Accept")()
	return domain.TenantContext{}, domain.NewError("INVITATIONS_REMOVED", "Undangan tidak lagi digunakan. Semua akun aktif dapat memilih setiap bisnis.")
}

func (t Tenancy) Register(ctx context.Context, input domain.RegisterInvitationInput) (uuid.UUID, error) {
	defer observability.StartSegment(ctx, "Usecase.Tenancy.Register")()
	return uuid.Nil, domain.NewError("INVITATIONS_REMOVED", "Pendaftaran melalui undangan telah dihapus. Hubungi Superadmin untuk membuat akun.")
}

func (t Tenancy) UpdateProfile(ctx context.Context, p domain.Principal, input domain.UpdateTenantProfileInput) (domain.TenantProfile, error) {
	defer observability.StartSegment(ctx, "Usecase.Tenancy.UpdateProfile")()
	if err := RequireSuperadmin(p); err != nil {
		return domain.TenantProfile{}, err
	}
	if err := RequireProduction(p); err != nil {
		return domain.TenantProfile{}, err
	}
	input.BusinessName = strings.TrimSpace(input.BusinessName)
	input.Address = strings.TrimSpace(input.Address)
	input.Phone = strings.TrimSpace(input.Phone)
	if input.BusinessName == "" || len(input.BusinessName) > 160 || len(input.Address) > 500 || len(input.Phone) > 50 || input.ExpectedRevision < 1 {
		return domain.TenantProfile{}, domain.Validation("Profil bisnis tidak valid", nil)
	}
	return t.Repo.UpdateTenantProfile(ctx, p, input)
}

func (t Tenancy) UpdateQRIS(ctx context.Context, p domain.Principal, input domain.UpdateTenantQRISInput) (domain.TenantQRISConfig, error) {
	defer observability.StartSegment(ctx, "Usecase.Tenancy.UpdateQRIS")()
	if err := RequireSuperadmin(p); err != nil {
		return domain.TenantQRISConfig{}, err
	}
	if err := RequireProduction(p); err != nil {
		return domain.TenantQRISConfig{}, err
	}
	input.StaticPayload = strings.TrimSpace(input.StaticPayload)
	if err := domain.ValidateStaticQRIS(input.StaticPayload); err != nil {
		return domain.TenantQRISConfig{}, err
	}
	if input.ExpectedRevision < 0 {
		return domain.TenantQRISConfig{}, domain.Validation("Revisi QRIS tidak valid", nil)
	}
	sum := sha256.Sum256([]byte(input.StaticPayload))
	return t.Repo.UpdateTenantQRIS(ctx, p, input, hex.EncodeToString(sum[:]))
}
