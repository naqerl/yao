package state

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"

	"github.com/naqerl/yao/system"
)

// Command is the interface for user-executable commands.
type Command interface {
	Execute(ctx context.Context, s *State, args string) error
	GetDescription() string
}

type State struct {
	// Name of the provider, for example opencode-go.
	Provider string
	// Name of the model, for example kimi-k2.5.
	Model string
	// Empty string means off
	Thinking string
	// Path to the script which's stdout will be used as a system prompt
	SystemPath string
	// Result of the execution of SystemPath script
	System string
	// The main engine to work with LLM
	Genkit *genkit.Genkit
	// Slice of available tools
	Tools []ai.ToolRef
	// Any additional setup which should be added to the Genkit.Generate method
	GenerateConfig any
	// Current persisted session ID.
	SessionID int64
	// Working directory the current session belongs to.
	CWD string
	// History of the current session's messages
	Chat []*ai.Message
	// Available commands indexed by name
	Commands map[string]Command
	// Store for session persistence
	Store *Store
	// FileTracker tracks file content to detect external modifications.
	FileTracker *FileTracker
}

// Init validates and resolves all required fields to work.
// It is safe to call on a zero-value State, but not on a nil *State.
func (s *State) Init(ctx context.Context) error {
	// System prompt
	var (
		systemPrompt string
		err          error
	)
	if s.SystemPath == "" {
		systemPrompt, err = system.Default()
	} else {
		systemPrompt, err = system.Eval(s.SystemPath)
	}
	if err != nil {
		return err
	}
	s.System = strings.TrimSpace(systemPrompt)

	// Genkit setup
	if err := InitGenkit(ctx, s); err != nil {
		return err
	}

	// Project path
	s.CWD, err = os.Getwd()
	if err != nil {
		return fmt.Errorf("resolve cwd: %w", err)
	}

	// Load or init session
	s.Store, err = NewStore(ctx)
	if err != nil {
		return fmt.Errorf("could not open store: %w", err)
	}
	s.FileTracker = NewFileTracker()
	if err := s.Store.LoadLatestByCwd(ctx, s); errors.Is(err, ErrSessionNotFound) {
		if err := s.Store.Create(ctx, s); err != nil {
			return fmt.Errorf("create session: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("load session: %w", err)
	}

	// Thinking setup
	switch s.Thinking {
	case "", "off":
		s.Thinking = "off"
		s.GenerateConfig = nil
	case "low":
		s.GenerateConfig = thinkingConfig(s.Thinking, 1024)
	case "medium":
		s.GenerateConfig = thinkingConfig(s.Thinking, 4096)
	case "high":
		s.GenerateConfig = thinkingConfig(s.Thinking, 8192)
	default:
		return fmt.Errorf("invalid thinking level %q", s.Thinking)
	}

	return nil
}

// String returns user friendly info about current state
func (s *State) String() string {
	systemSource := "default"
	if s.SystemPath != "" {
		systemSource = s.SystemPath
	}

	var b strings.Builder
	b.WriteString("state:\n")
	b.WriteString("  provider: " + s.Provider + "\n")
	b.WriteString("  model:    " + s.Model + "\n")
	b.WriteString("  thinking: " + s.Thinking + "\n")
	b.WriteString("  session:\n")
	b.WriteString("    id:     " + strconv.FormatInt(s.SessionID, 10) + "\n")
	b.WriteString("    cwd:    " + s.CWD + "\n")
	b.WriteString("    chat:   " + strconv.Itoa(len(s.Chat)) + "\n")
	b.WriteString("  system:   " + systemSource + "\n")
	b.WriteString("  tools:\n")
	for _, t := range s.Tools {
		b.WriteString("    - " + t.Name() + "\n")
	}
	b.WriteString("  commands:\n")
	for name, c := range s.Commands {
		b.WriteString(fmt.Sprintf("    - %s: %s\n", name, c.GetDescription()))
	}

	return b.String()
}

func (s *State) Close() error {
	if s.Store != nil {
		return s.Store.Close()
	}
	return nil
}

func thinkingConfig(level string, budgetTokens int64) map[string]any {
	return map[string]any{
		"thinking": map[string]any{
			"type":          level,
			"budget_tokens": budgetTokens,
		},
	}
}
