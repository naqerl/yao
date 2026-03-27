package model

import (
	"context"
	"fmt"
	"strings"

	"github.com/firebase/genkit/go/genkit"
)

type InitResult struct {
	Provider string
	Model    string
	Genkit   *genkit.Genkit
}

type ProviderSpec struct {
	Name         string
	DefaultModel string
	Init         func(ctx context.Context, model string) (InitResult, error)
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

func Init(ctx context.Context, providerName, modelName string) (InitResult, error) {
	providerName = strings.TrimSpace(providerName)
	modelName = strings.TrimSpace(modelName)
	if providerName == "" {
		providerName = openCodeGoProvider
	}

	provider, ok := Providers[providerName]
	if !ok {
		return InitResult{}, fmt.Errorf("unknown provider: %s", providerName)
	}

	if modelName == "" {
		modelName = provider.DefaultModel
	}

	result, err := provider.Init(ctx, modelName)
	if err != nil {
		return InitResult{}, err
	}
	if result.Provider == "" {
		result.Provider = providerName
	}
	if result.Model == "" {
		result.Model = modelName
	}
	if result.Genkit == nil {
		return InitResult{}, fmt.Errorf("provider %s did not initialize genkit", result.Provider)
	}

	return result, nil
}

type CredsNotSetError struct {
	Detail string
}

func (e *CredsNotSetError) Error() string {
	return fmt.Sprintf("Credential not found: %s", e.Detail)
}
