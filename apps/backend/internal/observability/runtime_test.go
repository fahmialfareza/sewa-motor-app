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
	ctx := context.Background()
	defer StartSegment(ctx, "test.segment")()
	NoticeError(ctx, errors.New("test failure"), "test.operation")

	log := output.String()
	if !strings.Contains(log, `"operation":"test.operation"`) {
		t.Fatalf("log does not contain operation: %s", log)
	}
	if !strings.Contains(log, `"error":"test failure"`) {
		t.Fatalf("log does not contain error: %s", log)
	}
}
