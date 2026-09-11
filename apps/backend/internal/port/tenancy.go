package port

import (
	"context"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
	"github.com/google/uuid"
)

// TenancyRepository deliberately separates global account/control operations
// from the data-plane Repository. Platform sessions never acquire data scope.
type TenancyRepository interface {
	ListPlatformAudit(context.Context, domain.Principal, int) ([]domain.PlatformAuditEvent, error)
	UpdateOwnProfile(context.Context, domain.Principal, string) (domain.User, error)
	RevalidateOrigins(context.Context, domain.Principal, []domain.OutboxOrigin) ([]domain.OutboxOriginValidation, error)
	CreateAccountSession(context.Context, uuid.UUID, []byte) (domain.Principal, error)
	SwitchContextSession(context.Context, domain.Principal, []byte, []byte, domain.SwitchContextInput) (domain.Principal, error)
	AvailableContexts(context.Context, uuid.UUID) (domain.AvailableContexts, error)
	AccountUser(context.Context, uuid.UUID) (domain.User, error)
	TenantMembers(context.Context, domain.Principal) ([]domain.TenantMember, error)
	UpdateTenantMember(context.Context, domain.Principal, uuid.UUID, domain.UpdateMembershipInput) (domain.TenantMember, error)
	ListTenants(context.Context, domain.Principal) ([]domain.Tenant, error)
	CreateTenant(context.Context, domain.Principal, domain.CreateTenantInput, []byte) (domain.CreateTenantResult, error)
	SetTenantStatus(context.Context, domain.Principal, uuid.UUID, string) (domain.Tenant, error)
	CreateTenantInvitation(context.Context, domain.Principal, uuid.UUID, domain.Role, bool, []byte) (domain.Invitation, error)
	ListTenantInvitations(context.Context, domain.Principal) ([]domain.Invitation, error)
	RevokeTenantInvitation(context.Context, domain.Principal, uuid.UUID) error
	AcceptTenantInvitation(context.Context, domain.Principal, []byte) (domain.TenantContext, error)
	RegisterTenantInvitation(context.Context, domain.RegisterInvitationInput, string, []byte) (uuid.UUID, error)
	GetTenantProfile(context.Context, uuid.UUID) (domain.TenantProfile, error)
	UpdateTenantProfile(context.Context, domain.Principal, domain.UpdateTenantProfileInput) (domain.TenantProfile, error)
	GetTenantQRIS(context.Context, uuid.UUID) (domain.TenantQRISConfig, error)
	UpdateTenantQRIS(context.Context, domain.Principal, domain.UpdateTenantQRISInput, string) (domain.TenantQRISConfig, error)
}
