package postgres

import (
	"context"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/observability"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s *Store) ListPackages(ctx context.Context, includeDeleted bool) ([]domain.Package, error) {
	defer observability.StartSegment(ctx, "Postgres.ListPackages")()
	query := s.ORM.WithContext(ctx).
		Table("packages AS p").
		Select(`
			p.id, p.code, p.current_revision, r.name, r.description, r.unit_price,
			p.created_at, p.updated_at, p.deleted_at`).
		Joins("JOIN package_revisions AS r ON r.package_id = p.id AND r.revision = p.current_revision")
	if !includeDeleted {
		query = query.Where("p.deleted_at IS NULL")
	}
	var records []packageProjection
	if err := query.Order("r.name, p.id").Scan(&records).Error; err != nil {
		return nil, dbError(err, "list packages")
	}
	packages := make([]domain.Package, 0, len(records))
	for _, record := range records {
		packages = append(packages, record.domainPackage())
	}
	return packages, nil
}

func (s *Store) GetPackage(ctx context.Context, id uuid.UUID) (domain.Package, error) {
	defer observability.StartSegment(ctx, "Postgres.GetPackage")()
	var record packageProjection
	err := s.ORM.WithContext(ctx).
		Table("packages AS p").
		Select(`
			p.id, p.code, p.current_revision, r.name, r.description, r.unit_price,
			p.created_at, p.updated_at, p.deleted_at`).
		Joins("JOIN package_revisions AS r ON r.package_id = p.id AND r.revision = p.current_revision").
		Where("p.id = ?", id).
		Take(&record).Error
	return record.domainPackage(), dbError(err, "get package")
}

func (s *Store) CreatePackage(ctx context.Context, actor domain.Principal, input domain.CreatePackageInput) (domain.Package, error) {
	defer observability.StartSegment(ctx, "Postgres.CreatePackage")()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return domain.Package{}, dbError(err, "begin create package")
	}
	defer tx.Rollback(ctx)
	id := uuid.New()
	_, err = tx.Exec(ctx, `
		INSERT INTO packages (id, code, current_revision, created_by, updated_by)
		VALUES ($1,$2,1,$3,$3)`,
		id, input.Code, actor.UserID,
	)
	if err != nil {
		return domain.Package{}, dbError(err, "create package")
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO package_revisions (
			package_id, revision, name, description, unit_price, change_reason, created_by
		) VALUES ($1,1,$2,$3,$4,$5,$6)`,
		id, input.Name, input.Description, input.UnitPrice, nullText(input.ChangeReason), actor.UserID,
	)
	if err != nil {
		return domain.Package{}, dbError(err, "create package revision")
	}
	item, err := packageByID(ctx, tx, id)
	if err != nil {
		return domain.Package{}, dbError(err, "read created package")
	}
	identity := principalIdentity(actor)
	if err = audit(ctx, tx, "package.created", "package", id.String(), identity, nil, item, nil, s.Now()); err != nil {
		return domain.Package{}, dbError(err, "audit create package")
	}
	revision := 1
	if err = addChange(ctx, tx, "package", id.String(), "created", &revision, item, false); err != nil {
		return domain.Package{}, dbError(err, "sync create package")
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.Package{}, dbError(err, "commit create package")
	}
	return item, nil
}

func (s *Store) UpdatePackage(ctx context.Context, actor domain.Principal, id uuid.UUID, input domain.UpdatePackageInput) (domain.Package, error) {
	defer observability.StartSegment(ctx, "Postgres.UpdatePackage")()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return domain.Package{}, dbError(err, "begin update package")
	}
	defer tx.Rollback(ctx)
	before, err := packageForUpdate(ctx, tx, id)
	if err != nil {
		return domain.Package{}, dbError(err, "lock package")
	}
	if before.DeletedAt != nil {
		return domain.Package{}, domain.NewError(domain.CodeConflict, "Paket yang dihapus tidak dapat diubah")
	}
	nextRevision := before.CurrentRevision + 1
	_, err = tx.Exec(ctx, `
		INSERT INTO package_revisions (
			package_id, revision, name, description, unit_price, change_reason, created_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		id, nextRevision, input.Name, input.Description, input.UnitPrice, input.ChangeReason, actor.UserID,
	)
	if err != nil {
		return domain.Package{}, dbError(err, "create package revision")
	}
	_, err = tx.Exec(ctx, `
		UPDATE packages
		SET current_revision = $2, updated_by = $3, updated_at = now()
		WHERE id = $1`,
		id, nextRevision, actor.UserID,
	)
	if err != nil {
		return domain.Package{}, dbError(err, "advance package revision")
	}
	after, err := packageByID(ctx, tx, id)
	if err != nil {
		return domain.Package{}, dbError(err, "read updated package")
	}
	identity := principalIdentity(actor)
	if err = audit(ctx, tx, "package.updated", "package", id.String(), identity, before, after,
		map[string]any{"reason": input.ChangeReason}, s.Now()); err != nil {
		return domain.Package{}, dbError(err, "audit update package")
	}
	if err = addChange(ctx, tx, "package", id.String(), "updated", &nextRevision, after, false); err != nil {
		return domain.Package{}, dbError(err, "sync update package")
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.Package{}, dbError(err, "commit update package")
	}
	return after, nil
}

