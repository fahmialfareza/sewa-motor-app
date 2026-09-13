package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func validArguments() []string {
	return []string{"--username", " Initial.Owner ", "--full-name", " Initial Owner ", "--operator", "Release operator", "--reason", "First production account", "--confirm-production"}
}

func TestParseOptionsRequiresExplicitSafeProductionIdentity(t *testing.T) {
	opts, err := parseOptions(validArguments())
	if err != nil {
		t.Fatal(err)
	}
	if opts.Username != "initial.owner" || opts.FullName != "Initial Owner" || opts.TemporaryPassword != "" {
		t.Fatal("identity was not normalized or placeholder credential survived validation")
	}
	for _, args := range [][]string{
		nil,
		validArguments()[:8],
		append(validArguments(), "--confirm-production=false"),
		append(validArguments(), "--password", "secret-never-echo"),
		append(validArguments(), "secret-never-echo"),
		append(validArguments(), "--username", "invalid name"),
		append(validArguments(), "--operator", ""),
		append(validArguments(), "--reason", ""),
		append(validArguments(), "--full-name", ""),
	} {
		if _, err := parseOptions(args); err == nil || strings.Contains(err.Error(), "secret-never-echo") {
			t.Fatal("unsafe/missing options were accepted or a secret was echoed")
		}
	}
}

func TestReadPasswordAcceptsOneBoundedLineOnly(t *testing.T) {
	for _, password := range []string{"12345678", strings.Repeat("x", 256), " with significant spaces "} {
		for _, ending := range []string{"", "\n", "\r\n"} {
			got, err := readPassword(strings.NewReader(password + ending))
			if err != nil || got != password {
				t.Fatal("valid bounded password line rejected or altered")
			}
		}
	}
	for _, password := range []string{"", "short", strings.Repeat("x", 257), "        ", "12345678\n12345678", "12345678\n\n"} {
		if _, err := readPassword(strings.NewReader(password)); err == nil {
			t.Fatal("invalid stdin password accepted")
		}
	}
	if _, err := readPassword(failingReader{}); err == nil || strings.Contains(err.Error(), "secret-never-echo") {
		t.Fatal("reader failure was not safely handled")
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("secret-never-echo") }

func TestRunRequiresInjectedDatabaseAndNeverLoadsDotenv(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	var output bytes.Buffer
	err := run(validArguments(), failingReader{}, &output)
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL") || output.Len() != 0 {
		t.Fatal("missing database environment should fail before consuming a credential")
	}
}

func TestRunDoesNotEchoInvalidDatabaseCredentials(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://operator:secret-never-echo@host:invalid/database")
	var output bytes.Buffer
	err := run(validArguments(), strings.NewReader("temporary-password"), &output)
	if err == nil || strings.Contains(err.Error(), "secret-never-echo") || output.Len() != 0 {
		t.Fatal("invalid connection configuration leaked or was accepted")
	}
}
