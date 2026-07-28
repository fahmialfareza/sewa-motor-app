package usecase

import (
	"context"
	"strings"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/observability"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/port"
	"github.com/google/uuid"
)

type Users struct {
	Repo      port.Repository
	Passwords port.PasswordHasher
}

func (u Users) List(ctx context.Context, principal domain.Principal, includeDeleted bool) ([]domain.User, error) {
	defer observability.StartSegment(ctx, "Usecase.Users.List")()
	if err := RequireSuperadmin(principal); err != nil {
		return nil, err
	}
	return u.Repo.ListUsers(ctx, includeDeleted)
}

func (u Users) Create(ctx context.Context, principal domain.Principal, input domain.CreateUserInput) (domain.User, error) {
	defer observability.StartSegment(ctx, "Usecase.Users.Create")()
	if err := RequireSuperadmin(principal); err != nil {
		return domain.User{}, err
	}
	input.FullName = strings.TrimSpace(input.FullName)
	input.Username = domain.NormalizeUsername(input.Username)
	if len(input.FullName) < 1 || len(input.FullName) > 160 || len(input.Username) < 3 || !input.Role.Valid() {
		return domain.User{}, domain.Validation("Data pengguna tidak valid", nil)
	}
	if err := domain.ValidatePassword(input.TemporaryPassword); err != nil {
		return domain.User{}, err
	}
	hash, err := u.Passwords.Hash(input.TemporaryPassword)
	if err != nil {
		return domain.User{}, domain.WrapInternal(err, "hash temporary password")
	}
	return u.Repo.CreateUser(ctx, principal, input, hash)
}

func (u Users) Update(ctx context.Context, principal domain.Principal, targetID uuid.UUID, input domain.UpdateUserInput) (domain.User, error) {
	defer observability.StartSegment(ctx, "Usecase.Users.Update")()
	if err := RequireSuperadmin(principal); err != nil {
		return domain.User{}, err
	}
	if targetID == uuid.Nil {
		return domain.User{}, domain.Validation("ID pengguna tidak valid", nil)
	}
	if targetID == principal.UserID {
		if input.Role != nil && *input.Role != domain.RoleSuperadmin {
			return domain.User{}, domain.NewError(domain.CodeSelfMutation, "Anda tidak dapat menurunkan peran sendiri")
		}
		if input.IsActive != nil && !*input.IsActive {
			return domain.User{}, domain.NewError(domain.CodeSelfMutation, "Anda tidak dapat menonaktifkan akun sendiri")
		}
	}
	if input.Role != nil && !input.Role.Valid() {
		return domain.User{}, domain.Validation("Peran pengguna tidak valid", map[string]any{"field": "role"})
	}
	if input.FullName != nil {
		name := strings.TrimSpace(*input.FullName)
		if name == "" || len(name) > 160 {
			return domain.User{}, domain.Validation("Nama lengkap tidak valid", map[string]any{"field": "fullName"})
		}
		input.FullName = &name
	}
	if input.Username != nil {
		username := domain.NormalizeUsername(*input.Username)
		if len(username) < 3 || len(username) > 64 {
			return domain.User{}, domain.Validation("Username tidak valid", map[string]any{"field": "username"})
		}
		input.Username = &username
	}
	return u.Repo.UpdateUser(ctx, principal, targetID, input)
}

func (u Users) Get(ctx context.Context, principal domain.Principal, targetID uuid.UUID) (domain.User, error) {
	defer observability.StartSegment(ctx, "Usecase.Users.Get")()
	if err := RequireSuperadmin(principal); err != nil {
		return domain.User{}, err
	}
	return u.Repo.GetUser(ctx, targetID)
}

func (u Users) ResetPassword(ctx context.Context, principal domain.Principal, targetID uuid.UUID, password string) (domain.User, error) {
	defer observability.StartSegment(ctx, "Usecase.Users.ResetPassword")()
	if err := RequireSuperadmin(principal); err != nil {
		return domain.User{}, err
	}
	if err := domain.ValidatePassword(password); err != nil {
		return domain.User{}, err
	}
	hash, err := u.Passwords.Hash(password)
	if err != nil {
		return domain.User{}, domain.WrapInternal(err, "hash reset password")
	}
	return u.Repo.ResetUserPassword(ctx, principal, targetID, hash)
}

func (u Users) Delete(ctx context.Context, principal domain.Principal, targetID uuid.UUID, reason string) error {
	defer observability.StartSegment(ctx, "Usecase.Users.Delete")()
	if err := RequireSuperadmin(principal); err != nil {
		return err
	}
	if targetID == principal.UserID {
		return domain.NewError(domain.CodeSelfMutation, "Anda tidak dapat menghapus akun sendiri")
	}
	reason = strings.TrimSpace(reason)
	if len(reason) < 3 {
		return domain.Validation("Alasan penghapusan wajib diisi", map[string]any{"field": "reason"})
	}
	return u.Repo.DeleteUser(ctx, principal, targetID, reason)
}
