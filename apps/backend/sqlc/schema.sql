-- Code-generation schema snapshot for sqlc.
-- Runtime schema changes are versioned Go migrations in ../migrations and run through GORM.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    full_name text NOT NULL CHECK (length(btrim(full_name)) BETWEEN 1 AND 160),
    username text NOT NULL UNIQUE
        CHECK (username = lower(username))
        CHECK (username ~ '^[a-z0-9][a-z0-9._-]{2,63}$'),
    password_hash text NOT NULL,
    role text NOT NULL CHECK (role IN ('admin', 'superadmin')),
    is_active boolean NOT NULL DEFAULT true,
    must_change_password boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    CHECK (deleted_at IS NULL OR NOT is_active)
);

CREATE INDEX users_active_role_idx ON users (role)
    WHERE is_active AND deleted_at IS NULL;

CREATE TABLE terminals (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    installation_id text NOT NULL UNIQUE CHECK (length(installation_id) BETWEEN 8 AND 200),
    name text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 120),
    public_key bytea NOT NULL CHECK (octet_length(public_key) = 32),
    platform text NOT NULL DEFAULT 'android' CHECK (platform = 'android'),
    device_model text,
    os_version text,
    app_version text,
    is_active boolean NOT NULL DEFAULT true,
    enrolled_by uuid NOT NULL REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz,
    CHECK (revoked_at IS NULL OR NOT is_active)
);

CREATE TABLE sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id),
    terminal_id uuid REFERENCES terminals(id),
    token_hash bytea NOT NULL UNIQUE CHECK (octet_length(token_hash) = 32),
    created_at timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz,
    revoked_reason text,
    CHECK ((revoked_at IS NULL AND revoked_reason IS NULL) OR
           (revoked_at IS NOT NULL AND length(btrim(revoked_reason)) > 0))
);

CREATE INDEX sessions_user_live_idx ON sessions (user_id) WHERE revoked_at IS NULL;

CREATE TABLE packages (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code text NOT NULL UNIQUE CHECK (code ~ '^[A-Z0-9][A-Z0-9_-]{1,31}$'),
    current_revision integer NOT NULL DEFAULT 1 CHECK (current_revision > 0),
    created_by uuid REFERENCES users(id),
    updated_by uuid REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    deleted_by uuid REFERENCES users(id)
);

CREATE TABLE package_revisions (
    package_id uuid NOT NULL REFERENCES packages(id),
    revision integer NOT NULL CHECK (revision > 0),
    name text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 120),
    description text NOT NULL DEFAULT '' CHECK (length(description) <= 1000),
    unit_price bigint NOT NULL CHECK (unit_price > 0),
    change_reason text,
    created_by uuid REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (package_id, revision)
);

ALTER TABLE packages ADD CONSTRAINT packages_current_revision_fk
    FOREIGN KEY (id, current_revision)
    REFERENCES package_revisions(package_id, revision)
    DEFERRABLE INITIALLY DEFERRED;

CREATE TABLE transactions (
    id text PRIMARY KEY
        CHECK (id ~ '^[0-9A-HJKMNP-TV-Z]{26}$'),
    current_revision integer NOT NULL DEFAULT 1 CHECK (current_revision > 0),
    occurred_at timestamptz NOT NULL,
    server_received_at timestamptz NOT NULL DEFAULT now(),
    origin_actor_id uuid NOT NULL REFERENCES users(id),
    origin_session_id uuid NOT NULL REFERENCES sessions(id),
    terminal_id uuid REFERENCES terminals(id),
    updated_by uuid NOT NULL REFERENCES users(id),
    subtotal bigint NOT NULL CHECK (subtotal >= 0),
    total bigint NOT NULL CHECK (total >= 0 AND total = subtotal),
    payment_method text NOT NULL DEFAULT 'legacy'
        CHECK (payment_method IN ('cash', 'qris', 'legacy')),
    qris_payload_hash text,
    payment_status text NOT NULL DEFAULT 'pending'
        CHECK (payment_status IN ('pending', 'success', 'failed')),
    payment_confirmed_revision integer,
    print_state text NOT NULL DEFAULT 'pending'
        CHECK (print_state IN ('pending', 'success', 'failed', 'unknown', 'needs-reprint')),
    latest_printed_revision integer,
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    deleted_by uuid REFERENCES users(id),
    delete_reason text,
    CHECK ((payment_status = 'success' AND payment_confirmed_revision = current_revision) OR
           (payment_status IN ('pending', 'failed') AND payment_confirmed_revision IS NULL)),
    CHECK (qris_payload_hash IS NULL OR
           (payment_method = 'qris' AND qris_payload_hash ~ '^[0-9a-f]{64}$')),
    CHECK ((deleted_at IS NULL AND deleted_by IS NULL AND delete_reason IS NULL) OR
           (deleted_at IS NOT NULL AND deleted_by IS NOT NULL AND length(btrim(delete_reason)) > 0))
);

