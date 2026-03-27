-- name: CreateSession :exec
INSERT INTO session (
  id,
  cwd,
  history_json
) VALUES (
  ?, ?, jsonb(?)
);

-- name: SaveSessionHistory :exec
INSERT INTO session (
  id,
  cwd,
  history_json
) VALUES (
  ?, ?, jsonb(?)
)
ON CONFLICT (cwd, id) DO UPDATE SET
  history_json = excluded.history_json;

-- name: GetLatestSessionByCwd :one
SELECT
  id,
  cwd,
  json(history_json) AS history_json
FROM session
WHERE cwd = ?
ORDER BY id DESC
LIMIT 1;

-- name: ListSessionsByCwd :many
SELECT
  id,
  cast(json_array_length(history_json) as int) AS message_count
FROM session
WHERE cwd = ?
ORDER BY id DESC;
