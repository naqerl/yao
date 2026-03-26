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
	"syscall"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"

	"github.com/naqerl/yao/model"
	"github.com/naqerl/yao/tool"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	slog.Info("starting")

	g, err := Init(ctx)
	if err != nil {
		log.Fatalf("init failed: %s", err)
	}

	bashTool := tool.DefineBash(g)

	var chat []*ai.Message
	agent := genkit.DefineFlow(g,
		"agent",
		func(ctx context.Context, task string) (string, error) {
			chat = append(chat, ai.NewUserMessage(ai.NewTextPart(task)))
			resp, err := genkit.Generate(ctx, g,
				ai.WithTools(bashTool),
				ai.WithMessages(chat...),
				ai.WithMaxTurns(10^24),
				ai.WithConfig(map[string]any{"max_tokens": 1024}),
			)
			if err != nil {
				return "", err
			}
			chat = resp.History()
			return resp.Text(), err
		})

	slog.Info("genkit inited")

	for {
		fmt.Print("> ")
		prompt, err := readWithContext(ctx)
		if err != nil {
			log.Fatalf("could not read from stdin")
		}

		resp, err := agent.Run(ctx, prompt)
		if err != nil {
			slog.Error("failed to run flow", "with", err)
		}
		fmt.Println(resp)
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

func readWithContext(ctx context.Context) (string, error) {
	inChan := make(chan any)
	go func() {
		reader := bufio.NewReader(os.Stdin)
		prompt, err := reader.ReadString('\n')
		if err != nil {
			inChan <- err
		} else {
			inChan <- prompt
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