CREATE INDEX transactions_occurred_idx ON transactions (occurred_at DESC, id DESC);
CREATE INDEX transactions_origin_actor_idx ON transactions (origin_actor_id, occurred_at DESC);
CREATE INDEX transactions_live_idx ON transactions (occurred_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX transactions_paid_occurred_idx ON transactions (payment_status, occurred_at DESC)
    WHERE deleted_at IS NULL AND payment_status = 'success';

CREATE TABLE transaction_revisions (
    transaction_id text NOT NULL REFERENCES transactions(id),
    revision integer NOT NULL CHECK (revision > 0),
    base_revision integer CHECK (base_revision IS NULL OR base_revision > 0),
    change_type text NOT NULL CHECK (change_type IN ('create', 'correction')),
    reason text,
    before_snapshot jsonb,
    after_snapshot jsonb NOT NULL,
    qris_payload_hash text,
    origin_actor_id uuid NOT NULL REFERENCES users(id),
    origin_session_id uuid NOT NULL REFERENCES sessions(id),
    terminal_id uuid REFERENCES terminals(id),
    submitted_by_actor_id uuid NOT NULL REFERENCES users(id),
    submitted_by_session_id uuid NOT NULL REFERENCES sessions(id),
    client_occurred_at timestamptz NOT NULL,
    server_received_at timestamptz NOT NULL DEFAULT now(),
    CHECK (qris_payload_hash IS NULL OR
           ((after_snapshot ->> 'paymentMethod') = 'qris' AND
            qris_payload_hash ~ '^[0-9a-f]{64}$')),
    PRIMARY KEY (transaction_id, revision),
    CHECK ((change_type = 'create' AND revision = 1 AND base_revision IS NULL AND reason IS NULL AND before_snapshot IS NULL)
        OR (change_type = 'correction' AND revision > 1 AND base_revision = revision - 1
            AND length(btrim(reason)) >= 5 AND before_snapshot IS NOT NULL))
);

ALTER TABLE transactions ADD CONSTRAINT transactions_current_revision_fk
    FOREIGN KEY (id, current_revision)
    REFERENCES transaction_revisions(transaction_id, revision)
    DEFERRABLE INITIALLY DEFERRED;

CREATE TABLE transaction_items (
    transaction_id text NOT NULL,
    revision integer NOT NULL,
    line_number integer NOT NULL CHECK (line_number > 0),
    package_id uuid NOT NULL,
    package_revision integer NOT NULL,
    package_code text NOT NULL,
    package_name text NOT NULL,
    package_description text NOT NULL DEFAULT '',
    unit_price bigint NOT NULL CHECK (unit_price > 0),
    quantity integer NOT NULL CHECK (quantity BETWEEN 1 AND 999),
    line_total bigint NOT NULL CHECK (line_total = unit_price * quantity),
    PRIMARY KEY (transaction_id, revision, line_number),
    FOREIGN KEY (transaction_id, revision)
        REFERENCES transaction_revisions(transaction_id, revision),
    FOREIGN KEY (package_id, package_revision)
        REFERENCES package_revisions(package_id, revision)
);

CREATE INDEX transaction_items_current_filter_idx
    ON transaction_items (package_id, transaction_id, revision);

CREATE TABLE print_attempts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    transaction_id text NOT NULL,
    transaction_revision integer NOT NULL,
    terminal_id uuid REFERENCES terminals(id),
    actor_id uuid NOT NULL REFERENCES users(id),
    session_id uuid NOT NULL REFERENCES sessions(id),
    status text NOT NULL CHECK (status IN ('pending', 'success', 'failed', 'unknown')),
    is_copy boolean NOT NULL DEFAULT false,
    printer_kind text NOT NULL CHECK (printer_kind IN ('simulator', 'bluetooth', 'integrated')),
    printer_identifier text,
    error_code text,
    error_message text,
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    client_occurred_at timestamptz NOT NULL,
    server_received_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (transaction_id, transaction_revision)
        REFERENCES transaction_revisions(transaction_id, revision),
    CHECK ((status IN ('failed', 'unknown')) OR (error_code IS NULL AND error_message IS NULL))
);

