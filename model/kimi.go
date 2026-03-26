// Provides configuration and setup for Moonshot AI's Kimi Coding API.
package model

import (
	"context"
	"os"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	"github.com/firebase/genkit/go/plugins/compat_oai"
)

func InitKimi(ctx context.Context) (*genkit.Genkit, error) {
	const (
		apiKeyEnv = "KIMI_API_KEY"
		modelName = "k2p5"
		baseURL   = "https://api.kimi.com/coding/v1"
	)

	apiKey := os.Getenv(apiKeyEnv)
	if apiKey == "" {
		return nil, &CredsNotSetError{Detail: apiKeyEnv}
	}

	// Use the OpenAI-compatible plugin since Kimi API is OpenAI-compatible
	provider := &compat_oai.OpenAICompatible{
		Provider: "kimi",
		APIKey:   apiKey,
		BaseURL:  baseURL,
	}

	g := genkit.Init(ctx,
		genkit.WithPlugins(provider),
		genkit.WithDefaultModel("kimi/"+modelName),
	)

	// Define the model with its capabilities
	provider.DefineModel("kimi", modelName, ai.ModelOptions{
		Label:    "Kimi " + modelName,
		Versions: []string{modelName},
		Supports: &ai.ModelSupports{
			Multiturn:  true,
			Tools:      true,
			Media:      true,
			SystemRole: true,
			ToolChoice: true,
		},
	})

	return g, nil
}
