package tool

import (
	"github.com/firebase/genkit/go/ai"

	"github.com/naqerl/yao/state"
)

// Register adds all tools to the state's tools slice.
func Register(s *state.State) {
	s.Tools = []ai.ToolRef{
		DefineBash(s.Genkit, s),
		DefineRead(s.Genkit, s),
		DefineWrite(s.Genkit, s),
	}
}
