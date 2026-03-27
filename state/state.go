package state

import (
	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
)

type State struct {
	// Name of the provider opencode-go/kimi-k2.5
	//                      ^^^^^^^^^^^
	Provider string
	// Name of the model e.g. opencode-go/kimi-k2.5
	//                                    ^^^^^^^^^
	Model string
	// Empty string means off
	Thinking string
	// Path to the script which's stdout will be used as a system prompt
	SystemPath string
	// Result of the execution of SystemPath script
	System string
	// The main engine to work with LLM
	Genkit *genkit.Genkit
	// Slice of available tools
	Tools []ai.ToolRef
	// Any additional setup which should be added to the Genkit.Generate method
	GenerateConfig any
	// History of the current session's messages
	Chat []*ai.Message
}

// Init validates and resolves all required fields to work
// It's good enough to be called on the default value
func (s *State) Init() error {
	panic("not implemented")
}

// String returns user friendly info about current state
func (s *State) String() error {
	panic("not implemented")
}
