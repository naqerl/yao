package tool

import (
	"fmt"
	"os"
	"strings"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"

	"github.com/naqerl/yao/state"
)

// DefineRead defines the read tool on the given genkit instance.
func DefineRead(g *genkit.Genkit, s *state.State) *ai.ToolDef[readInput, readOutput] {
	return genkit.DefineTool(
		g, "read", `Read a file and track its content for safe editing.

Use this tool instead of 'cat' when reading files you plan to edit later.
The tool records a snapshot of the file content, allowing the edit tool
to detect if the file was modified by another process.`,
		func(ctx *ai.ToolContext, input readInput) (readOutput, error) {
			var out readOutput
			content, err := performRead(input, s)
			if err != nil {
				out.Message = err.Error()
			} else {
				out.Success = true
				out.Message = content
			}
			return out, err
		})
}

type readInput struct {
	Path   string `json:"path" jsonschema_description:"Path to the file to read"`
	Offset int    `json:"offset,omitempty" jsonschema_description:"Line number to start reading from (1-indexed). 1 = first line."`
	Limit  int    `json:"limit,omitempty" jsonschema_description:"Maximum number of lines to read. Omit to read to the end."`
}

type readOutput struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// TODO: Add proper boundary error return
func performRead(input readInput, s *state.State) (string, error) {
	if input.Offset < 1 {
		input.Offset = 1
	}
	if input.Limit < 0 {
		return "", fmt.Errorf("limit should be >= 0, got %d", input.Limit)
	}

	// Read fully to build a complete hash in file tracker
	content, err := os.ReadFile(input.Path)
	if err != nil {
		return "", fmt.Errorf("cannot read file at %s with %w", input.Path, err)
	}

	allLines := strings.Split(string(content), "\n")

	fromIdx := input.Offset - 1 // convert to 0-indexed for slicing
	toIdx := len(allLines)
	if input.Limit > 0 {
		toIdx = fromIdx + input.Limit
		if toIdx > len(allLines) {
			return "", fmt.Errorf("offset+limit (%d) exceeds file length (%d lines)", input.Offset+input.Limit, len(allLines))
		}
	}

	lines := allLines[fromIdx:toIdx]

	// Build content with line numbers - fixed 6-char width with tab separator (like cat -n)
	var b strings.Builder
	for i, line := range lines {
		b.WriteString(fmt.Sprintf("%6d\t%s\n", input.Offset+i, line))
	}

	comment := fmt.Sprintf("→ read %s", input.Path)

	if diff := len(allLines) - len(lines); diff > 0 {
		b.WriteString(fmt.Sprintf("<system>%d more lines</system>\n", diff))
		// Show 1-indexed line numbers in comment
		endLine := input.Offset + len(lines) - 1
		if input.Limit == 0 {
			endLine = len(allLines)
		}
		comment += fmt.Sprintf(" (lines %d-%d of %d)", input.Offset, endLine, len(allLines))
	}

	s.FileTracker.RecordSnapshot(input.Path, content)

	fmt.Println(comment)

	return b.String(), nil
}
