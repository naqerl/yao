package state

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"

	"github.com/naqerl/yao/model"
	"github.com/naqerl/yao/system"
	"github.com/naqerl/yao/tool"
)

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
}

// Init validates and resolves all required fields to work.
// It is safe to call on a zero-value State, but not on a nil *State.
func (s *State) Init(ctx context.Context) error {
	if s == nil {
		return fmt.Errorf("state is required")
	}
	if ctx == nil {
		return fmt.Errorf("context is required")
	}

	s.Provider = strings.TrimSpace(s.Provider)
	s.Model = strings.TrimSpace(s.Model)
	s.Thinking = strings.ToLower(strings.TrimSpace(s.Thinking))
	s.SystemPath = strings.TrimSpace(s.SystemPath)

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

	modelRuntime, err := model.Init(ctx, s.Provider, s.Model)
	if err != nil {
		return err
	}
	s.Provider = modelRuntime.Provider
	s.Model = modelRuntime.Model
	s.Genkit = modelRuntime.Genkit

	s.Tools = []ai.ToolRef{
		tool.DefineBash(s.Genkit),
	}
	s.CWD, err = os.Getwd()
	if err != nil {
		return fmt.Errorf("resolve cwd: %w", err)
	}

	return nil
}

// String returns user friendly info about current state
func (s *State) String() string {
	if s == nil {
		return "<nil>"
	}

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
	for _, toolRef := range s.Tools {
		b.WriteString("    - " + toolRef.Name() + "\n")
	}
	if len(s.Tools) == 0 {
		b.WriteString("    []\n")
	}

	return b.String()
}

func thinkingConfig(level string, budgetTokens int64) map[string]any {
	return map[string]any{
		"thinking": map[string]any{
			"type":          level,
			"budget_tokens": budgetTokens,
		},
	}
}
