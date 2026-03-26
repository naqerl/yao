package main

import (
	"bufio"
	"context"
	"errors"
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
	initLogger()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	slog.Info("starting")

	g, err := Init(ctx)
	if err != nil {
		log.Fatalf("init failed: %s", err)
	}
	slog.Info("genkit inited")

	bashTool := tool.DefineBash(g)

	var chat []*ai.Message
	for {
		// Read user prompt
		fmt.Print("> ")
		prompt, err := readWithContext(ctx)
		if err != nil {
			log.Fatalf("could not read from stdin")
		}
		slog.Debug("got user prompt", "prompt", prompt)
		fmt.Println("\n" + kaomoji.GetRandom())

		// Call agent
		chat = append(chat, ai.NewUserMessage(ai.NewTextPart(prompt)))
		stream := genkit.GenerateStream(ctx, g,
			ai.WithTools(bashTool),
			ai.WithMessages(chat...),
			ai.WithMaxTurns(10^24),
			ai.WithConfig(map[string]any{"max_tokens": 1024}),
		)

		// Stream output
		for result, err := range stream {
			if err != nil {
				log.Printf("Stream error: %v", err)
				break
			}
			if result.Done {
				chat = result.Response.History()
			} else {
				fmt.Print(result.Chunk.Text())
			}
		}

	}
}

func Init(ctx context.Context) (*genkit.Genkit, error) {
	var errs []error

	for _, factory := range model.Factories {
		g, err := factory(ctx)

		if err == nil {
			return g, nil
		}

		if _, ok := errors.AsType[*model.CredsNotSetError](err); ok {
			errs = append(errs, err)
			continue
		}

		return nil, fmt.Errorf("could not run factory: %w", err)
	}

	return nil, errors.Join(errs...)
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
				// Ignore empty string submission
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
		} else {
			return "", d.(error)
		}
	}
}
