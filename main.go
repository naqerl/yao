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
	slog.SetDefault(slog.New(slogenv.NewHandler(slog.NewTextHandler(os.Stderr, nil))))

	// Globally respect only SIGTERM
	// SIGINT is handled on per operation basis
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM)
	defer stop()

	// Initialize state
	err := state.Init(ctx)
	if err != nil {
		log.Fatalf("init failed: %v", err)
	}
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

// func runPrompt(ctx context.Context, g *genkit.Genkit, bashTool ai.Tool, systemPrompt string, chat []*ai.Message, prompt string, config map[string]any) ([]*ai.Message, error) {
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
