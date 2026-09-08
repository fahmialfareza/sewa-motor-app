package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/observability"
	"github.com/google/uuid"
)

func (s *Store) Dashboard(ctx context.Context, dataSpaceID uuid.UUID, from, to time.Time, bucket string) (domain.Dashboard, error) {
	defer observability.StartSegment(ctx, "Postgres.Dashboard")()
	dataSpaceID = domain.EffectiveDataSpaceID(dataSpaceID)
	result := domain.Dashboard{From: from, To: to}
	if err := s.Pool.QueryRow(ctx, `
		SELECT COALESCE(sum(total), 0),
		       COALESCE(sum(CASE WHEN payment_method = 'qris' THEN payment_amount ELSE 0 END), 0),
		       count(*)
		FROM transactions
		WHERE data_space_id = $3 AND deleted_at IS NULL
		  AND payment_status = 'success'
		  AND payment_confirmed_revision = current_revision
		  AND occurred_at >= $1 AND occurred_at < $2`,
		from, to, dataSpaceID,
	).Scan(&result.GrossRevenue, &result.ActualQrisAmount, &result.TransactionCount); err != nil {
		return domain.Dashboard{}, dbError(err, "dashboard totals")
	}

	rows, err := s.Pool.Query(ctx, `
		SELECT i.package_id, i.package_name, sum(i.quantity)
		FROM transactions t
		JOIN transaction_items i
		  ON i.transaction_id = t.id AND i.revision = t.current_revision
		 AND i.data_space_id = t.data_space_id
		WHERE t.data_space_id = $3 AND t.deleted_at IS NULL
		  AND t.payment_status = 'success'
		  AND t.payment_confirmed_revision = t.current_revision
		  AND t.occurred_at >= $1 AND t.occurred_at < $2
		GROUP BY i.package_id, i.package_name
		ORDER BY i.package_name`,
		from, to, dataSpaceID,
	)
	if err != nil {
		return domain.Dashboard{}, dbError(err, "dashboard package quantities")
	}
	result.PackageQuantities = make([]domain.PackageQuantity, 0)
	for rows.Next() {
		var quantity domain.PackageQuantity
		if err := rows.Scan(&quantity.PackageID, &quantity.PackageName, &quantity.Quantity); err != nil {
			rows.Close()
			return domain.Dashboard{}, dbError(err, "scan dashboard package quantity")
		}
		result.PackageQuantities = append(result.PackageQuantities, quantity)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return domain.Dashboard{}, dbError(err, "iterate dashboard package quantities")
	}
	rows.Close()

	rows, err = s.Pool.Query(ctx, `
		SELECT (
			date_trunc($3, t.occurred_at AT TIME ZONE 'Asia/Jakarta')
			AT TIME ZONE 'Asia/Jakarta'
		) AS bucket, sum(t.total), count(*)
		FROM transactions t
		WHERE t.data_space_id = $4 AND t.deleted_at IS NULL
		  AND t.payment_status = 'success'
		  AND t.payment_confirmed_revision = t.current_revision
		  AND t.occurred_at >= $1 AND t.occurred_at < $2
		GROUP BY bucket
		ORDER BY bucket`,
		from, to, bucket, dataSpaceID,
	)
	if err != nil {
		return domain.Dashboard{}, dbError(err, "dashboard trend")
	}
	result.Trend = make([]domain.TrendBucket, 0)
	for rows.Next() {
		var point domain.TrendBucket
		if err := rows.Scan(&point.Bucket, &point.Total, &point.Count); err != nil {
			rows.Close()
			return domain.Dashboard{}, dbError(err, "scan dashboard trend")
		}
		result.Trend = append(result.Trend, point)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return domain.Dashboard{}, dbError(err, "iterate dashboard trend")
	}
	rows.Close()

	success := domain.PaymentStatusSuccess
	page, err := s.ListTransactions(ctx, domain.TransactionFilter{
		From: &from, To: &to, PaymentStatus: &success, Limit: 5, DataSpaceID: dataSpaceID,
	})
	if err != nil {
		return domain.Dashboard{}, err
	}
	result.RecentTransactions = page.Transactions
	return result, nil
}

func (s *Store) ExportRows(ctx context.Context, filter domain.TransactionFilter) ([]domain.ExportRow, error) {
	defer observability.StartSegment(ctx, "Postgres.ExportRows")()
	conditions, args := exportQueryConditions(filter)
	sql := `
		SELECT t.id, t.occurred_at, t.current_revision,
		       i.package_code, i.package_name, i.package_revision,
		       i.unit_price, i.quantity, i.line_total, t.total, t.payment_amount,
		       u.full_name, u.username, t.payment_method, t.qris_payload_hash,
		       t.payment_status, t.print_state
		FROM transactions t
		JOIN transaction_items i
		  ON i.transaction_id = t.id AND i.revision = t.current_revision
		 AND i.data_space_id = t.data_space_id
		JOIN users u ON u.id = t.origin_actor_id
		WHERE ` + strings.Join(conditions, " AND ") + `
		ORDER BY t.occurred_at, t.id, i.line_number
		LIMIT 100000`
	rows, err := s.Pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, dbError(err, "query export rows")
	}
	defer rows.Close()
	result := make([]domain.ExportRow, 0)
	for rows.Next() {
		var row domain.ExportRow
		if err := rows.Scan(
			&row.TransactionID, &row.OccurredAt, &row.Revision,
			&row.PackageCode, &row.PackageName, &row.PackageRevision,
			&row.UnitPrice, &row.Quantity, &row.LineTotal, &row.TransactionTotal, &row.PaymentAmount,
			&row.CreatorName, &row.CreatorUsername,
			&row.PaymentMethod, &row.QrisPayloadHash,
			&row.PaymentStatus, &row.PrintState,
		); err != nil {
			return nil, dbError(err, "scan export row")
		}
		result = append(result, row)
	}
	return result, dbError(rows.Err(), "iterate export rows")
}

func exportQueryConditions(filter domain.TransactionFilter) ([]string, []any) {
	args := make([]any, 0, 8)
	add := func(value any) string {
		args = append(args, value)
		return fmt.Sprintf("$%d", len(args))
	}
	conditions := []string{"t.data_space_id = " + add(domain.EffectiveDataSpaceID(filter.DataSpaceID))}
	if !filter.IncludeDeleted {
		conditions = append(conditions, "t.deleted_at IS NULL")
	}
	search := strings.ToUpper(strings.TrimSpace(filter.Search))
	search = strings.TrimPrefix(search, "TEST-TRX-")
	search = strings.TrimPrefix(search, "TRX-")
	if search != "" {
		conditions = append(conditions, "t.id LIKE "+add(search+"%"))
	}
	if filter.From != nil {
		conditions = append(conditions, "t.occurred_at >= "+add(*filter.From))
	}
	if filter.To != nil {
		conditions = append(conditions, "t.occurred_at < "+add(*filter.To))
	}
	if filter.PackageID != nil {
		conditions = append(conditions, "i.package_id = "+add(*filter.PackageID))
	}
	if filter.CreatorID != nil {
		conditions = append(conditions, "t.origin_actor_id = "+add(*filter.CreatorID))
	}
	if filter.TerminalID != nil {
		conditions = append(conditions, "t.terminal_id = "+add(*filter.TerminalID))
	}
	if filter.PaymentMethod != nil {
		conditions = append(conditions, "t.payment_method = "+add(*filter.PaymentMethod))
	}
	if filter.PaymentStatus != nil {
		conditions = append(conditions, "t.payment_status = "+add(*filter.PaymentStatus))
	}
	return conditions, args
}
