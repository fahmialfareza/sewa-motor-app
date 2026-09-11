package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/port"
	"github.com/google/uuid"
)

type sandboxRepository struct {
	port.Repository
	active          domain.DataSpace
	resetCalled     bool
	resetGeneration int64
	resetRetention  time.Duration
	cleanupAt       time.Time
	activeCalled    bool
	ensureCalled    bool
	activeErr       error
}

func (repository *sandboxRepository) ActiveDataSpace(
	context.Context,
	uuid.UUID,
	domain.DataMode,
) (domain.DataSpace, error) {
	repository.activeCalled = true
	return repository.active, repository.activeErr
}

func (repository *sandboxRepository) EnsureSandbox(context.Context, uuid.UUID) (domain.DataSpace, error) {
	repository.ensureCalled = true
	return repository.active, nil
}

func (repository *sandboxRepository) ResetSandbox(
	_ context.Context,
	_ domain.Principal,
	expectedGeneration int64,
	retention time.Duration,
) (domain.SandboxResetResult, error) {
	repository.resetCalled = true
	repository.resetGeneration = expectedGeneration
	repository.resetRetention = retention
	return domain.SandboxResetResult{Previous: repository.active}, nil
}

func (repository *sandboxRepository) CleanupExpiredSandboxes(
	_ context.Context,
	now time.Time,
) (domain.SandboxCleanupResult, error) {
	repository.cleanupAt = now
	return domain.SandboxCleanupResult{PurgedGenerationCount: 1}, nil
}

type fixedClock struct{ value time.Time }

func (clock fixedClock) Now() time.Time { return clock.value }

