// Provides configuration and setup for Moonshot AI's Kimi Coding API.
package model

import (
	"context"
	"fmt"
	"os"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
)

func InitKimi(ctx context.Context, modelName string) (InitResult, error) {
	const (
		apiKeyEnv = "KIMI_API_KEY"
		baseURL   = "https://api.kimi.com/coding/v1"
	)

	apiKey := os.Getenv(apiKeyEnv)
	if apiKey == "" {
		return InitResult{}, &CredsNotSetError{Detail: apiKeyEnv}
	}

	if modelName != "k2p5" {
		return InitResult{}, fmt.Errorf("provider kimi does not support model %q", modelName)
	}

	provider := newReasoningCompatibleProvider("kimi", apiKey, baseURL, map[string]ai.ModelOptions{
		modelName: {
			Label:    "Kimi " + modelName,
			Versions: []string{modelName},
			Supports: &ai.ModelSupports{
				Multiturn:  true,
				Tools:      true,
				Media:      true,
				SystemRole: true,
				ToolChoice: true,
			},
		},
	})

	return InitResult{
		Provider: "kimi",
		Model:    modelName,
		Genkit: genkit.Init(ctx,
			genkit.WithPlugins(provider),
			genkit.WithDefaultModel("kimi/"+modelName),
		),
	}, nil
}
