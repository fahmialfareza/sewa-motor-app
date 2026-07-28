package usecase

import (
	"context"
	"testing"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/port"
	"github.com/google/uuid"
)

type guardedUserRepository struct {
	port.Repository
	updateCalled bool
	updateErr    error
}

func (r *guardedUserRepository) UpdateUser(context.Context, domain.Principal, uuid.UUID, domain.UpdateUserInput) (domain.User, error) {
	r.updateCalled = true
	return domain.User{}, r.updateErr
}

func TestUsersRejectSelfDemotionBeforeRepository(t *testing.T) {
	repo := &guardedUserRepository{}
	service := Users{Repo: repo}
	id := uuid.New()
	adminRole := domain.RoleAdmin
	_, err := service.Update(context.Background(), domain.Principal{
		UserID: id, Role: domain.RoleSuperadmin,
	}, id, domain.UpdateUserInput{Role: &adminRole})
	if !domain.IsCode(err, domain.CodeSelfMutation) {
		t.Fatalf("expected self-protection error, got %v", err)
	}
	if repo.updateCalled {
		t.Fatal("repository was called for rejected self-demotion")
	}
}

func TestUsersPropagateFinalSuperadminGuard(t *testing.T) {
	repo := &guardedUserRepository{
		updateErr: domain.NewError(domain.CodeFinalSuperadmin, "final"),
	}
	service := Users{Repo: repo}
	adminRole := domain.RoleAdmin
	_, err := service.Update(context.Background(), domain.Principal{
		UserID: uuid.New(), Role: domain.RoleSuperadmin,
	}, uuid.New(), domain.UpdateUserInput{Role: &adminRole})
	if !domain.IsCode(err, domain.CodeFinalSuperadmin) {
		t.Fatalf("expected final-superadmin error, got %v", err)
	}
	if !repo.updateCalled {
		t.Fatal("repository guard was not reached")
	}
}

func TestRequireSuperadminEnforcesRoleAndForcedPasswordChange(t *testing.T) {
	if err := RequireSuperadmin(domain.Principal{Role: domain.RoleAdmin}); !domain.IsCode(err, domain.CodeForbidden) {
		t.Fatalf("expected forbidden for admin, got %v", err)
	}
	if err := RequireSuperadmin(domain.Principal{
		Role: domain.RoleSuperadmin, MustChangePassword: true,
	}); !domain.IsCode(err, domain.CodePasswordChange) {
		t.Fatalf("expected forced-password error, got %v", err)
	}
}
