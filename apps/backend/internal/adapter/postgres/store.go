package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/observability"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/newrelic/go-agent/v3/integrations/nrpgx5"
	"github.com/sirupsen/logrus"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type Store struct {
	Pool   *pgxpool.Pool
	ORM    *gorm.DB
	ormSQL *sql.DB
	Logger *logrus.Logger
	Now    func() time.Time
}

type Option func(*storeOptions)

type storeOptions struct {
	logger *logrus.Logger
}

func WithLogger(logger *logrus.Logger) Option {
	return func(options *storeOptions) {
		options.logger = logger
	}
}

func Open(ctx context.Context, databaseURL string, options ...Option) (*Store, error) {
	defer observability.StartSegment(ctx, "Postgres.Open")()

	settings := storeOptions{logger: observability.Logger()}
	for _, option := range options {
		option(&settings)
	}
	if settings.logger == nil {
		settings.logger = observability.Logger()
	}

	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	config.ConnConfig.Tracer = nrpgx5.NewTracer(nrpgx5.WithQueryParameters(false))
	config.MaxConns = 20
	config.MinConns = 2
	config.MaxConnLifetime = time.Hour
	config.MaxConnIdleTime = 15 * time.Minute
	config.HealthCheckPeriod = 30 * time.Second
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("open postgres pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	orm, err := gorm.Open(gormpostgres.Open(databaseURL), &gorm.Config{
		Logger:         newGORMLogger(settings.logger),
		TranslateError: true,
	})
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("open GORM postgres connection: %w", err)
	}
	if err := orm.Use(newRelicGORMPlugin{}); err != nil {
		pool.Close()
		return nil, fmt.Errorf("register GORM New Relic plugin: %w", err)
	}
	ormSQL, err := orm.DB()
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("resolve GORM SQL pool: %w", err)
	}
	ormSQL.SetMaxOpenConns(12)
	ormSQL.SetMaxIdleConns(2)
	ormSQL.SetConnMaxLifetime(time.Hour)
	ormSQL.SetConnMaxIdleTime(15 * time.Minute)
	if err := ormSQL.PingContext(ctx); err != nil {
		_ = ormSQL.Close()
		pool.Close()
		return nil, fmt.Errorf("ping GORM postgres: %w", err)
	}

	return &Store{
		Pool:   pool,
		ORM:    orm,
		ormSQL: ormSQL,
		Logger: settings.logger,
		Now:    func() time.Time { return time.Now().UTC() },
	}, nil
}

func (s *Store) Close() {
	if s == nil {
		return
	}
	if s.ormSQL != nil {
		if err := s.ormSQL.Close(); err != nil {
			s.Logger.WithError(err).Error("close GORM postgres pool")
		}
	}
	if s.Pool != nil {
		s.Pool.Close()
	}
}

func (s *Store) Ping(ctx context.Context) error {
	defer observability.StartSegment(ctx, "Postgres.Ping")()
	if s.ormSQL != nil {
		if err := s.ormSQL.PingContext(ctx); err != nil {
			observability.NoticeError(ctx, err, "ping GORM postgres")
			return err
		}
	}
	if err := s.Pool.Ping(ctx); err != nil {
		observability.NoticeError(ctx, err, "ping pgx postgres")
		return err
	}
	return nil
}

func dbError(err error, operation string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.NewError(domain.CodeNotFound, "Data tidak ditemukan")
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return &domain.Error{Code: domain.CodeConflict, Message: "Data sudah digunakan", Cause: err}
	}
	if errors.Is(err, gorm.ErrForeignKeyViolated) {
		return &domain.Error{Code: domain.CodeValidation, Message: "Data tidak memenuhi aturan sistem", Cause: err}
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return &domain.Error{Code: domain.CodeConflict, Message: "Data sudah digunakan", Details: map[string]any{"constraint": pgErr.ConstraintName}, Cause: err}
		case "23503", "23514", "22P02":
			return &domain.Error{Code: domain.CodeValidation, Message: "Data tidak memenuhi aturan sistem", Details: map[string]any{"constraint": pgErr.ConstraintName}, Cause: err}
		case "40001", "40P01":
			return &domain.Error{Code: domain.CodeConflict, Message: "Data berubah bersamaan; silakan coba lagi", Cause: err}
		}
	}
	return domain.WrapInternal(err, operation)
}

func audit(ctx context.Context, tx pgx.Tx, eventType, aggregateType, aggregateID string, identity domain.MutationIdentity, before, after any, metadata map[string]any, occurredAt time.Time) error {
	defer observability.StartSegment(ctx, "Postgres.audit")()
	beforeJSON, err := nullableJSON(before)
	if err != nil {
		return err
	}
	afterJSON, err := nullableJSON(after)
	if err != nil {
		return err
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO audit_events (
			event_type, aggregate_type, aggregate_id,
			origin_actor_id, origin_session_id,
			submitted_by_actor_id, submitted_by_session_id, terminal_id,
			before_values, after_values, metadata, occurred_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		eventType, aggregateType, aggregateID,
		nilUUID(identity.OriginActorID), nilUUID(identity.OriginSessionID),
		nilUUID(identity.SubmittedByActorID), nilUUID(identity.SubmittedBySessionID), identity.TerminalID,
		beforeJSON, afterJSON, metadataJSON, occurredAt,
	)
	return err
}

func addChange(ctx context.Context, tx pgx.Tx, aggregate, aggregateID, action string, revision *int, payload any, tombstone bool) error {
	defer observability.StartSegment(ctx, "Postgres.addChange")()
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO sync_changes (aggregate, aggregate_id, action, revision, payload, tombstone)
		VALUES ($1,$2,$3,$4,$5,$6)`,
		aggregate, aggregateID, action, revision, body, tombstone,
	)
	return err
}

func nullableJSON(value any) ([]byte, error) {
	if value == nil {
		return nil, nil
	}
	return json.Marshal(value)
}

func nilUUID(value uuid.UUID) any {
	if value == uuid.Nil {
		return nil
	}
	return value
}

func principalIdentity(principal domain.Principal) domain.MutationIdentity {
	return domain.MutationIdentity{
		OriginActorID:        principal.UserID,
		OriginSessionID:      principal.SessionID,
		TerminalID:           principal.TerminalID,
		SubmittedByActorID:   principal.UserID,
		SubmittedBySessionID: principal.SessionID,
	}
}
