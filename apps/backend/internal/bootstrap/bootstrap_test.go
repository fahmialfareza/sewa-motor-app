package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestManifestRequiresOneSuperadminAndSevenAdmins(t *testing.T) {
	var users []string
	for index := 0; index < 8; index++ {
		role := "admin"
		if index == 0 {
			role = "superadmin"
		}
		users = append(users, fmt.Sprintf(
			`{"fullName":"User %d","username":"user%d","role":"%s","temporaryPassword":"temporary-%02d"}`,
			index, index, role, index,
		))
	}
	body := []byte(`{"users":[` + strings.Join(users, ",") + `]}`)
	manifest, err := Parse(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Users) != 8 {
		t.Fatalf("got %d users", len(manifest.Users))
	}
}

func TestManifestRejectsSecondSuperadmin(t *testing.T) {
	var users []string
	for index := 0; index < 8; index++ {
		role := "admin"
		if index < 2 {
			role = "superadmin"
		}
		users = append(users, fmt.Sprintf(
			`{"fullName":"User %d","username":"user%d","role":"%s","temporaryPassword":"temporary-%02d"}`,
			index, index, role, index,
		))
	}
	_, err := Parse([]byte(`{"users":[` + strings.Join(users, ",") + `]}`))
	if err == nil {
		t.Fatal("expected manifest to reject two superadmins")
	}
}

func TestNewSampleSuperadminManifestNormalizesIdentity(t *testing.T) {
	manifest, err := NewSampleSuperadminManifest(
		"  Penyok  ",
		"  SuperAdmin  ",
		"temporary-password",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Users) != 1 {
		t.Fatalf("got %d users, want one", len(manifest.Users))
	}
	user := manifest.Users[0]
	if user.FullName != "Penyok" || user.Username != "superadmin" {
		t.Fatalf("sample identity was not normalized: %#v", user)
	}
	if user.Role != "superadmin" {
		t.Fatalf("sample role = %q", user.Role)
	}
}

func TestNewSampleSuperadminManifestRejectsWeakPassword(t *testing.T) {
	_, err := NewSampleSuperadminManifest("Penyok", "superadmin", "too-short")
	if err == nil {
		t.Fatal("expected weak sample password to be rejected")
	}
}

func TestApplyRemainsIdempotentForExistingSampleSuperadmin(t *testing.T) {
	t.Parallel()

	tx := &recordingSampleResetTx{
		rows: []pgx.Row{
			sampleIdentityRow{
				role:     domain.RoleSuperadmin,
				fullName: "Penyok",
			},
		},
	}
	hasher := &recordingPasswordHasher{hash: "must-not-be-used"}
	manifest, err := NewSampleSuperadminManifest(
		"Penyok",
		"superadmin",
		"a-different-password",
	)
	if err != nil {
		t.Fatal(err)
	}

	inserted, err := apply(
		context.Background(),
		func(_ context.Context, options pgx.TxOptions) (bootstrapTx, error) {
			if options.IsoLevel != pgx.ReadCommitted {
				t.Fatalf("isolation = %q, want read committed", options.IsoLevel)
			}
			return tx, nil
		},
		hasher,
		manifest,
	)
	if err != nil {
		t.Fatal(err)
	}

	if inserted != 0 {
		t.Fatalf("inserted = %d, want 0", inserted)
	}
	if hasher.password != "" {
		t.Fatal("ordinary idempotent apply hashed a replacement password")
	}
	if len(tx.queries) != 1 || len(tx.execs) != 1 || !strings.Contains(tx.execs[0].statement, "organization-account-administration") {
		t.Fatal("ordinary idempotent apply mutated the existing sample user")
	}
	if !tx.committed {
		t.Fatal("ordinary idempotent apply did not commit its read transaction")
	}
}

