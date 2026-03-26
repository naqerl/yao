package model

import (
	"context"
	"fmt"

	"github.com/firebase/genkit/go/genkit"
)

type Factory = func(ctx context.Context) (*genkit.Genkit, error)

var Factories = []Factory{InitOpencode, InitKimi}

type CredsNotSetError struct {
	Detail string
}

func (e *CredsNotSetError) Error() string {
	return fmt.Sprintf("Credential not found: %s", e.Detail)
}
