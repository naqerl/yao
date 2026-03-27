package state

import (
	"context"
	"fmt"

	"github.com/naqerl/yao/model"
)

func InitGenkit(ctx context.Context, s *State) error {
	if s.Provider == "" {
		s.Provider = model.DefaultProviderName
	}

	provider, ok := model.Providers[s.Provider]
	if !ok {
		return fmt.Errorf("unknown provider: %s", s.Provider)
	}

	if s.Model == "" {
		s.Model = provider.DefaultModel
	}

	var err error
	s.Genkit, err = provider.Init(ctx, s.Model)
	if err != nil {
		return nil
	}

	return nil
}
