package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/observability"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/port"
)

type Reporting struct {
	Repo     port.Repository
	Exporter port.Exporter
}

func (r Reporting) Dashboard(ctx context.Context, principal domain.Principal, from, to time.Time, bucket string) (domain.Dashboard, error) {
	defer observability.StartSegment(ctx, "Usecase.Reporting.Dashboard")()
	if err := RequireReady(principal); err != nil {
		return domain.Dashboard{}, err
	}
	if !from.Before(to) || to.Sub(from) > 370*24*time.Hour {
		return domain.Dashboard{}, domain.Validation("Rentang laporan tidak valid", nil)
	}
	switch bucket {
	case "day", "week", "month":
	default:
		bucket = "day"
	}
	return r.Repo.Dashboard(ctx, from, to, bucket)
}

func (r Reporting) Export(ctx context.Context, principal domain.Principal, format string, filter domain.TransactionFilter) ([]byte, string, error) {
	defer observability.StartSegment(ctx, "Usecase.Reporting.Export")()
	if err := RequireReady(principal); err != nil {
		return nil, "", err
	}
	if filter.PaymentMethod != nil && !filter.PaymentMethod.Valid() {
		return nil, "", domain.Validation(
			"Metode pembayaran tidak valid",
			map[string]any{"field": "filters.paymentMethod"},
		)
	}
	if filter.PaymentStatus != nil && !filter.PaymentStatus.Valid() {
		return nil, "", domain.Validation(
			"Status pembayaran tidak valid",
			map[string]any{"field": "filters.paymentStatus"},
		)
	}
	if !principal.IsSuperadmin() {
		filter.IncludeDeleted = false
	}
	filter.Limit = 10000
	rows, err := r.Repo.ExportRows(ctx, filter)
	if err != nil {
		return nil, "", err
	}
	switch format {
	case "xlsx":
		body, exportErr := r.Exporter.XLSX(rows, filter.From, filter.To)
		return body, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", exportErr
	case "pdf":
		body, exportErr := r.Exporter.PDF(rows, filter.From, filter.To)
		return body, "application/pdf", exportErr
	default:
		return nil, "", domain.Validation("Format ekspor harus xlsx atau pdf", map[string]any{"field": "format"})
	}
}

func JakartaRange(from, to string) (time.Time, time.Time, error) {
	location, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("load Asia/Jakarta: %w", err)
	}
	start, err := time.ParseInLocation("2006-01-02", from, location)
	if err != nil {
		return time.Time{}, time.Time{}, domain.Validation("Tanggal awal tidak valid", map[string]any{"field": "from"})
	}
	endDay, err := time.ParseInLocation("2006-01-02", to, location)
	if err != nil {
		return time.Time{}, time.Time{}, domain.Validation("Tanggal akhir tidak valid", map[string]any{"field": "to"})
	}
	return start.UTC(), endDay.AddDate(0, 0, 1).UTC(), nil
}
