package config

import (
	"strings"
	"testing"
)

func setRequiredTestEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/test")
	t.Setenv("AUTO_MIGRATE", "")
	t.Setenv("SHUTDOWN_TIMEOUT", "")
	t.Setenv("SESSION_CACHE_TTL", "")
	t.Setenv("LOGIN_RATE_LIMIT", "")
	t.Setenv("LOGIN_RATE_WINDOW", "")
	t.Setenv("TRUSTED_PROXIES", "")
	t.Setenv("NEW_RELIC_APP_NAME", "")
	t.Setenv("NEW_RELIC_DISTRIBUTED_TRACING_ENABLED", "")
	t.Setenv("NEW_RELIC_LOG_FORWARDING_ENABLED", "")
}

func TestLoadDisablesNewRelicWithoutLicenseByDefault(t *testing.T) {
	setRequiredTestEnvironment(t)
	t.Setenv("NEW_RELIC_ENABLED", "")
	t.Setenv("NEW_RELIC_LICENSE_KEY", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.NewRelicEnabled {
		t.Fatal("New Relic should be disabled when no license key is configured")
	}
	if cfg.NewRelicAppName != "sewa-motor-backend" {
		t.Fatalf("NewRelicAppName = %q", cfg.NewRelicAppName)
	}
}

func TestLoadRequiresLicenseWhenNewRelicEnabled(t *testing.T) {
	setRequiredTestEnvironment(t)
	t.Setenv("NEW_RELIC_ENABLED", "true")
	t.Setenv("NEW_RELIC_LICENSE_KEY", "")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "NEW_RELIC_LICENSE_KEY") {
		t.Fatalf("Load() error = %v, want missing New Relic license error", err)
	}
}

func TestLoadEnablesNewRelicWhenLicenseIsPresent(t *testing.T) {
	setRequiredTestEnvironment(t)
	t.Setenv("NEW_RELIC_ENABLED", "")
	t.Setenv("NEW_RELIC_LICENSE_KEY", "test-license")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !cfg.NewRelicEnabled {
		t.Fatal("New Relic should default to enabled when a license key is configured")
	}
	if !cfg.NewRelicDistributedTracing || !cfg.NewRelicLogForwarding {
		t.Fatal("New Relic tracing and log forwarding should default to enabled")
	}
}
