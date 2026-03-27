CREATE TABLE IF NOT EXISTS session (
  id INTEGER NOT NULL,
  cwd TEXT NOT NULL,
  history_json BLOB NOT NULL CHECK (json_valid(history_json, 8)),
  PRIMARY KEY (cwd, id)
);
