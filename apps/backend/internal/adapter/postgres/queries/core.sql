-- name: GetLiveSessionPrincipal :one
SELECT s.id AS session_id, u.id AS user_id, u.role, u.must_change_password, s.terminal_id
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.token_hash = $1
  AND s.revoked_at IS NULL
  AND u.is_active
  AND u.deleted_at IS NULL;

-- name: PullSyncChanges :many
SELECT cursor, aggregate, aggregate_id, action, revision, payload, tombstone, created_at
FROM sync_changes
WHERE cursor > $1
ORDER BY cursor
LIMIT $2;

-- name: CurrentTransactionItems :many
SELECT i.*
FROM transaction_items i
JOIN transactions t ON t.id = i.transaction_id AND t.current_revision = i.revision
WHERE i.transaction_id = $1
ORDER BY i.line_number;