func TestApplyRefreshesActiveSpacesAfterSeparateGenerationLock(t *testing.T) {
	t.Parallel()

	tx := &recordingSampleResetTx{
		rows: []pgx.Row{sampleUserRow{err: pgx.ErrNoRows}},
	}
	hasher := &recordingPasswordHasher{hash: "encoded-temporary-password"}
	manifest, err := NewSampleSuperadminManifest(
		"Penyok",
		"superadmin",
		"superadmin123",
	)
	if err != nil {
		t.Fatal(err)
	}

	inserted, err := apply(
		context.Background(),
		func(_ context.Context, options pgx.TxOptions) (bootstrapTx, error) {
			if options.IsoLevel != pgx.ReadCommitted {
				t.Fatalf("isolation = %q, want read committed", options.IsoLevel)
			}
			return tx, nil
		},
		hasher,
		manifest,
	)
	if err != nil {
		t.Fatal(err)
	}
	if inserted != 1 || !tx.committed {
		t.Fatalf("inserted = %d, committed = %v", inserted, tx.committed)
	}
	if len(tx.execs) != 7 {
		t.Fatalf("exec count = %d, want organization lock, account, membership, audit, generation lock, sync, and organization audit", len(tx.execs))
	}
	if !strings.Contains(tx.execs[0].statement, "organization-account-administration") {
		t.Fatal("organization lock missing before account creation")
	}
	if !strings.Contains(tx.execs[2].statement, "INSERT INTO tenant_memberships") {
		t.Fatal("initial tenant membership missing")
	}
	if !strings.Contains(tx.execs[4].statement, "pg_advisory_xact_lock_shared") || !strings.Contains(tx.execs[4].statement, "ORDER BY id") {
		t.Fatal("Sandbox generation lock was not acquired before sync fanout")
	}
	if strings.Contains(tx.execs[5].statement, "pg_advisory_xact_lock_shared") ||
		!strings.Contains(tx.execs[5].statement, "FROM data_spaces") || strings.Contains(tx.execs[5].statement, "ds.tenant_id=$3") {
		t.Fatal("sync fanout did not use a fresh statement snapshot after the generation lock")
	}
}

