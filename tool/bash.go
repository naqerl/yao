package tool

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"os/exec"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
)

// DefineBash defines the bash tool on the given genkit instance.
func DefineBash(g *genkit.Genkit) *ai.ToolDef[bashInput, bashOutput] {
	return genkit.DefineTool(
		g, "bash", "Execute bash command",
		func(ctx *ai.ToolContext, input bashInput) (bashOutput, error) {
			slog.Info("[bash]", "command", input.Cmd)
			out, err := runBash(input)
			if err != nil {
				err = fmt.Errorf("could not run bash: %w", err)
			}
			slog.Info("[bash]", "out", out)
			return out, err
		})
}

type bashInput struct {
	Cmd string `json:"cmd" jsonschema_description:"Bash command to be executed"`
}

type bashOutput struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

func runBash(input bashInput) (bashOutput, error) {
	var out bashOutput
	cmd := exec.Command("bash", "-c", input.Cmd)

	// Use current working directory
	wd, err := os.Getwd()
	if err != nil {
		return out, err
	}
	cmd.Dir = wd

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return out, err
		}
	}

	return bashOutput{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
	}, nil
}
