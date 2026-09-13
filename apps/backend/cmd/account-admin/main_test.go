package main

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestParseOptionsRequiresExplicitAuditedTarget(t *testing.T) {
	valid := []string{"--username", "  Owner  ", "--action", "grant-superadmin", "--operator", "on-call@example.test", "--reason", "Approved organization Superadmin"}
	got, err := parseOptions(valid)
	if err != nil || got.username != "owner" || got.operator != "on-call@example.test" {
		t.Fatalf("parse explicit target = %+v, %v", got, err)
	}
	for _, args := range [][]string{
		nil,
		{"--username", "owner", "--action", "grant-superadmin", "--reason", "Approved"},
		{"--username", "owner", "--action", "grant-superadmin", "--operator", "operator"},
		append(append([]string{}, valid...), "unexpected"),
		append(append([]string{}, valid...), "--password", "do-not-echo-this"),
		{"--username", "owner", "--action", "delete-tenant", "--operator", "operator", "--reason", "Approved"},
	} {
		if _, err := parseOptions(args); err == nil {
			t.Fatalf("accepted invalid options: %v", args)
		} else if strings.Contains(err.Error(), "do-not-echo-this") {
			t.Fatal("argument parser exposed a supplied credential")
		}
	}
}

func TestRetiredPlatformActionsFailBeforeReadingCredentials(t *testing.T) {
	for _, action := range []string{"grant-platform", "revoke-platform"} {
		args := []string{"--username", "owner", "--action", action, "--operator", "operator", "--reason", "Approved"}
		var output bytes.Buffer
		err := run(args, panicReader{}, &output)
		if err == nil || !strings.Contains(err.Error(), "platform permission has been removed") || output.Len() != 0 {
			t.Fatalf("retired action did not fail closed: %v", err)
		}
	}
}

func TestReadPasswordPreservesOneLineAndBounds(t *testing.T) {
	for _, input := range []string{"password123", "password123\n", "password123\r\n"} {
		got, err := readPassword(strings.NewReader(input))
		if got != "password123" || err != nil {
			t.Fatalf("password line parsing failed: %v", err)
		}
	}
	for _, input := range []string{"", "short", "        ", "password123\nextra", strings.Repeat("x", 257), strings.Repeat("x", 256) + "\nextra"} {
		if _, err := readPassword(strings.NewReader(input)); err == nil {
			t.Fatal("accepted invalid password input")
		}
	}
	if _, err := readPassword(failingReader{}); err == nil {
		t.Fatal("ignored password input failure")
	}
}

func TestInvalidCommandDoesNotReadStdinOrConnect(t *testing.T) {
	var output bytes.Buffer
	if err := run([]string{"--action", "reset-password"}, panicReader{}, &output); err == nil {
		t.Fatal("accepted implicit target")
	}
	if output.Len() != 0 {
		t.Fatal("invalid command emitted a success message")
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("private reader failure") }

type panicReader struct{}

func (panicReader) Read([]byte) (int, error) { panic("must not read stdin before validation") }

var _ io.Reader = failingReader{}
