package observability

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestDisabledRuntimeStillProvidesStructuredLogger(t *testing.T) {
	runtime, err := New(Config{
		Enabled:       false,
		AppName:       "sewa-motor-test",
		LogLevel:      "debug",
		LogForwarding: true,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer runtime.Shutdown(time.Second)

	var output bytes.Buffer
	runtime.Logger.SetOutput(&output)
	ctx := WithDataScope(context.Background(), DataScope{
		Mode: "sandbox", SpaceID: "11111111-1111-4111-8111-111111111111", Generation: 7,
	})
	defer StartSegment(ctx, "test.segment")()
	NoticeError(ctx, errors.New("test failure"), "test.operation")

	log := output.String()
	if !strings.Contains(log, `"operation":"test.operation"`) {
		t.Fatalf("log does not contain operation: %s", log)
	}
	if !strings.Contains(log, `"error":"test failure"`) {
		t.Fatalf("log does not contain error: %s", log)
	}
	for _, field := range []string{
		`"data.mode":"sandbox"`,
		`"data.space_id":"11111111-1111-4111-8111-111111111111"`,
		`"data.generation":7`,
	} {
		if !strings.Contains(log, field) {
			t.Fatalf("log does not contain %s: %s", field, log)
		}
	}
}

func TestDataScopeFromContextRejectsUnscopedContext(t *testing.T) {
	if _, ok := DataScopeFromContext(context.Background()); ok {
		t.Fatal("background context unexpectedly contained a data scope")
	}
}

func TestLoggerHookAddsAuthorizedDataScopeToEveryContextLog(t *testing.T) {
	runtime, err := New(Config{Enabled: false, AppName: "scope-log-test"})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Shutdown(time.Second)

	var output bytes.Buffer
	runtime.Logger.SetOutput(&output)
	ctx := WithDataScope(context.Background(), DataScope{
		Mode: "production", SpaceID: "00000000-0000-4000-8000-000000000100", Generation: 1,
	})
	runtime.Logger.WithContext(ctx).Info("scoped log")

	log := output.String()
	for _, field := range []string{
		`"data.mode":"production"`,
		`"data.space_id":"00000000-0000-4000-8000-000000000100"`,
		`"data.generation":1`,
	} {
		if !strings.Contains(log, field) {
			t.Fatalf("log does not contain %s: %s", field, log)
		}
	}
}
