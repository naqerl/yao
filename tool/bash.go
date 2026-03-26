package tool

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
)

// DefineBash defines the bash tool on the given genkit instance.
func DefineBash(g *genkit.Genkit) *ai.ToolDef[bashInput, bashOutput] {
	return genkit.DefineTool(
		g, "bash", "Execute bash command",
		func(ctx *ai.ToolContext, input bashInput) (bashOutput, error) {
			fmt.Printf("\n=== bash -c '%s' ===\n", input.Cmd)
			out, err := runBash(input)
			if err != nil {
				err = fmt.Errorf("could not run bash: %w", err)
			}
			fmt.Print(out.String())
			fmt.Printf("=== bash ===\n\n")
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

func (o bashOutput) String() string {
	var b strings.Builder

	if len(o.Stdout) > 0 {
		for _, line := range strings.Split(o.Stdout, "\n") {
			b.WriteString("  " + line + "\n")
		}
	}

	if len(o.Stderr) > 0 {
		for _, line := range strings.Split(o.Stderr, "\n") {
			b.WriteString("  " + line + "\n")
		}

	}

	if b.Len() == 0 {
		b.WriteString("[empty output]")
	}
	if o.ExitCode != 0 {
		b.WriteString(fmt.Sprintf("exit code: %d", o.ExitCode))
	}

	return b.String()
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
