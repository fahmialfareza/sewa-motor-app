-- name: GetLiveSessionPrincipal :one
SELECT s.id AS session_id, u.id AS user_id, u.role, u.must_change_password,
       s.terminal_id, ds.id AS data_space_id, ds.mode,
       CASE WHEN ds.mode = 'sandbox' THEN ds.generation ELSE 0::bigint END AS sandbox_generation
FROM sessions s
JOIN users u ON u.id = s.user_id
JOIN data_spaces ds ON ds.id = s.data_space_id
WHERE s.token_hash = $1
  AND s.revoked_at IS NULL
  AND ds.status = 'active'
  AND u.is_active
  AND u.deleted_at IS NULL;

-- name: PullSyncChanges :many
SELECT cursor, aggregate, aggregate_id, action, revision, payload, tombstone,
       created_at, data_space_id
FROM sync_changes
WHERE data_space_id = $1 AND cursor > $2
ORDER BY cursor
LIMIT $3;

-- name: CurrentTransactionItems :many
SELECT i.*
FROM transaction_items i
JOIN transactions t
  ON t.id = i.transaction_id AND t.current_revision = i.revision
 AND t.data_space_id = i.data_space_id
WHERE i.transaction_id = $1 AND i.data_space_id = $2
ORDER BY i.line_number;
