package usecase

import (
	"context"
	"testing"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
	"github.com/google/uuid"
)

func TestAdminCannotManageTerminalEnrollment(t *testing.T) {
	p := domain.Principal{UserID: uuid.New(), SessionID: uuid.New(), ContextKind: domain.ContextTenant, TenantID: domain.InitialTenantID(), MembershipID: uuid.New(), DataSpaceID: domain.LiveDataSpaceID(), DataMode: domain.DataModeProduction, Role: domain.RoleAdmin}
	_, err := (Terminals{}).Enroll(context.Background(), p, domain.EnrollTerminalInput{})
	if !domain.IsCode(err, domain.CodeForbidden) {
		t.Fatalf("Admin enrollment=%v", err)
	}
}
