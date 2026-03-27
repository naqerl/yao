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

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"

	"github.com/naqerl/yao/kaomoji"
	"github.com/naqerl/yao/model"
	"github.com/naqerl/yao/tool"
)

func main() {
	promptFlag := flag.String("p", "", "single prompt to run and exit")
	flag.StringVar(promptFlag, "prompt", "", "single prompt to run and exit")
	providerFlag := flag.String("provider", "", "provider to initialize")
	modelFlag := flag.String("m", "", "model to use, optionally as provider/model")
	flag.StringVar(modelFlag, "model", "", "model to use, optionally as provider/model")
	thinkingFlag := flag.String("t", "off", "thinking level: off, low, medium, or high")
	flag.StringVar(thinkingFlag, "thinking", "off", "thinking level: off, low, medium, or high")
	flag.Parse()

	initLogger()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	slog.Info("starting")

	state, err := resolveRuntimeState(*providerFlag, *modelFlag)
	if err != nil {
		log.Fatalf("state resolution failed: %s", err)
	}
	if err := model.Init(ctx, state); err != nil {
		log.Fatalf("init failed: %s", err)
	}
	slog.Info("genkit inited", "provider", state.Provider, "model", state.Model)

	bashTool := tool.DefineBash(state.Genkit)
	thinkingConfig, err := makeThinkingConfig(*thinkingFlag)
	if err != nil {
		log.Fatalf("invalid thinking setting: %s", err)
	}
	fmt.Printf("provider: %s\nmodel: %s\nthinking: %s\n\n", state.Provider, state.Model, strings.ToLower(strings.TrimSpace(*thinkingFlag)))

	if *promptFlag != "" {
		if _, err := runPrompt(ctx, state.Genkit, bashTool, nil, *promptFlag, thinkingConfig); err != nil {
			log.Fatalf("prompt failed: %s", err)
		}
		return
	}

	var chat []*ai.Message
	for {
		fmt.Print("> ")
		prompt, err := readWithContext(ctx)
		if err != nil {
			log.Fatalf("could not read from stdin")
		}
		slog.Debug("got user prompt", "prompt", prompt)
		fmt.Println("\n" + kaomoji.GetRandom())

		updatedChat, err := runPrompt(ctx, state.Genkit, bashTool, chat, prompt, thinkingConfig)
		if err != nil {
			slog.Error("Prompt failed", "error", err)
			continue
		}
		chat = updatedChat
	}
}

func initLogger() {
	level := slog.LevelWarn
	if logEnv := os.Getenv("LOG"); logEnv != "" {
		switch strings.ToUpper(logEnv) {
		case "DEBUG":
			level = slog.LevelDebug
		case "INFO":
			level = slog.LevelInfo
		case "WARN", "WARNING":
			level = slog.LevelWarn
		case "ERROR":
			level = slog.LevelError
		}
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))
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
		return "", errors.New("context done")
	case d := <-inChan:
		if s, ok := d.(string); ok {
			return s, nil
		}
		return "", d.(error)
	}
}

func runPrompt(ctx context.Context, g *genkit.Genkit, bashTool ai.Tool, chat []*ai.Message, prompt string, config map[string]any) ([]*ai.Message, error) {
	chat = append(chat, ai.NewUserMessage(ai.NewTextPart(prompt)))
	stream := genkit.GenerateStream(ctx, g,
		ai.WithTools(bashTool),
		ai.WithMessages(chat...),
		ai.WithMaxTurns(int(^uint(0)>>1)),
		ai.WithConfig(config),
	)

	for result, err := range stream {
		if err != nil {
			return chat, err
		}
		if result.Done {
			fmt.Println()
			return result.Response.History(), nil
		}
		fmt.Print(result.Chunk.Text())
	}

	return chat, nil
}

func makeThinkingConfig(level string) (map[string]any, error) {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "", "off", "none", "disable", "disabled":
		return map[string]any{
			"thinking": map[string]any{
				"type": "off",
			},
		}, nil
	case "low", "medium", "high":
		return map[string]any{
			"thinking": map[string]any{
				"type": "enabled",
			},
			"reasoning_effort": level,
		}, nil
	default:
		return nil, fmt.Errorf("expected off, low, medium, or high, got %q", level)
	}
}

func resolveRuntimeState(providerValue, modelValue string) (*model.RuntimeState, error) {
	state := &model.RuntimeState{
		Provider: strings.TrimSpace(providerValue),
		Model:    strings.TrimSpace(modelValue),
	}

	if state.Provider == "" {
		state.Provider = strings.TrimSpace(os.Getenv("YAO_PROVIDER"))
	}
	if state.Model == "" {
		state.Model = strings.TrimSpace(os.Getenv("YAO_MODEL"))
	}

	if state.Model != "" && strings.Contains(state.Model, "/") {
		parts := strings.SplitN(state.Model, "/", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return nil, fmt.Errorf("invalid model reference %q, expected provider/model", state.Model)
		}
		if state.Provider == "" {
			state.Provider = strings.TrimSpace(parts[0])
		}
		state.Model = strings.TrimSpace(parts[1])
	}

	return state, nil
}