func TestResetSampleSuperadminPasswordIsExplicitAndTransactional(t *testing.T) {
	t.Parallel()

	userID := uuid.MustParse("10000000-0000-4000-8000-000000000001")
	createdAt := time.Date(2026, time.July, 1, 8, 0, 0, 0, time.UTC)
	before := domain.User{
		ID: userID, FullName: "Penyok", Username: "superadmin",
		Role: domain.RoleSuperadmin, IsActive: true, MustChangePassword: false,
		CreatedAt: createdAt, UpdatedAt: createdAt,
	}
	after := before
	after.MustChangePassword = true
	after.UpdatedAt = createdAt.Add(time.Hour)

	tx := &recordingSampleResetTx{
		rows: []pgx.Row{
			sampleUserRow{user: before},
			sampleUserRow{user: after},
		},
	}
	hasher := &recordingPasswordHasher{hash: "encoded-new-password"}
	manifest, err := NewSampleSuperadminManifest(
		"Penyok",
		"superadmin",
		"superadmin123",
	)
	if err != nil {
		t.Fatal(err)
	}

	err = resetSampleSuperadminPassword(
		context.Background(),
		func(_ context.Context, options pgx.TxOptions) (bootstrapTx, error) {
			if options.IsoLevel != pgx.ReadCommitted {
				t.Fatalf("isolation = %q, want read committed", options.IsoLevel)
			}
			return tx, nil
		},
		hasher,
		manifest,
	)
	if err != nil {
		t.Fatal(err)
	}

	if hasher.password != "superadmin123" {
		t.Fatalf("hashed password = %q", hasher.password)
	}
	if !tx.committed {
		t.Fatal("password reset transaction was not committed")
	}
	if len(tx.queries) != 2 {
		t.Fatalf("query count = %d, want 2", len(tx.queries))
	}
	if !strings.Contains(tx.queries[0].statement, "FOR UPDATE") {
		t.Fatal("sample identity was not locked before reset")
	}
	if got := tx.queries[1].args[1]; got != "encoded-new-password" {
		t.Fatalf("password update received %#v", got)
	}
	if len(tx.execs) != 6 {
		t.Fatalf("exec count = %d, want organization lock, session, audit, generation lock, sync and organization audit writes", len(tx.execs))
	}
	if !strings.Contains(tx.execs[0].statement, "organization-account-administration") {
		t.Fatal("organization lock missing before account recovery")
	}
	if !strings.Contains(tx.execs[1].statement, "revoked_reason = 'password_recovered'") {
		t.Fatal("active sessions were not revoked with the quarantine-compatible recovery reason")
	}
	if !strings.Contains(tx.execs[2].statement, "'user.password_reset'") {
		t.Fatal("password reset audit event was not appended")
	}
	if !strings.Contains(tx.execs[3].statement, "pg_advisory_xact_lock_shared") || !strings.Contains(tx.execs[3].statement, "ORDER BY id") {
		t.Fatal("Sandbox generation lock was not acquired before password reset fanout")
	}
	if strings.Contains(tx.execs[4].statement, "pg_advisory_xact_lock_shared") ||
		!strings.Contains(tx.execs[4].statement, "'updated'") || strings.Contains(tx.execs[4].statement, "ds.tenant_id=$3") {
		t.Fatal("password reset sync change was not appended")
	}

	var auditAfter domain.User
	if err := json.Unmarshal(tx.execs[2].args[2].([]byte), &auditAfter); err != nil {
		t.Fatalf("decode audit after-values: %v", err)
	}
	if !auditAfter.MustChangePassword {
		t.Fatal("audit after-values do not force a password change")
	}
	var metadata map[string]any
	if err := json.Unmarshal(tx.execs[2].args[3].([]byte), &metadata); err != nil {
		t.Fatalf("decode audit metadata: %v", err)
	}
	if metadata["command"] != "seed-superadmin" || metadata["explicit"] != true {
		t.Fatalf("audit metadata = %#v", metadata)
	}
	var syncUser domain.User
	if err := json.Unmarshal(tx.execs[4].args[1].([]byte), &syncUser); err != nil {
		t.Fatalf("decode sync payload: %v", err)
	}
	if syncUser.ID != userID || !syncUser.MustChangePassword {
		t.Fatalf("sync payload = %#v", syncUser)
	}

	for _, call := range tx.execs {
		for _, argument := range call.args {
			body, _ := json.Marshal(argument)
			if strings.Contains(string(body), "superadmin123") ||
				strings.Contains(string(body), "encoded-new-password") {
				t.Fatal("password material leaked into audit, sync, or SQL payloads")
			}
		}
	}
}

func TestResetSampleSuperadminPasswordRollsBackOnLateFailure(t *testing.T) {
	t.Parallel()

	userID := uuid.MustParse("10000000-0000-4000-8000-000000000002")
	before := domain.User{
		ID: userID, FullName: "Penyok", Username: "superadmin",
		Role: domain.RoleSuperadmin, IsActive: true, MustChangePassword: false,
	}
	after := before
	after.MustChangePassword = true
	tx := &recordingSampleResetTx{
		rows:       []pgx.Row{sampleUserRow{user: before}, sampleUserRow{user: after}},
		failExecAt: 5,
	}
	hasher := &recordingPasswordHasher{hash: "encoded-new-password"}
	manifest, err := NewSampleSuperadminManifest(
		"Penyok",
		"superadmin",
		"superadmin123",
	)
	if err != nil {
		t.Fatal(err)
	}

	err = resetSampleSuperadminPassword(
		context.Background(),
		func(context.Context, pgx.TxOptions) (bootstrapTx, error) {
			return tx, nil
		},
		hasher,
		manifest,
	)

	if err == nil || !strings.Contains(err.Error(), "sync sample superadmin") {
		t.Fatalf("error = %v, want late sync failure", err)
	}
	if tx.committed {
		t.Fatal("partially completed password reset was committed")
	}
	if !tx.rolledBack {
		t.Fatal("partially completed password reset was not rolled back")
	}
	if len(tx.execs) != 5 {
		t.Fatalf("exec count before rollback = %d, want 5", len(tx.execs))
	}
}