func TestSandboxStatusAndResetEnforceLifecycleContract(t *testing.T) {
	t.Parallel()

	spaceID := uuid.New()
	repository := &sandboxRepository{active: domain.DataSpace{
		ID: spaceID, Mode: domain.DataModeSandbox, Generation: 4,
		Status: domain.DataSpaceStatusActive,
	}}
	service := Sandbox{
		Repo: repository, Enabled: true,
		QRISAmount: 1_000, RetentionDays: 30,
	}
	principal := domain.Principal{
		ContextKind: domain.ContextTenant, TenantID: domain.InitialTenantID(), MembershipID: uuid.New(), DataSpaceID: domain.LiveDataSpaceID(),
		Role: domain.RoleSuperadmin, DataMode: domain.DataModeProduction,
	}

	status, err := service.Status(context.Background(), principal)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Enabled || status.DataSpaceID == nil || *status.DataSpaceID != spaceID ||
		status.Generation == nil || *status.Generation != 4 || status.QrisAmount != 1_000 ||
		status.RetentionDays != 30 {
		t.Fatalf("unexpected sandbox status: %+v", status)
	}

	_, err = service.Reset(context.Background(), principal, domain.ResetSandboxInput{
		ExpectedGeneration: 4,
		Confirmation:       "RESET SANDBOX",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !repository.resetCalled || repository.resetGeneration != 4 ||
		repository.resetRetention != 30*24*time.Hour {
		t.Fatalf("unexpected reset call: %+v", repository)
	}
}

func TestSandboxDisabledStatusDoesNotRequireAnActivatedGeneration(t *testing.T) {
	t.Parallel()

	repository := &sandboxRepository{
		activeErr: domain.NewError(domain.CodeNotFound, "Data tidak ditemukan"),
	}
	service := Sandbox{Repo: repository, Enabled: false}
	status, err := service.Status(context.Background(), sandboxTestPrincipal(domain.RoleAdmin, domain.DataModeProduction))
	if err != nil {
		t.Fatal(err)
	}
	if status.Enabled || status.DataSpaceID != nil || status.Generation != nil ||
		status.DataMode != domain.DataModeSandbox || !repository.activeCalled {
		t.Fatalf("unexpected never-activated disabled status: %+v", status)
	}
	if _, err = service.Initialize(context.Background()); err != nil || repository.ensureCalled {
		t.Fatalf("disabled initialization called repository: err=%v called=%v", err, repository.ensureCalled)
	}
}

func TestSandboxDisabledStatusStillReportsPreviouslyActivatedGeneration(t *testing.T) {
	t.Parallel()

	spaceID := uuid.New()
	repository := &sandboxRepository{active: domain.DataSpace{
		ID: spaceID, Mode: domain.DataModeSandbox, Generation: 8,
		Status: domain.DataSpaceStatusActive,
	}}
	status, err := (Sandbox{Repo: repository, Enabled: false}).Status(
		context.Background(),
		sandboxTestPrincipal(domain.RoleAdmin, domain.DataModeProduction),
	)
	if err != nil {
		t.Fatal(err)
	}
	if status.Enabled || status.DataSpaceID == nil || *status.DataSpaceID != spaceID ||
		status.Generation == nil || *status.Generation != 8 || !repository.activeCalled {
		t.Fatalf("disabled status lost retained generation identity: %+v", status)
	}
}

func TestSandboxInitializeNeverDependsOnOriginalTenant(t *testing.T) {
	t.Parallel()

	repository := &sandboxRepository{active: domain.DataSpace{
		ID: uuid.New(), Mode: domain.DataModeSandbox, Generation: 1,
		Status: domain.DataSpaceStatusActive,
	}}
	space, err := (Sandbox{Repo: repository, Enabled: true}).Initialize(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if repository.ensureCalled || space.ID != uuid.Nil {
		t.Fatalf("initialization result=%+v repository=%+v", space, repository)
	}
}

func TestSandboxResetRejectsUnsafeRequests(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		service   Sandbox
		principal domain.Principal
		input     domain.ResetSandboxInput
		code      string
	}{
		{
			name:      "disabled",
			service:   Sandbox{Repo: &sandboxRepository{}, Enabled: false},
			principal: sandboxTestPrincipal(domain.RoleSuperadmin, domain.DataModeProduction),
			input:     domain.ResetSandboxInput{ExpectedGeneration: 1, Confirmation: "RESET SANDBOX"},
			code:      domain.CodeForbidden,
		},
		{
			name:      "sandbox session",
			service:   Sandbox{Repo: &sandboxRepository{}, Enabled: true},
			principal: sandboxTestPrincipal(domain.RoleSuperadmin, domain.DataModeSandbox),
			input:     domain.ResetSandboxInput{ExpectedGeneration: 1, Confirmation: "RESET SANDBOX"},
			code:      domain.CodeForbidden,
		},
		{
			name:      "admin",
			service:   Sandbox{Repo: &sandboxRepository{}, Enabled: true},
			principal: sandboxTestPrincipal(domain.RoleAdmin, domain.DataModeProduction),
			input:     domain.ResetSandboxInput{ExpectedGeneration: 1, Confirmation: "RESET SANDBOX"},
			code:      domain.CodeForbidden,
		},
		{
			name:      "wrong confirmation",
			service:   Sandbox{Repo: &sandboxRepository{}, Enabled: true},
			principal: sandboxTestPrincipal(domain.RoleSuperadmin, domain.DataModeProduction),
			input:     domain.ResetSandboxInput{ExpectedGeneration: 1, Confirmation: "RESET"},
			code:      domain.CodeValidation,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := test.service.Reset(context.Background(), test.principal, test.input)
			if !domain.IsCode(err, test.code) {
				t.Fatalf("error=%v, want code %s", err, test.code)
			}
		})
	}
}

func TestSandboxCleanupUsesClockEvenWhenModeIsDisabled(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 5, 3, 4, 5, 0, time.UTC)
	repository := &sandboxRepository{}
	service := Sandbox{Repo: repository, Clock: fixedClock{value: now}}
	result, err := service.Cleanup(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.PurgedGenerationCount != 1 || !repository.cleanupAt.Equal(now) {
		t.Fatalf("cleanup result=%+v at=%v", result, repository.cleanupAt)
	}
}

func sandboxTestPrincipal(role domain.Role, mode domain.DataMode) domain.Principal {
	return domain.Principal{ContextKind: domain.ContextTenant, TenantID: domain.InitialTenantID(), MembershipID: uuid.New(), DataSpaceID: domain.LiveDataSpaceID(), Role: role, DataMode: mode}
}
