package bootstrap

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
	"github.com/jackc/pgx/v5"
)

func initialInput() InitialSuperadminInput {
	return InitialSuperadminInput{Username: "initial.owner", FullName: "Initial Owner", TemporaryPassword: "temporary-password", Operator: "Test operator", Reason: "Initial setup"}
}

func TestInitialSuperadminValidationDoesNotWeakenManifestValidation(t *testing.T) {
	input := initialInput()
	input.Username, input.FullName, input.TemporaryPassword = " Initial.Owner ", " Initial Owner ", "12345678"
	got, err := input.NormalizeAndValidate()
	if err != nil || got.Username != "initial.owner" || got.FullName != "Initial Owner" {
		t.Fatal("initial operator identity normalization failed")
	}
	if _, err = NewSampleSuperadminManifest(input.FullName, input.Username, input.TemporaryPassword); err == nil {
		t.Fatal("the existing bootstrap password policy changed")
	}
	for _, update := range []func(*InitialSuperadminInput){
		func(v *InitialSuperadminInput) { v.Username = "bad name" },
		func(v *InitialSuperadminInput) { v.FullName = "" },
		func(v *InitialSuperadminInput) { v.Operator = "" },
		func(v *InitialSuperadminInput) { v.Reason = "" },
		func(v *InitialSuperadminInput) { v.TemporaryPassword = "short" },
		func(v *InitialSuperadminInput) { v.TemporaryPassword = strings.Repeat("x", 257) },
	} {
		input := initialInput()
		update(&input)
		if _, err := input.NormalizeAndValidate(); err == nil {
			t.Fatal("invalid initial account accepted")
		}
	}
}

func TestInitialSuperadminNoOpNeverHashesOrMutatesCredentials(t *testing.T) {
	tx := &recordingSampleResetTx{rows: []pgx.Row{initialBoolRow(true), initialIdentityRow{name: "Initial Owner", role: domain.RoleSuperadmin, active: true}}}
	hasher := &recordingPasswordHasher{}
	created, err := applyInitialSuperadmin(context.Background(), initialBegin(t, tx), hasher, initialInput())
	if err != nil || created || !tx.committed || hasher.password != "" || len(tx.execs) != 1 {
		t.Fatalf("matching account was not an immutable no-op: created=%v committed=%v error=%v", created, tx.committed, err)
	}
}

func TestInitialSuperadminRefusesExistingAuthorityOrIdentityConflicts(t *testing.T) {
	for _, row := range []initialIdentityRow{
		{name: "Initial Owner", role: domain.RoleAdmin, active: true},
		{name: "Initial Owner", role: domain.RoleSuperadmin, active: false},
		{name: "Different Name", role: domain.RoleSuperadmin, active: true},
	} {
		tx := &recordingSampleResetTx{rows: []pgx.Row{initialBoolRow(true), row}}
		if created, err := applyInitialSuperadmin(context.Background(), initialBegin(t, tx), &recordingPasswordHasher{}, initialInput()); err == nil || created || tx.committed || len(tx.execs) != 1 {
			t.Fatal("conflicting existing account was altered")
		}
	}
	tx := &recordingSampleResetTx{rows: []pgx.Row{initialBoolRow(true), initialIdentityRow{err: pgx.ErrNoRows}, initialBoolRow(true)}}
	if created, err := applyInitialSuperadmin(context.Background(), initialBegin(t, tx), &recordingPasswordHasher{}, initialInput()); err == nil || created || tx.committed || len(tx.execs) != 1 {
		t.Fatal("additional initial Superadmin was allowed")
	}
}

func TestInitialSuperadminRequiresMigrationAndRollsBackLateFailure(t *testing.T) {
	tx := &recordingSampleResetTx{rows: []pgx.Row{initialBoolRow(false)}}
	if _, err := applyInitialSuperadmin(context.Background(), initialBegin(t, tx), &recordingPasswordHasher{}, initialInput()); err == nil || tx.committed || len(tx.execs) != 1 {
		t.Fatal("initial command attempted to migrate or accepted an incompatible schema")
	}
	tx = &recordingSampleResetTx{rows: []pgx.Row{initialBoolRow(true), initialIdentityRow{err: pgx.ErrNoRows}, initialBoolRow(false)}, failExecAt: 7}
	if created, err := applyInitialSuperadmin(context.Background(), initialBegin(t, tx), &recordingPasswordHasher{hash: "fixture-hash"}, initialInput()); err == nil || created || tx.committed || !tx.rolledBack {
		t.Fatal("late audit failure did not roll back the new account")
	}
}

func initialBegin(t *testing.T, tx bootstrapTx) beginBootstrapTx {
	t.Helper()
	return func(_ context.Context, opts pgx.TxOptions) (bootstrapTx, error) {
		if opts.IsoLevel != pgx.ReadCommitted {
			t.Fatal("bootstrap needs fresh snapshots after generation locks")
		}
		return tx, nil
	}
}

type initialBoolRow bool

func (row initialBoolRow) Scan(dest ...any) error {
	if len(dest) != 1 {
		return errors.New("wrong boolean scan arity")
	}
	*dest[0].(*bool) = bool(row)
	return nil
}

type initialIdentityRow struct {
	name   string
	role   domain.Role
	active bool
	err    error
}

func (row initialIdentityRow) Scan(dest ...any) error {
	if row.err != nil {
		return row.err
	}
	if len(dest) != 3 {
		return errors.New("wrong identity scan arity")
	}
	*dest[0].(*string), *dest[1].(*domain.Role), *dest[2].(*bool) = row.name, row.role, row.active
	return nil
}
