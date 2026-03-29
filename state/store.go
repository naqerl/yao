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

	"github.com/naqerl/yao/db"
	sqldata "github.com/naqerl/yao/sql"
)

var ErrSessionNotFound = errors.New("session not found")

type Store struct {
	db *sql.DB
}

// SessionSummary holds session info with user message count.
type SessionSummary struct {
	ID               int64
	UserMessageCount int64
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

// LoadByID loads a specific session by ID for the given CWD.
func (s *Store) LoadByID(ctx context.Context, state *State, id int64) error {
	row, err := db.New(s.db).GetSessionByID(ctx, db.GetSessionByIDParams{Cwd: state.CWD, ID: id})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrSessionNotFound
		}
		return fmt.Errorf("query session by id: %w", err)
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

// messageForCounting is a minimal struct for unmarshaling just the role field.
type messageForCounting struct {
	Role ai.Role `json:"role"`
}

// countUserMessages unmarshals JSON history and counts only user messages.
func countUserMessages(raw any) (int64, error) {
	if raw == nil {
		return 0, nil
	}

	text, ok := raw.(string)
	if !ok {
		return 0, fmt.Errorf("unexpected history json type %T", raw)
	}
	if text == "" || text == "[]" {
		return 0, nil
	}

	var messages []messageForCounting
	if err := json.Unmarshal([]byte(text), &messages); err != nil {
		return 0, err
	}

	var count int64
	for _, m := range messages {
		if m.Role == ai.RoleUser {
			count++
		}
	}
	return count, nil
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

// ListByCwd returns session summaries for the given CWD with user message counts.
func (s *Store) ListByCwd(ctx context.Context, cwd string) ([]SessionSummary, error) {
	rows, err := db.New(s.db).ListSessionsByCwd(ctx, cwd)
	if err != nil {
		return nil, fmt.Errorf("query sessions: %w", err)
	}

	summaries := make([]SessionSummary, 0, len(rows))
	for _, row := range rows {
		userCount, err := countUserMessages(row.HistoryJson)
		if err != nil {
			return nil, fmt.Errorf("count user messages for session %d: %w", row.ID, err)
		}
		summaries = append(summaries, SessionSummary{
			ID:               row.ID,
			UserMessageCount: userCount,
		})
	}

	return summaries, nil
}
