// Provides configuration and setup for opencode provider
package model

import (
        "context"
        "fmt"
        "os"

        "github.com/firebase/genkit/go/genkit"
        "github.com/firebase/genkit/go/plugins/compat_oai"
)

func InitOpencode(ctx context.Context) (*genkit.Genkit, error) {
        const (
                apiKeyEnv = "OPENCODE_API_KEY"
                modelName = "glm-5"
                baseURL   = "https://opencode.ai/zen/go/v1"
        )

        apiKey := os.Getenv(apiKeyEnv)
        if apiKey == "" {
                return nil, &CredsNotSetError{Detail: apiKeyEnv}
        }

        // Use the OpenAI-compatible plugin since Kimi API is OpenAI-compatible
        provider := &compat_oai.OpenAICompatible{
                Provider: "opencode-go",
                APIKey:   apiKey,
                BaseURL:  baseURL,
        }

        g := genkit.Init(ctx,
                genkit.WithPlugins(provider),
                genkit.WithDefaultModel("opencode-go/"+modelName),
        )

        fmt.Println("opencode")
        return g, nil
}