func TestResetSampleSuperadminPasswordRejectsIdentityMismatchBeforeHashing(t *testing.T) {
	t.Parallel()

	tx := &recordingSampleResetTx{
		rows: []pgx.Row{
			sampleUserRow{user: domain.User{
				ID: uuid.New(), FullName: "Different Person", Username: "superadmin",
				Role: domain.RoleSuperadmin, IsActive: true,
			}},
		},
	}
	hasher := &recordingPasswordHasher{hash: "should-not-be-used"}
	manifest, err := NewSampleSuperadminManifest(
		"Penyok",
		"superadmin",
		"superadmin123",
	)
	if err != nil {
		t.Fatal(err)
	}

	err = resetSampleSuperadminPassword(
		context.Background(),
		func(context.Context, pgx.TxOptions) (bootstrapTx, error) {
			return tx, nil
		},
		hasher,
		manifest,
	)

	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("error = %v, want identity mismatch", err)
	}
	if hasher.password != "" {
		t.Fatal("identity mismatch was hashed before it was rejected")
	}
	if tx.committed || len(tx.execs) != 1 || !strings.Contains(tx.execs[0].statement, "organization-account-administration") || len(tx.queries) != 1 {
		t.Fatal("identity mismatch mutated the transaction")
	}
	if !tx.rolledBack {
		t.Fatal("identity mismatch did not roll back")
	}
}

type recordingPasswordHasher struct {
	password string
	hash     string
}

func (h *recordingPasswordHasher) Hash(password string) (string, error) {
	h.password = password
	return h.hash, nil
}

func (*recordingPasswordHasher) Verify(string, string) (bool, error) {
	return false, errors.New("not implemented")
}

type recordedSampleResetCall struct {
	statement string
	args      []any
}

type recordingSampleResetTx struct {
	rows       []pgx.Row
	queries    []recordedSampleResetCall
	execs      []recordedSampleResetCall
	committed  bool
	rolledBack bool
	failExecAt int
}

func (tx *recordingSampleResetTx) QueryRow(
	_ context.Context,
	statement string,
	args ...any,
) pgx.Row {
	tx.queries = append(tx.queries, recordedSampleResetCall{statement: statement, args: args})
	if len(tx.rows) == 0 {
		return sampleUserRow{err: errors.New("unexpected query")}
	}
	row := tx.rows[0]
	tx.rows = tx.rows[1:]
	return row
}

func (tx *recordingSampleResetTx) Exec(
	_ context.Context,
	statement string,
	args ...any,
) (pgconn.CommandTag, error) {
	tx.execs = append(tx.execs, recordedSampleResetCall{statement: statement, args: args})
	if tx.failExecAt == len(tx.execs) {
		return pgconn.CommandTag{}, errors.New("injected exec failure")
	}
	return pgconn.NewCommandTag("UPDATE 1"), nil
}

func (tx *recordingSampleResetTx) Commit(context.Context) error {
	tx.committed = true
	return nil
}

func (tx *recordingSampleResetTx) Rollback(context.Context) error {
	tx.rolledBack = true
	return nil
}

type sampleUserRow struct {
	user domain.User
	err  error
}

func (row sampleUserRow) Scan(dest ...any) error {
	if row.err != nil {
		return row.err
	}
	if len(dest) != 9 {
		return fmt.Errorf("scan destination count = %d, want 9", len(dest))
	}
	*dest[0].(*uuid.UUID) = row.user.ID
	*dest[1].(*string) = row.user.FullName
	*dest[2].(*string) = row.user.Username
	*dest[3].(*domain.Role) = row.user.Role
	*dest[4].(*bool) = row.user.IsActive
	*dest[5].(*bool) = row.user.MustChangePassword
	*dest[6].(*time.Time) = row.user.CreatedAt
	*dest[7].(*time.Time) = row.user.UpdatedAt
	*dest[8].(**time.Time) = row.user.DeletedAt
	return nil
}

type sampleIdentityRow struct {
	role     domain.Role
	fullName string
}

func (row sampleIdentityRow) Scan(dest ...any) error {
	if len(dest) != 2 {
		return fmt.Errorf("scan destination count = %d, want 2", len(dest))
	}
	*dest[0].(*domain.Role) = row.role
	*dest[1].(*string) = row.fullName
	return nil
}
