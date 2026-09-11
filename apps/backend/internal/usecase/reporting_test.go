package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/port"
	"github.com/google/uuid"
)

type reportingRepository struct {
	port.Repository
	filter domain.TransactionFilter
	called bool
}

func (repository *reportingRepository) ExportRows(
	_ context.Context,
	filter domain.TransactionFilter,
) ([]domain.ExportRow, error) {
	repository.called = true
	repository.filter = filter
	return nil, nil
}

type reportingExporter struct{}

func (repository *reportingRepository) GetTenantProfile(context.Context, uuid.UUID) (domain.TenantProfile, error) {
	return domain.TenantProfile{BusinessName: "Test Business"}, nil
}

func (reportingExporter) XLSX(
	[]domain.ExportRow,
	*time.Time,
	*time.Time,
	domain.DataMode,
	domain.TenantProfile,
) ([]byte, error) {
	return []byte("xlsx"), nil
}

func (reportingExporter) PDF(
	[]domain.ExportRow,
	*time.Time,
	*time.Time,
	domain.DataMode,
	domain.TenantProfile,
) ([]byte, error) {
	return []byte("pdf"), nil
}

func TestExportRestrictsDeletedRowsAndValidatesPaymentFilters(t *testing.T) {
	t.Parallel()

	repository := &reportingRepository{}
	service := Reporting{Repo: repository, Exporter: reportingExporter{}}
	failed := domain.PaymentStatusFailed
	if _, _, err := service.Export(
		context.Background(),
		domain.Principal{ContextKind: domain.ContextTenant, TenantID: domain.InitialTenantID(), MembershipID: uuid.New(), DataSpaceID: domain.LiveDataSpaceID(), Role: domain.RoleAdmin},
		"xlsx",
		domain.TransactionFilter{
			PaymentStatus:  &failed,
			IncludeDeleted: true,
		},
	); err != nil {
		t.Fatal(err)
	}
	if !repository.called ||
		repository.filter.IncludeDeleted ||
		repository.filter.PaymentStatus == nil ||
		*repository.filter.PaymentStatus != domain.PaymentStatusFailed {
		t.Fatalf("admin export filter was not normalized: %+v", repository.filter)
	}

	repository.called = false
	invalid := domain.PaymentMethod("card")
	if _, _, err := service.Export(
		context.Background(),
		domain.Principal{ContextKind: domain.ContextTenant, TenantID: domain.InitialTenantID(), MembershipID: uuid.New(), DataSpaceID: domain.LiveDataSpaceID(), Role: domain.RoleSuperadmin},
		"xlsx",
		domain.TransactionFilter{PaymentMethod: &invalid},
	); !domain.IsCode(err, domain.CodeValidation) {
		t.Fatalf("invalid payment method error=%v", err)
	}
	if repository.called {
		t.Fatal("invalid filter reached the repository")
	}
}
