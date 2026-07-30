package main

import (
	"testing"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/bootstrap"
)

func TestDevelopmentPasswordUsesSimpleFallback(t *testing.T) {
	t.Setenv("DEV_SUPERADMIN_PASSWORD", "")

	password, usingDefault := developmentPassword()

	if password != "superadmin123" {
		t.Fatalf("password = %q, want development fallback", password)
	}
	if !usingDefault {
		t.Fatal("expected fallback password to be reported as the default")
	}
	if _, err := bootstrap.NewSampleSuperadminManifest(
		"Penyok",
		"superadmin",
		password,
	); err != nil {
		t.Fatalf("development fallback must satisfy password validation: %v", err)
	}
}

func TestDevelopmentPasswordPreservesEnvironmentOverride(t *testing.T) {
	t.Setenv("DEV_SUPERADMIN_PASSWORD", "  another-temporary-password  ")

	password, usingDefault := developmentPassword()

	if password != "another-temporary-password" {
		t.Fatalf("password = %q, want trimmed environment value", password)
	}
	if usingDefault {
		t.Fatal("environment override was reported as the default")
	}
}

func TestParseOptionsRequiresExplicitResetFlag(t *testing.T) {
	withoutReset, err := parseOptions(nil)
	if err != nil {
		t.Fatal(err)
	}
	if withoutReset.resetPassword {
		t.Fatal("ordinary seed unexpectedly enables password reset")
	}

	withReset, err := parseOptions([]string{"--reset-password"})
	if err != nil {
		t.Fatal(err)
	}
	if !withReset.resetPassword {
		t.Fatal("explicit reset flag was not enabled")
	}
}

func TestParseOptionsRejectsUnexpectedArguments(t *testing.T) {
	if _, err := parseOptions([]string{"reset-password"}); err == nil {
		t.Fatal("expected positional reset argument to be rejected")
	}
}
