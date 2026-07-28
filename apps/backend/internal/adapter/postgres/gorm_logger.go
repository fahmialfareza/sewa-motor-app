package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/observability"
	"github.com/newrelic/go-agent/v3/newrelic"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const gormSegmentKey = "newrelic:gorm-segment"

type gormLogger struct {
	logger *logrus.Logger
	level  logger.LogLevel
}

func newGORMLogger(log *logrus.Logger) logger.Interface {
	return &gormLogger{logger: log, level: logger.Warn}
}

func (l *gormLogger) LogMode(level logger.LogLevel) logger.Interface {
	clone := *l
	clone.level = level
	return &clone
}

func (l *gormLogger) Info(ctx context.Context, message string, values ...any) {
	defer observability.StartSegment(ctx, "GORM.Logger.Info")()
	if l.level >= logger.Info {
		l.logger.WithContext(ctx).Infof(message, values...)
	}
}

func (l *gormLogger) Warn(ctx context.Context, message string, values ...any) {
	defer observability.StartSegment(ctx, "GORM.Logger.Warn")()
	if l.level >= logger.Warn {
		l.logger.WithContext(ctx).Warnf(message, values...)
	}
}

func (l *gormLogger) Error(ctx context.Context, message string, values ...any) {
	defer observability.StartSegment(ctx, "GORM.Logger.Error")()
	if l.level >= logger.Error {
		l.logger.WithContext(ctx).Errorf(message, values...)
	}
}

func (l *gormLogger) Trace(
	ctx context.Context,
	begin time.Time,
	_ func() (sql string, rowsAffected int64),
	err error,
) {
	defer observability.StartSegment(ctx, "GORM.Logger.Trace")()
	if l.level == logger.Silent {
		return
	}
	entry := l.logger.WithContext(ctx).WithFields(logrus.Fields{
		"component":   "gorm",
		"duration_ms": time.Since(begin).Milliseconds(),
	})
	switch {
	case err != nil && !errors.Is(err, gorm.ErrRecordNotFound):
		observability.NoticeError(ctx, err, "gorm query")
	case err != nil:
		entry.WithError(err).Debug("GORM record not found")
	case time.Since(begin) >= 500*time.Millisecond:
		entry.Warn("slow GORM query")
	case l.level >= logger.Info:
		entry.Debug("GORM query completed")
	}
}

type newRelicGORMPlugin struct{}

func (newRelicGORMPlugin) Name() string { return "newrelic" }

func (newRelicGORMPlugin) Initialize(db *gorm.DB) error {
	before := func(name string) func(*gorm.DB) {
		return func(tx *gorm.DB) {
			transaction := newrelic.FromContext(tx.Statement.Context)
			if transaction == nil {
				return
			}
			collection := tx.Statement.Table
			if collection == "" {
				collection = "unknown"
			}
			segment := &newrelic.DatastoreSegment{
				StartTime:  transaction.StartSegmentNow(),
				Product:    newrelic.DatastorePostgres,
				Collection: collection,
				Operation:  strings.ToUpper(name),
			}
			tx.InstanceSet(gormSegmentKey, segment)
		}
	}
	after := func(tx *gorm.DB) {
		if value, ok := tx.InstanceGet(gormSegmentKey); ok {
			if segment, valid := value.(*newrelic.DatastoreSegment); valid {
				segment.ParameterizedQuery = tx.Statement.SQL.String()
				segment.End()
			}
		}
	}

	registrations := []func() error{
		func() error {
			return db.Callback().Create().Before("*").Register("newrelic:before_create", before("create"))
		},
		func() error {
			return db.Callback().Create().After("*").Register("newrelic:after_create", after)
		},
		func() error {
			return db.Callback().Query().Before("*").Register("newrelic:before_query", before("query"))
		},
		func() error {
			return db.Callback().Query().After("*").Register("newrelic:after_query", after)
		},
		func() error {
			return db.Callback().Update().Before("*").Register("newrelic:before_update", before("update"))
		},
		func() error {
			return db.Callback().Update().After("*").Register("newrelic:after_update", after)
		},
		func() error {
			return db.Callback().Delete().Before("*").Register("newrelic:before_delete", before("delete"))
		},
		func() error {
			return db.Callback().Delete().After("*").Register("newrelic:after_delete", after)
		},
		func() error {
			return db.Callback().Row().Before("*").Register("newrelic:before_row", before("row"))
		},
		func() error {
			return db.Callback().Row().After("*").Register("newrelic:after_row", after)
		},
		func() error {
			return db.Callback().Raw().Before("*").Register("newrelic:before_raw", before("raw"))
		},
		func() error {
			return db.Callback().Raw().After("*").Register("newrelic:after_raw", after)
		},
	}
	for _, register := range registrations {
		if err := register(); err != nil {
			return err
		}
	}
	return nil
}
