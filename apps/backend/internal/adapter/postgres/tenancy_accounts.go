package postgres

import (
	"context"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/observability"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Account owners may update global profile/credentials from account or platform
// context, or an authorized Production context. The account lock also coordinates
// operator recovery and all business mutations from that account.
func lockOwnAccount(ctx context.Context, tx pgx.Tx, actor domain.Principal, allowTemporary bool) error {
	defer observability.StartSegment(ctx, "Postgres.lockOwnAccount")()
	var active, temporary bool
	if err := tx.QueryRow(ctx, `SELECT is_active AND deleted_at IS NULL,must_change_password FROM users WHERE id=$1 FOR UPDATE`, actor.UserID).Scan(&active, &temporary); err != nil {
		return err
	}
	if !active {
		return domain.NewError(domain.CodeUnauthorized, "Akun tidak aktif")
	}
	if temporary && !allowTemporary {
		return domain.NewError(domain.CodePasswordChange, "Ganti kata sandi sementara sebelum melanjutkan")
	}
	if actor.ContextKind == domain.ContextTenant {
		if !actor.IsTenantContext() || actor.DataMode != domain.DataModeProduction {
			return domain.NewError(domain.CodeForbidden, "Perubahan akun hanya tersedia di mode produksi")
		}
		var status string
		if err := tx.QueryRow(ctx, `SELECT status FROM tenants WHERE id=$1 FOR SHARE`, actor.TenantID).Scan(&status); err != nil {
			return err
		}
		if status != "active" {
			return domain.NewError(domain.CodeTenantSuspended, "Bisnis sedang dinonaktifkan")
		}
		var valid bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM tenant_memberships m JOIN data_spaces ds ON ds.tenant_id=m.tenant_id
   WHERE m.id=$1 AND m.user_id=$2 AND m.tenant_id=$3 AND m.status='active' AND ds.id=$4 AND ds.mode='production' AND ds.status='active')
   AND ($5::uuid IS NULL OR EXISTS(SELECT 1 FROM terminals WHERE id=$5 AND tenant_id=$3 AND is_active AND revoked_at IS NULL))`, actor.MembershipID, actor.UserID, actor.TenantID, actor.DataSpaceID, actor.TerminalID).Scan(&valid); err != nil {
			return err
		}
		if !valid {
			return domain.NewError(domain.CodeMembershipInactive, "Akses bisnis telah berubah. Pilih bisnis kembali")
		}
	}
	if err := validateControlSession(ctx, tx, actor, false); err != nil {
		return err
	}
	var id uuid.UUID
	return tx.QueryRow(ctx, `SELECT id FROM sessions WHERE id=$1 AND user_id=$2 AND revoked_at IS NULL FOR UPDATE`, actor.SessionID, actor.UserID).Scan(&id)
}

func (s *Store) UpdateOwnProfile(ctx context.Context, actor domain.Principal, name string) (user domain.User, err error) {
	defer observability.StartSegment(ctx, "Postgres.UpdateOwnProfile")()
	err = s.controlTx(ctx, func(tx pgx.Tx) error {
		if e := lockOwnAccount(ctx, tx, actor, false); e != nil {
			return e
		}
		var e error
		user, e = scanAccountUser(tx.QueryRow(ctx, `UPDATE users SET full_name=$2,updated_at=now() WHERE id=$1 RETURNING id,full_name,username,role,is_active,must_change_password,created_at,updated_at,deleted_at`, actor.UserID, name))
		if e != nil {
			return e
		}
		if e = controlAudit(ctx, tx, &actor.UserID, nil, "account.profile_updated", map[string]any{"fullName": name}); e != nil {
			return e
		}
		return addSharedChange(ctx, tx, "user", actor.UserID.String(), "updated", nil, user, false)
	})
	return
}

func (s *Store) ListPlatformAudit(ctx context.Context, actor domain.Principal, limit int) ([]domain.PlatformAuditEvent, error) {
	defer observability.StartSegment(ctx, "Postgres.ListPlatformAudit")()
	if actor.ContextKind != domain.ContextPlatform || !actor.IsPlatformAdmin {
		return nil, domain.NewError(domain.CodeForbidden, "Akses platform diperlukan")
	}
	if limit < 1 || limit > 200 {
		limit = 50
	}
	rows, err := s.Pool.Query(ctx, `SELECT id,event_type,actor_id,tenant_id,metadata,created_at FROM platform_audit_events ORDER BY created_at DESC,id DESC LIMIT $1`, limit)
	if err != nil {
		return nil, dbError(err, "read platform audit")
	}
	defer rows.Close()
	events := []domain.PlatformAuditEvent{}
	for rows.Next() {
		var event domain.PlatformAuditEvent
		if err = rows.Scan(&event.ID, &event.EventType, &event.ActorID, &event.TenantID, &event.Metadata, &event.CreatedAt); err != nil {
			return nil, dbError(err, "scan platform audit")
		}
		events = append(events, event)
	}
	return events, dbError(rows.Err(), "read platform audit")
}

func (s *Store) RevalidateOrigins(ctx context.Context, actor domain.Principal, origins []domain.OutboxOrigin) (results []domain.OutboxOriginValidation, err error) {
	defer observability.StartSegment(ctx, "Postgres.RevalidateOrigins")()
	if !actor.IsTenantContext() || actor.TerminalID == nil {
		return nil, domain.NewError(domain.CodeContextRequired, "Pilih bisnis dan terminal untuk memeriksa data tertunda")
	}
	if len(origins) > 100 {
		return nil, domain.Validation("Maksimal 100 identitas per pemeriksaan", nil)
	}
	results = make([]domain.OutboxOriginValidation, 0, len(origins))
	err = s.controlTx(ctx, func(tx pgx.Tx) error {
		if _, e := lockTenantAccess(ctx, tx, actor); e != nil {
			return e
		}
		for _, origin := range origins {
			result := domain.OutboxOriginValidation{OutboxOrigin: origin}
			// Another employee—even a superadmin—cannot release someone else's
			// quarantined evidence by signing in on the same physical installation.
			if origin.OriginActorID == actor.UserID && origin.TerminalID == *actor.TerminalID {
				if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sessions s JOIN tenant_memberships m ON m.id=s.membership_id
     WHERE s.id=$1 AND s.user_id=$2 AND s.terminal_id=$3 AND s.tenant_id=$4 AND s.data_space_id=$5
      AND s.context_kind='tenant' AND s.membership_id=$6 AND m.status='active')`, origin.OriginSessionID, actor.UserID, origin.TerminalID, actor.TenantID, actor.DataSpaceID, actor.MembershipID).Scan(&result.Allowed); e != nil {
					return e
				}
			}
			results = append(results, result)
		}
		return nil
	})
	return
}
