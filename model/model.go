package model

import (
	"context"
	"fmt"

	"github.com/firebase/genkit/go/genkit"
)

const DefaultProviderName = openCodeGoProvider

type ProviderSpec struct {
	DefaultModel string
	Init         func(ctx context.Context, model string) (*genkit.Genkit, error)
}

var Providers = map[string]ProviderSpec{
	openCodeGoProvider: {
		DefaultModel: openCodeGoFallbackModel,
		Init:         InitOpenCodeGo,
	},
	"kimi": {
		DefaultModel: "k2p5",
		Init:         InitKimi,
	},
}

type CredsNotSetError struct {
	Detail string
}

func (e *CredsNotSetError) Error() string {
	return fmt.Sprintf("Credential not found: %s", e.Detail)
}