func (s *Store) DeletePackage(ctx context.Context, actor domain.Principal, id uuid.UUID, reason string) error {
	defer observability.StartSegment(ctx, "Postgres.DeletePackage")()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return dbError(err, "begin delete package")
	}
	defer tx.Rollback(ctx)
	before, err := packageForUpdate(ctx, tx, id)
	if err != nil {
		return dbError(err, "lock deleted package")
	}
	if before.DeletedAt != nil {
		return domain.NewError(domain.CodeConflict, "Paket sudah dihapus")
	}
	_, err = tx.Exec(ctx, `
		UPDATE packages
		SET deleted_at = now(), deleted_by = $2, updated_by = $2, updated_at = now()
		WHERE id = $1`,
		id, actor.UserID,
	)
	if err != nil {
		return dbError(err, "delete package")
	}
	after := map[string]any{
		"id": id, "revision": before.CurrentRevision, "deleted": true,
	}
	identity := principalIdentity(actor)
	if err = audit(ctx, tx, "package.deleted", "package", id.String(), identity, before, after,
		map[string]any{"reason": reason}, s.Now()); err != nil {
		return dbError(err, "audit delete package")
	}
	revision := before.CurrentRevision
	if err = addChange(ctx, tx, "package", id.String(), "deleted", &revision, after, true); err != nil {
		return dbError(err, "sync delete package")
	}
	return dbError(tx.Commit(ctx), "commit delete package")
}

func packageByID(ctx context.Context, query rowQuerier, id uuid.UUID) (domain.Package, error) {
	defer observability.StartSegment(ctx, "Postgres.packageByID")()
	var item domain.Package
	err := query.QueryRow(ctx, `
		SELECT p.id, p.code, p.current_revision, r.name, r.description, r.unit_price,
		       p.created_at, p.updated_at, p.deleted_at
		FROM packages p
		JOIN package_revisions r ON r.package_id = p.id AND r.revision = p.current_revision
		WHERE p.id = $1`,
		id,
	).Scan(
		&item.ID, &item.Code, &item.CurrentRevision, &item.Name, &item.Description,
		&item.UnitPrice, &item.CreatedAt, &item.UpdatedAt, &item.DeletedAt,
	)
	return item, err
}

func packageForUpdate(ctx context.Context, tx pgx.Tx, id uuid.UUID) (domain.Package, error) {
	defer observability.StartSegment(ctx, "Postgres.packageForUpdate")()
	var item domain.Package
	err := tx.QueryRow(ctx, `
		SELECT p.id, p.code, p.current_revision, r.name, r.description, r.unit_price,
		       p.created_at, p.updated_at, p.deleted_at
		FROM packages p
		JOIN package_revisions r ON r.package_id = p.id AND r.revision = p.current_revision
		WHERE p.id = $1
		FOR UPDATE OF p`,
		id,
	).Scan(
		&item.ID, &item.Code, &item.CurrentRevision, &item.Name, &item.Description,
		&item.UnitPrice, &item.CreatedAt, &item.UpdatedAt, &item.DeletedAt,
	)
	return item, err
}

func nullText(value string) any {
	if value == "" {
		return nil
	}
	return value
}
