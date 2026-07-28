package observability

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	contextlogrus "github.com/newrelic/go-agent/v3/integrations/logcontext-v2/nrlogrus"
	agentlogrus "github.com/newrelic/go-agent/v3/integrations/nrlogrus"
	"github.com/newrelic/go-agent/v3/newrelic"
	"github.com/sirupsen/logrus"
)

type Config struct {
	Enabled            bool
	AppName            string
	LicenseKey         string
	DistributedTracing bool
	LogForwarding      bool
	LogLevel           string
}

type Runtime struct {
	App     *newrelic.Application
	Logger  *logrus.Logger
	enabled bool
}

var defaultLogger = newBaseLogger("info")

func New(config Config) (*Runtime, error) {
	logger := newBaseLogger(config.LogLevel)
	app, err := newrelic.NewApplication(
		newrelic.ConfigEnabled(config.Enabled),
		newrelic.ConfigAppName(config.AppName),
		newrelic.ConfigLicense(config.LicenseKey),
		newrelic.ConfigDistributedTracerEnabled(config.DistributedTracing),
		newrelic.ConfigAppLogEnabled(true),
		newrelic.ConfigAppLogForwardingEnabled(config.LogForwarding),
		newrelic.ConfigAppLogDecoratingEnabled(!config.LogForwarding),
		agentlogrus.ConfigLogger(logger),
	)
	if err != nil {
		return nil, fmt.Errorf("initialize New Relic: %w", err)
	}

	logger.SetFormatter(contextlogrus.NewFormatter(app, &logrus.JSONFormatter{
		TimestampFormat: time.RFC3339Nano,
	}))
	defaultLogger = logger
	return &Runtime{App: app, Logger: logger, enabled: config.Enabled}, nil
}

func (r *Runtime) WaitForConnection(timeout time.Duration) error {
	if r == nil || !r.enabled {
		return nil
	}
	return r.App.WaitForConnection(timeout)
}

func (r *Runtime) Shutdown(timeout time.Duration) {
	if r == nil || r.App == nil {
		return
	}
	r.App.Shutdown(timeout)
}

func Logger() *logrus.Logger {
	return defaultLogger
}

func StartSegment(ctx context.Context, name string) func() {
	transaction := newrelic.FromContext(ctx)
	if transaction == nil {
		return func() {}
	}
	segment := transaction.StartSegment(name)
	return segment.End
}

func NoticeError(ctx context.Context, err error, operation string) {
	if err == nil {
		return
	}
	attributes := map[string]any{"operation": operation}
	if transaction := newrelic.FromContext(ctx); transaction != nil {
		transaction.NoticeError(newrelic.Error{
			Message:    err.Error(),
			Class:      fmt.Sprintf("%T", err),
			Attributes: attributes,
			Stack:      newrelic.NewStackTrace(),
		})
	}
	Logger().
		WithContext(ctx).
		WithError(err).
		WithField("operation", operation).
		Error("operation failed")
}

func newBaseLogger(level string) *logrus.Logger {
	logger := logrus.New()
	logger.SetOutput(os.Stdout)
	logger.SetFormatter(&logrus.JSONFormatter{
		TimestampFormat: time.RFC3339Nano,
	})
	parsed, err := logrus.ParseLevel(strings.ToLower(strings.TrimSpace(level)))
	if err != nil {
		parsed = logrus.InfoLevel
	}
	logger.SetLevel(parsed)
	return logger
}
