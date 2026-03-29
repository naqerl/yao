package tool

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"

	"github.com/naqerl/yao/state"
)

// DefineBash defines the bash tool on the given genkit instance.
func DefineBash(g *genkit.Genkit, s *state.State) *ai.ToolDef[bashInput, bashOutput] {
	return genkit.DefineTool(
		g, "bash", `Execute bash command.

Do NOT use '| head' or '| tail' to limit output - the tool handles truncation automatically.
If output is truncated, the tool will indicate how to read the full output.`,
		func(ctx *ai.ToolContext, input bashInput) (bashOutput, error) {
			msg := fmt.Sprintf("\n$ %s", input.Cmd)
			select {
			case s.Bus <- msg:
			case <-ctx.Done():
			}
			out, err := runBash(input)
			if err != nil {
				err = fmt.Errorf("could not run bash: %w", err)
			}
			return out, err
		})
}

type bashInput struct {
	Cmd string `json:"cmd" jsonschema_description:"Bash command to execute. Do NOT pipe to head/tail - output will be truncated automatically if needed."`
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
			b.WriteString("  " + strings.TrimSuffix(line, "\n") + "\n")
		}
	}

	if len(o.Stderr) > 0 {
		for _, line := range strings.Split(o.Stderr, "\n") {
			b.WriteString("  " + strings.TrimSuffix(line, "\n") + "\n")
		}

	}

	if b.Len() == 0 {
		b.WriteString("[empty output]\n")
	}
	if o.ExitCode != 0 {
		b.WriteString(fmt.Sprintf("exit code: %d\n", o.ExitCode))
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
