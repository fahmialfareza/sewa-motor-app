package postgres

import (
	"context"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/observability"
	"github.com/jackc/pgx/v5"
)

func resolveReceiptIdentity(ctx context.Context, tx pgx.Tx, identity domain.MutationIdentity, revision *int) (domain.ReceiptIdentity, error) {
	defer observability.StartSegment(ctx, "Postgres.resolveReceiptIdentity")()
	var result domain.ReceiptIdentity
	err := tx.QueryRow(ctx, `SELECT p.business_name,p.address,p.phone,p.revision
		FROM data_spaces ds JOIN tenants t ON t.id=ds.tenant_id
		JOIN tenant_profile_revisions p ON p.tenant_id=t.id AND p.revision=COALESCE($2,t.profile_revision)
		WHERE ds.id=$1`, identity.DataSpaceID, revision).Scan(&result.BusinessName, &result.Address, &result.Phone, &result.Revision)
	if err != nil {
		return result, dbError(err, "resolve receipt business profile")
	}
	return result, nil
}

// validateMerchantBinding accepts historical offline evidence only if its
// immutable origin session was migrated from the original single-tenant app.
// The transport/app version is deliberately irrelevant to this decision.
func validateMerchantBinding(ctx context.Context, tx pgx.Tx, identity domain.MutationIdentity, method domain.PaymentMethod, hash *string) error {
	defer observability.StartSegment(ctx, "Postgres.validateMerchantBinding")()
	if method != domain.PaymentMethodQRIS {
		return nil
	}
	if err := domain.ValidateQrisPayloadBinding(method, hash); err != nil {
		return err
	}
	var allowed bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1 FROM tenant_qris_revisions q JOIN data_spaces ds ON ds.tenant_id=q.tenant_id
		WHERE ds.id=$1 AND q.payload_hash=$2
	) OR EXISTS(
		SELECT 1 FROM sessions s JOIN data_spaces ds ON ds.id=s.data_space_id
		WHERE s.id=$3 AND s.user_id=$4 AND s.data_space_id=$1 AND s.legacy_origin
		AND ds.tenant_id=$5
	)`, identity.DataSpaceID, hash, identity.OriginSessionID, identity.OriginActorID, domain.InitialTenantID()).Scan(&allowed)
	if err != nil {
		return dbError(err, "validate tenant merchant binding")
	}
	if !allowed {
		return domain.Validation("QRIS belum terdaftar untuk usaha ini. Sinkronkan pengaturan QRIS sebelum melanjutkan.", nil)
	}
	return nil
}
