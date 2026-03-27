package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/firebase/genkit/go/ai"
	_ "modernc.org/sqlite"

	sqldata "github.com/naqerl/yao/sql"
	"github.com/naqerl/yao/state/db"
)

var ErrSessionNotFound = errors.New("session not found")

type Store struct {
	db *sql.DB
}

func NewStore(ctx context.Context) (*Store, error) {
	dbPath, err := storePath()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o700); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite db: %w", err)
	}

	conn.SetMaxOpenConns(1)
	conn.SetMaxIdleConns(1)

	if _, err := conn.ExecContext(ctx, sqldata.Schema); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("initialize schema: %w", err)
	}
	if err := conn.PingContext(ctx); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("ping sqlite db: %w", err)
	}

	return &Store{
		db: conn,
	}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) LoadLatestByCwd(ctx context.Context, state *State) error {
	row, err := db.New(s.db).GetLatestSessionByCwd(ctx, state.CWD)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrSessionNotFound
		}
		return fmt.Errorf("query latest session: %w", err)
	}

	history, err := decodeHistory(row.HistoryJson)
	if err != nil {
		return fmt.Errorf("decode history for session %d: %w", row.ID, err)
	}

	state.SessionID = row.ID
	state.CWD = row.Cwd
	state.Chat = history

	return nil
}

func (s *Store) Create(ctx context.Context, state *State) error {
	session := struct {
		ID      int64
		CWD     string
		History []*ai.Message
	}{
		ID:      sessionIDNow(),
		CWD:     state.CWD,
		History: nil,
	}

	if err := db.New(s.db).CreateSession(ctx, db.CreateSessionParams{
		ID:    session.ID,
		Cwd:   session.CWD,
		Jsonb: "[]",
	}); err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	state.SessionID = session.ID
	state.CWD = session.CWD
	state.Chat = session.History

	return nil
}

func (s *Store) SaveHistory(ctx context.Context, state *State) error {
	historyJSON, err := encodeHistory(state.Chat)
	if err != nil {
		return fmt.Errorf("encode history: %w", err)
	}

	if err := db.New(s.db).SaveSessionHistory(ctx, db.SaveSessionHistoryParams{
		ID:    state.SessionID,
		Cwd:   state.CWD,
		Jsonb: historyJSON,
	}); err != nil {
		return fmt.Errorf("save session history: %w", err)
	}

	return nil
}

func encodeHistory(history []*ai.Message) (string, error) {
	if len(history) == 0 {
		return "[]", nil
	}

	body, err := json.Marshal(history)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func decodeHistory(raw any) ([]*ai.Message, error) {
	if raw == nil {
		return nil, nil
	}

	text, ok := raw.(string)
	if !ok {
		return nil, fmt.Errorf("unexpected history json type %T", raw)
	}
	if text == "" {
		return nil, nil
	}

	var history []*ai.Message
	if err := json.Unmarshal([]byte(text), &history); err != nil {
		return nil, err
	}
	return history, nil
}

func storePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home: %w", err)
	}

	return filepath.Join(home, ".local", "share", "yao", "state.db"), nil
}

func sessionIDNow() int64 {
	id, err := strconv.ParseInt(time.Now().UTC().Format("20060102150405"), 10, 64)
	if err != nil {
		panic(fmt.Sprintf("parse session id: %v", err))
	}
	return id
}

// ListByCwd returns session summaries for the given CWD with message counts.
func (s *Store) ListByCwd(ctx context.Context, cwd string) ([]db.ListSessionsByCwdRow, error) {
	rows, err := db.New(s.db).ListSessionsByCwd(ctx, cwd)
	if err != nil {
		return nil, fmt.Errorf("query sessions: %w", err)
	}
	return rows, nil
}
