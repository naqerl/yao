package session

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

	"github.com/naqerl/yao/session/db"
	sqldata "github.com/naqerl/yao/sql"
	"github.com/naqerl/yao/state"
)

type Session struct {
	ID      int64
	CWD     string
	History []*ai.Message
}

var ErrSessionNotFound = errors.New("session not found")

type Store struct {
	db      *sql.DB
	queries *db.Queries
}

func Open(ctx context.Context) (*Store, error) {
	if ctx == nil {
		return nil, fmt.Errorf("context is required")
	}

	dbPath, err := path()
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
		db:      conn,
		queries: db.New(conn),
	}, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) LoadLatestByCwd(ctx context.Context, state *state.State) error {
	if s == nil {
		return fmt.Errorf("store is required")
	}

	row, err := s.queries.GetLatestSessionByCwd(ctx, state.CWD)
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

func (s *Store) Create(ctx context.Context, state *state.State) error {
	if s == nil {
		return fmt.Errorf("store is required")
	}

	session := Session{
		ID:      sessionIDNow(),
		CWD:     state.CWD,
		History: nil,
	}

	if err := s.queries.CreateSession(ctx, db.CreateSessionParams{
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

func (s *Store) SaveHistory(ctx context.Context, state *state.State) error {
	if s == nil {
		return fmt.Errorf("store is required")
	}
	historyJSON, err := encodeHistory(state.Chat)
	if err != nil {
		return fmt.Errorf("encode history: %w", err)
	}

	if err := s.queries.SaveSessionHistory(ctx, db.SaveSessionHistoryParams{
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

func path() (string, error) {
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