CREATE INDEX print_attempts_transaction_idx
    ON print_attempts (transaction_id, server_received_at DESC);

CREATE TABLE audit_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    event_type text NOT NULL,
    aggregate_type text NOT NULL,
    aggregate_id text NOT NULL,
    origin_actor_id uuid REFERENCES users(id),
    origin_session_id uuid REFERENCES sessions(id),
    submitted_by_actor_id uuid REFERENCES users(id),
    submitted_by_session_id uuid REFERENCES sessions(id),
    terminal_id uuid REFERENCES terminals(id),
    before_values jsonb,
    after_values jsonb,
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    occurred_at timestamptz NOT NULL,
    server_received_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX audit_events_aggregate_idx
    ON audit_events (aggregate_type, aggregate_id, server_received_at DESC);
CREATE INDEX audit_events_actor_idx
    ON audit_events (submitted_by_actor_id, server_received_at DESC);

CREATE TABLE sync_changes (
    cursor bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    aggregate text NOT NULL CHECK (aggregate IN ('user', 'package', 'transaction', 'print_attempt', 'terminal')),
    aggregate_id text NOT NULL,
    action text NOT NULL CHECK (action IN ('created', 'updated', 'deleted')),
    revision integer,
    payload jsonb NOT NULL,
    tombstone boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((tombstone AND action = 'deleted') OR NOT tombstone)
);

CREATE INDEX sync_changes_aggregate_idx
    ON sync_changes (aggregate, aggregate_id, cursor DESC);

CREATE TABLE idempotency_records (
    terminal_id uuid NOT NULL REFERENCES terminals(id),
    operation_id text NOT NULL CHECK (length(operation_id) BETWEEN 8 AND 100),
    request_hash bytea NOT NULL CHECK (octet_length(request_hash) = 32),
    response_status integer NOT NULL CHECK (response_status BETWEEN 100 AND 599),
    response jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (terminal_id, operation_id)
);

CREATE OR REPLACE FUNCTION reject_append_only_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION '% is append-only', TG_TABLE_NAME
        USING ERRCODE = '55000';
END
$$;

CREATE TRIGGER package_revisions_append_only
    BEFORE UPDATE OR DELETE ON package_revisions
    FOR EACH ROW EXECUTE FUNCTION reject_append_only_mutation();
CREATE TRIGGER transaction_revisions_append_only
    BEFORE UPDATE OR DELETE ON transaction_revisions
    FOR EACH ROW EXECUTE FUNCTION reject_append_only_mutation();
CREATE TRIGGER transaction_items_append_only
    BEFORE UPDATE OR DELETE ON transaction_items
    FOR EACH ROW EXECUTE FUNCTION reject_append_only_mutation();
CREATE TRIGGER print_attempts_append_only
    BEFORE UPDATE OR DELETE ON print_attempts
    FOR EACH ROW EXECUTE FUNCTION reject_append_only_mutation();
CREATE TRIGGER audit_events_append_only
    BEFORE UPDATE OR DELETE ON audit_events
    FOR EACH ROW EXECUTE FUNCTION reject_append_only_mutation();
CREATE TRIGGER sync_changes_append_only
    BEFORE UPDATE OR DELETE ON sync_changes
    FOR EACH ROW EXECUTE FUNCTION reject_append_only_mutation();
CREATE TRIGGER idempotency_records_append_only
    BEFORE UPDATE OR DELETE ON idempotency_records
    FOR EACH ROW EXECUTE FUNCTION reject_append_only_mutation();

INSERT INTO packages (id, code, current_revision)
VALUES
    ('00000000-0000-4000-8000-000000000001', 'STANDARD', 1),
    ('00000000-0000-4000-8000-000000000002', 'SUNRISE', 1)
ON CONFLICT (id) DO NOTHING;

INSERT INTO package_revisions
    (package_id, revision, name, description, unit_price, change_reason)
VALUES
    ('00000000-0000-4000-8000-000000000001', 1, 'Paket Standar', 'Paket sewa motor standar', 70000, 'Paket awal sistem'),
    ('00000000-0000-4000-8000-000000000002', 1, 'Paket Sunrise', 'Paket sewa motor sunrise', 100000, 'Paket awal sistem')
ON CONFLICT (package_id, revision) DO NOTHING;
