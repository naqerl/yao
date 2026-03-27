package model

import (
	"context"
	"fmt"
	"strings"

	"github.com/firebase/genkit/go/genkit"
)

type RuntimeState struct {
	Provider string
	Model    string
	Genkit   *genkit.Genkit
}

type ProviderSpec struct {
	Name         string
	DefaultModel string
	Init         func(ctx context.Context, state *RuntimeState) error
}

var Providers = map[string]ProviderSpec{
	openCodeGoProvider: {
		Name:         openCodeGoProvider,
		DefaultModel: openCodeGoFallbackModel,
		Init:         InitOpenCodeGo,
	},
	"kimi": {
		Name:         "kimi",
		DefaultModel: "k2p5",
		Init:         InitKimi,
	},
}

func Init(ctx context.Context, state *RuntimeState) error {
	if state == nil {
		return fmt.Errorf("runtime state is required")
	}

	state.Provider = strings.TrimSpace(state.Provider)
	state.Model = strings.TrimSpace(state.Model)
	if state.Provider == "" {
		state.Provider = openCodeGoProvider
	}

	provider, ok := Providers[state.Provider]
	if !ok {
		return fmt.Errorf("unknown provider: %s", state.Provider)
	}

	if state.Model == "" {
		state.Model = provider.DefaultModel
	}

	if err := provider.Init(ctx, state); err != nil {
		return err
	}
	if state.Genkit == nil {
		return fmt.Errorf("provider %s did not initialize genkit", state.Provider)
	}

	return nil
}

type CredsNotSetError struct {
	Detail string
}

func (e *CredsNotSetError) Error() string {
	return fmt.Sprintf("Credential not found: %s", e.Detail)
}
