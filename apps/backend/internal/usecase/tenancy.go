package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
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
	if err := RequireReady(p); err != nil {
		return err
	}
	if p.ContextKind != domain.ContextPlatform || !p.IsPlatformAdmin {
		return domain.NewError(domain.CodeForbidden, "Akses pengelola platform diperlukan")
	}
	return nil
}

func (t Tenancy) Contexts(ctx context.Context, p domain.Principal) (domain.AvailableContexts, error) {
	defer observability.StartSegment(ctx, "Usecase.Tenancy.Contexts")()
	return t.Repo.AvailableContexts(ctx, p.UserID)
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
	code, hash, err := newInvitationCode()
	if err != nil {
		return domain.CreateTenantResult{}, err
	}
	result, err := t.Repo.CreateTenant(ctx, p, input, hash)
	if err == nil {
		result.Invitation.Code = code
	}
	return result, err
}

func (t Tenancy) Invite(ctx context.Context, p domain.Principal, tenantID uuid.UUID, role domain.Role, owner bool) (domain.Invitation, error) {
	defer observability.StartSegment(ctx, "Usecase.Tenancy.Invite")()
	if owner {
		if err := RequirePlatform(p); err != nil {
			return domain.Invitation{}, err
		}
		role = domain.RoleSuperadmin
	} else {
		if err := RequireProduction(p); err != nil {
			return domain.Invitation{}, err
		}
		if err := RequireSuperadmin(p); err != nil {
			return domain.Invitation{}, err
		}
		tenantID = p.TenantID
	}
	if !role.Valid() {
		return domain.Invitation{}, domain.Validation("Peran undangan tidak valid", nil)
	}
	code, hash, err := newInvitationCode()
	if err != nil {
		return domain.Invitation{}, err
	}
	result, err := t.Repo.CreateTenantInvitation(ctx, p, tenantID, role, owner, hash)
	if err == nil {
		result.Code = code
	}
	return result, err
}

func newInvitationCode() (string, []byte, error) {
	var value [24]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", nil, domain.WrapInternal(err, "generate invitation")
	}
	code := base64.RawURLEncoding.EncodeToString(value[:])
	return code, invitationHash(code), nil
}
func invitationHash(code string) []byte {
	sum := sha256.Sum256([]byte(strings.TrimSpace(code)))
	return sum[:]
}

func (t Tenancy) Accept(ctx context.Context, p domain.Principal, code string) (domain.TenantContext, error) {
	defer observability.StartSegment(ctx, "Usecase.Tenancy.Accept")()
	if err := RequireReady(p); err != nil {
		return domain.TenantContext{}, err
	}
	if len(strings.TrimSpace(code)) != 32 {
		return domain.TenantContext{}, domain.NewError(domain.CodeInvitationInvalid, "Kode undangan tidak valid")
	}
	if p.IsTenantContext() && p.DataMode == domain.DataModeSandbox {
		return domain.TenantContext{}, domain.NewError(domain.CodeForbidden, "Beralih ke produksi atau akun sebelum menerima undangan")
	}
	return t.Repo.AcceptTenantInvitation(ctx, p, invitationHash(code))
}

func (t Tenancy) Register(ctx context.Context, input domain.RegisterInvitationInput) (uuid.UUID, error) {
	defer observability.StartSegment(ctx, "Usecase.Tenancy.Register")()
	input.Username = domain.NormalizeUsername(input.Username)
	input.FullName = strings.TrimSpace(input.FullName)
	input.Code = strings.TrimSpace(input.Code)
	if !regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{2,63}$`).MatchString(input.Username) || input.FullName == "" || len(input.FullName) > 160 || len(input.Code) != 32 {
		return uuid.Nil, domain.Validation("Data pendaftaran atau undangan tidak valid", nil)
	}
	if err := domain.ValidatePassword(input.Password); err != nil {
		return uuid.Nil, err
	}
	hash, err := t.Passwords.Hash(input.Password)
	if err != nil {
		return uuid.Nil, domain.WrapInternal(err, "hash invited password")
	}
	return t.Repo.RegisterTenantInvitation(ctx, input, hash, invitationHash(input.Code))
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
