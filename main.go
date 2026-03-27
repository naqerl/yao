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

	"github.com/naqerl/yao/kaomoji"
	"github.com/naqerl/yao/session"
	"github.com/naqerl/yao/state"
)

func main() {
	var state state.State

	// Parse CLI flags
	flag.StringVar(&state.Provider, "provider", "",
		"provider to initialize")
	flag.StringVar(&state.Model, "m", "",
		"model to use, optionally as provider/model")
	flag.StringVar(&state.Thinking, "t", "off",
		"thinking level: off, low, medium, high")
	flag.StringVar(&state.SystemPath, "s", "",
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
	if err := state.Init(ctx); err != nil {
		log.Fatalf("init failed: %v", err)
	}
	store, err := session.Open(ctx)
	if err != nil {
		log.Fatalf("open session store failed: %v", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			slog.Error("close session store failed", "error", err)
		}
	}()

	// Load last session or create a new one
	err = store.LoadLatestByCwd(ctx, &state)
	if errors.Is(err, session.ErrSessionNotFound) {
		err = store.Create(ctx, &state)
		if err != nil {
			log.Fatalf("create session failed: %v", err)
		}
		slog.Info("created session", "id", state.SessionID, "cwd", state.CWD)
	} else if err != nil {
		log.Fatalf("load session failed: %v", err)
	}
	slog.Info("resumed session", "id", state.SessionID)
	fmt.Println(state.String())

	// Agent loop
	for {
		promptCtx, stop := signal.NotifyContext(ctx, os.Interrupt)

		// Read user input
		fmt.Print("> ")
		prompt, err := readWithContext(promptCtx)
		if err != nil {
			stop()
			if errors.Is(err, context.Canceled) {
				fmt.Println("[cancelled]")
				break
			}
			log.Fatalf("could not read from stdin")
		}
		slog.Debug("got user prompt", "prompt", prompt)
		fmt.Println("\n" + kaomoji.GetRandom())

		// Start LLM
		err = runPrompt(promptCtx, &state, prompt)
		fmt.Println()
		stop()
		if saveErr := store.SaveHistory(ctx, &state); saveErr != nil {
			slog.Error("save session failed", "error", saveErr, "id", state.SessionID)
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

func runPrompt(ctx context.Context, state *state.State, prompt string) error {
	state.Chat = append(state.Chat, ai.NewUserMessage(ai.NewTextPart(prompt)))

	stream := genkit.GenerateStream(ctx, state.Genkit,
		ai.WithSystem(state.System),
		ai.WithTools(state.Tools...),
		ai.WithMessages(state.Chat...),
		ai.WithMaxTurns(int(^uint(0)>>1)),
		ai.WithConfig(state.GenerateConfig),
	)

	acc := newStreamMessageAccumulator(len(state.Chat))

	for result, err := range stream {
		if err != nil {
			acc.Flush(&state.Chat)
			return err
		}

		if result.Done {
			acc.Flush(&state.Chat)
			break
		}

		fmt.Print(result.Chunk.Text())
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
