// Provides configuration and setup for Moonshot AI's Kimi Coding API.
package model

import (
	"context"
	"fmt"
	"os"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	"github.com/firebase/genkit/go/plugins/anthropic"
)

func InitKimi(ctx context.Context) (*genkit.Genkit, error) {
	const (
		apiKeyEnv = "KIMI_API_KEY"
		modelName = "k2p5"
		baseURL   = "https://api.kimi.com/coding"
	)

	apiKey := os.Getenv(apiKeyEnv)
	if apiKey == "" {
		return nil, &CredsNotSetError{Detail: apiKeyEnv}
	}

	provider := &anthropic.Anthropic{
		APIKey:  apiKey,
		BaseURL: baseURL,
	}
	g := genkit.Init(ctx,
		genkit.WithPlugins(provider),
		genkit.WithDefaultModel(fmt.Sprintf("anthropic/%s", modelName)),
	)
	_, err := provider.DefineModel(nil, modelName, &ai.ModelOptions{
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
	return g, err
}

// func (c Config) GenOpts(tools []ai.ToolRef, msgs []*ai.Message) []ai.GenerateOption {
//      return []ai.GenerateOption{
//              ai.WithTools(tools...),
//              ai.WithMessages(msgs...),
//              ai.WithConfig(map[string]any{"max_tokens": c.MaxTokens}),
//      }
// }
