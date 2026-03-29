package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	slogenv "github.com/cbrewster/slog-env"
	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"

	"github.com/naqerl/yao/cmd"
	"github.com/naqerl/yao/kaomoji"
	"github.com/naqerl/yao/state"
	"github.com/naqerl/yao/tool"
)

func main() {
	var st state.State

	// Parse CLI flags
	flag.StringVar(&st.Provider, "provider", "",
		"provider to initialize")
	flag.StringVar(&st.Model, "model", "",
		"model to use, optionally as provider/model")
	flag.StringVar(&st.Thinking, "thinking", "off",
		"thinking level: off, low, medium, high")
	flag.StringVar(&st.SystemPath, "system", "",
		"path to a system script")
	flag.Parse()

	// GO_LOG=info,mypackage=debug go run ./...
	if os.Getenv("GO_LOG") == "" {
		os.Setenv("GO_LOG", "warn")
	}
	slog.SetDefault(slog.New(slogenv.NewHandler(slog.NewTextHandler(os.Stderr, nil))))

	// Globally respect only SIGTERM
	// SIGINT is handled on per operation basis
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM)
	defer stop()

	// Initialize state
	if err := st.Init(ctx); err != nil {
		log.Fatalf("init failed: %v", err)
	}

	defer func() {
		if err := st.Close(); err != nil {
			slog.Error("close store failed", "error", err)
		}
	}()

	// Register commands and tools
	cmd.Register(&st)
	tool.Register(&st)

	// Start goroutine to drain Bus to stdout
	go func() {
		for msg := range st.Bus {
			fmt.Print(msg)
		}
	}()

	slog.Info("resumed session", "id", st.SessionID)
	select {
	case st.Bus <- st.String() + "\n":
	case <-ctx.Done():
	}

	// Agent loop
	for {
		promptCtx, stop := signal.NotifyContext(ctx, os.Interrupt)

		// Print prompt through Bus for proper ordering
		select {
		case st.Bus <- "> ":
		case <-promptCtx.Done():
			stop()
			continue
		}

		prompt, err := readWithContext(promptCtx)
		if err != nil {
			stop()
			if errors.Is(err, context.Canceled) {
				select {
				case st.Bus <- "[cancelled]\n":
				case <-ctx.Done():
				}
				break
			}
			log.Fatalf("could not read from stdin")
		}
		slog.Debug("got user prompt", "prompt", prompt)

		// Check if input is a command
		if cmdName, isCmd := cmd.IsCommand(prompt); isCmd {
			command, ok := st.Commands[cmdName]
			if !ok {
				select {
				case st.Bus <- fmt.Sprintf("\nunknown command: /%s\n", cmdName):
				case <-promptCtx.Done():
				}
			} else if err := command.Execute(promptCtx, &st, prompt); err != nil {
				slog.Error("command failed", "command", cmdName, "error", err)
			}
			stop()
			continue
		}

		// Not a command, proceed with LLM
		select {
		case st.Bus <- "\n" + kaomoji.GetRandom() + "\n":
		case <-promptCtx.Done():
		}

		// Eval LLM
		err = runPrompt(promptCtx, &st, prompt)
		select {
		case st.Bus <- "\n":
		case <-promptCtx.Done():
		}
		stop()

		// TODO: Nice to have all the history before the error
		// but currently leads to the existense of toolRequest
		// w/o corresponding toolResponse. This breaks conversation
		// and requires manual cleanup
		if saveErr := st.Store.SaveHistory(ctx, &st); saveErr != nil {
			slog.Error("save session failed", "error", saveErr, "id", st.SessionID)
		}
		if err != nil {
			slog.Error("Prompt failed", "error", err)
			continue
		}
	}
}

func readWithContext(ctx context.Context) (string, error) {
	inChan := make(chan any)

	go func() {
		scn := bufio.NewScanner(os.Stdin)
		var b strings.Builder
		for {
			for scn.Scan() {
				b.WriteString(scn.Text() + "\n")
			}
			if b.Len() > 0 {
				inChan <- b.String()
				return
			}
			if err := scn.Err(); err != nil {
				inChan <- err
			}
			if b.Len() == 0 {
				continue
			}
		}
	}()

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case d := <-inChan:
		if s, ok := d.(string); ok {
			return s, nil
		}
		return "", d.(error)
	}
}

func runPrompt(ctx context.Context, st *state.State, prompt string) error {
	st.Chat = append(st.Chat, ai.NewUserMessage(ai.NewTextPart(prompt)))

	stream := genkit.GenerateStream(ctx, st.Genkit,
		ai.WithSystem(st.System),
		ai.WithTools(st.Tools...),
		ai.WithMessages(st.Chat...),
		ai.WithMaxTurns(int(^uint(0)>>1)),
		ai.WithConfig(st.GenerateConfig),
	)

	acc := newStreamMessageAccumulator(len(st.Chat))

	for result, err := range stream {
		if err != nil {
			acc.Flush(&st.Chat)
			return err
		}

		if result.Done {
			acc.Flush(&st.Chat)
			break
		}

		select {
		case st.Bus <- result.Chunk.Text():
		case <-ctx.Done():
		}
		acc.Add(result.Chunk)
	}

	return nil
}

type streamMessageAccumulator struct {
	baseIndex int
	streamed  map[int]*ai.Message
	order     []int
}

func newStreamMessageAccumulator(baseIndex int) *streamMessageAccumulator {
	return &streamMessageAccumulator{
		baseIndex: baseIndex,
		streamed:  make(map[int]*ai.Message),
	}
}

func (a *streamMessageAccumulator) Add(chunk *ai.ModelResponseChunk) {
	if a == nil || chunk == nil || len(chunk.Content) == 0 {
		return
	}

	idx := a.baseIndex + chunk.Index
	msg, ok := a.streamed[idx]
	if !ok {
		role := chunk.Role
		if role == "" {
			role = ai.RoleModel
		}
		msg = &ai.Message{Role: role}
		a.streamed[idx] = msg
		a.order = append(a.order, idx)
	}

	msg.Content = append(msg.Content, chunk.Content...)
}

func (a *streamMessageAccumulator) Flush(dst *[]*ai.Message) {
	if a == nil || dst == nil {
		return
	}

	for _, idx := range a.order {
		msg := a.streamed[idx]
		if msg == nil || len(msg.Content) == 0 {
			continue
		}
		for _, part := range msg.Content {
			if part.IsToolRequest() {
				slog.Debug("model message part before return", "part", fmt.Sprintf("%#v", part.ToolRequest))
			}
		}
		*dst = append(*dst, msg)
	}
}
