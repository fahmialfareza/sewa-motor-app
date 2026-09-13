package port

import (
	"context"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
	"github.com/google/uuid"
)

// Global staff management is deliberately separate from legacy tenant-member
// methods so an old request cannot acquire a new, organization-wide meaning.
type ManagementRepository interface {
	ListManagedUsers(context.Context, domain.Principal) ([]domain.User, error)
	GetManagedUser(context.Context, domain.Principal, uuid.UUID) (domain.User, error)
	CreateManagedUser(context.Context, domain.Principal, domain.CreateUserInput, string) (domain.User, error)
	UpdateManagedUser(context.Context, domain.Principal, uuid.UUID, domain.UpdateManagedUserInput) (domain.User, error)
	ResetManagedPassword(context.Context, domain.Principal, uuid.UUID, string) (domain.User, error)
	UpdateManagedTenant(context.Context, domain.Principal, uuid.UUID, domain.UpdateTenantInput) (domain.Tenant, error)
}
