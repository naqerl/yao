package cmd

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/naqerl/yao/state"
)

// Executor is the function signature for command execution.
type Executor func(ctx context.Context, s *state.State, args string) error

// command implements state.Command interface.
type command struct {
	description string
	execute     Executor
}

// Execute implements state.Command.
func (c command) Execute(ctx context.Context, s *state.State, args string) error {
	return c.execute(ctx, s, args)
}

// GetDescription implements state.Command.
func (c command) GetDescription() string {
	return c.description
}

// IsCommand checks if the input matches a command pattern (/word).
// Returns the command name and true if it's a command.
func IsCommand(input string) (string, bool) {
	input = strings.TrimSpace(input)
	if !strings.HasPrefix(input, "/") {
		return "", false
	}
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return "", false
	}
	name := parts[0][1:] // Remove the leading "/"
	if name == "" {
		return "", false
	}
	return name, true
}

// Register adds all commands to the state's command map.
func Register(s *state.State) {
	s.Commands = map[string]state.Command{
		"state": command{
			description: "Print current state",
			execute:     cmdState,
		},
		"list": command{
			description: "List sessions for current directory",
			execute:     cmdList,
		},
		"switch": command{
			description: "Switch to an existing session by ID",
			execute:     cmdSwitch,
		},
		"new": command{
			description: "Create and switch to a new session",
			execute:     cmdNew,
		},
	}
}

// cmdState prints the current state.
func cmdState(ctx context.Context, s *state.State, args string) error {
	select {
	case s.Bus <- "\n" + s.String() + "\n":
	case <-ctx.Done():
	}
	return nil
}

// cmdList lists all sessions for the current CWD with user message counts.
func cmdList(ctx context.Context, s *state.State, args string) error {
	sessions, err := s.Store.ListByCwd(ctx, s.CWD)
	if err != nil {
		return fmt.Errorf("list sessions: %w", err)
	}

	if len(sessions) == 0 {
		select {
		case s.Bus <- "\nNo sessions found for current directory.\n":
		case <-ctx.Done():
		}
		return nil
	}

	select {
	case s.Bus <- "\nSessions for current directory:\n":
	case <-ctx.Done():
	}
	for _, sess := range sessions {
		marker := ""
		if sess.ID == s.SessionID {
			marker = " (active)"
		}
		line := fmt.Sprintf("\n  %d: %d user messages%s\n", sess.ID, sess.UserMessageCount, marker)
		select {
		case s.Bus <- line:
		case <-ctx.Done():
		}
	}
	return nil
}

// cmdNew creates and switches to a new session.
func cmdNew(ctx context.Context, s *state.State, args string) error {
	if err := s.Store.Create(ctx, s); err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	// Clear chat history for the new session
	s.Chat = nil
	msg := fmt.Sprintf("\nCreated and switched to new session: %d\n", s.SessionID)
	select {
	case s.Bus <- msg:
	case <-ctx.Done():
	}
	return nil
}

// cmdSwitch switches to an existing session by ID.
func cmdSwitch(ctx context.Context, s *state.State, args string) error {
	// Parse the session ID from args (everything after the command name)
	fields := strings.Fields(args)
	if len(fields) < 2 {
		return fmt.Errorf("usage: /switch <sessionID>")
	}

	sessionID, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid session ID: %w", err)
	}

	if err := s.Store.LoadByID(ctx, s, sessionID); err != nil {
		if errors.Is(err, state.ErrSessionNotFound) {
			return fmt.Errorf("session %d not found", sessionID)
		}
		return fmt.Errorf("load session: %w", err)
	}

	msg := fmt.Sprintf("\nSwitched to session: %d (%d messages)\n", s.SessionID, len(s.Chat))
	select {
	case s.Bus <- msg:
	case <-ctx.Done():
	}
	return nil
}
