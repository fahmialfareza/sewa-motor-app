package usecase

import (
	"context"
	"strings"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/observability"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/port"
	"github.com/google/uuid"
)

type Packages struct {
	Repo port.Repository
}

func (p Packages) List(ctx context.Context, principal domain.Principal, includeDeleted bool) ([]domain.Package, error) {
	defer observability.StartSegment(ctx, "Usecase.Packages.List")()
	if err := RequireReady(principal); err != nil {
		return nil, err
	}
	if includeDeleted && !principal.IsSuperadmin() {
		includeDeleted = false
	}
	return p.Repo.ListPackages(ctx, includeDeleted)
}

func (p Packages) Get(ctx context.Context, principal domain.Principal, id uuid.UUID) (domain.Package, error) {
	defer observability.StartSegment(ctx, "Usecase.Packages.Get")()
	if err := RequireReady(principal); err != nil {
		return domain.Package{}, err
	}
	return p.Repo.GetPackage(ctx, id)
}

func (p Packages) Create(ctx context.Context, principal domain.Principal, input domain.CreatePackageInput) (domain.Package, error) {
	defer observability.StartSegment(ctx, "Usecase.Packages.Create")()
	if err := RequireSuperadmin(principal); err != nil {
		return domain.Package{}, err
	}
	input.Code = domain.NormalizePackageCode(input.Code)
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.ChangeReason = strings.TrimSpace(input.ChangeReason)
	if len(input.Code) < 2 || len(input.Name) < 1 || len(input.Name) > 120 || len(input.Description) > 1000 || input.UnitPrice <= 0 {
		return domain.Package{}, domain.Validation("Data paket tidak valid", nil)
	}
	return p.Repo.CreatePackage(ctx, principal, input)
}

func (p Packages) Update(ctx context.Context, principal domain.Principal, id uuid.UUID, input domain.UpdatePackageInput) (domain.Package, error) {
	defer observability.StartSegment(ctx, "Usecase.Packages.Update")()
	if err := RequireSuperadmin(principal); err != nil {
		return domain.Package{}, err
	}
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.ChangeReason = strings.TrimSpace(input.ChangeReason)
	if id == uuid.Nil || input.Name == "" || input.UnitPrice <= 0 || len(input.ChangeReason) < 3 {
		return domain.Package{}, domain.Validation("Nama, harga, dan alasan perubahan paket wajib diisi", nil)
	}
	return p.Repo.UpdatePackage(ctx, principal, id, input)
}

func (p Packages) Delete(ctx context.Context, principal domain.Principal, id uuid.UUID, reason string) error {
	defer observability.StartSegment(ctx, "Usecase.Packages.Delete")()
	if err := RequireSuperadmin(principal); err != nil {
		return err
	}
	reason = strings.TrimSpace(reason)
	if id == uuid.Nil || len(reason) < 3 {
		return domain.Validation("ID dan alasan penghapusan paket wajib diisi", nil)
	}
	return p.Repo.DeletePackage(ctx, principal, id, reason)
}
