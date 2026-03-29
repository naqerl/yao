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
  json(history_json) AS history_json
FROM session
WHERE cwd = ?
ORDER BY id DESC;

-- name: GetSessionByID :one
SELECT
  id,
  cwd,
  json(history_json) AS history_json
FROM session
WHERE cwd = ? AND id = ?;


-- name: GetConfig :one
SELECT value FROM config WHERE key = ?;

-- name: SetConfig :exec
INSERT INTO config (key, value) VALUES (?, ?)
ON CONFLICT (key) DO UPDATE SET value = excluded.value;
