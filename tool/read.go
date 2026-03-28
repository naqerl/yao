package tool

import (
	"fmt"
	"os"
	"strings"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"

	"github.com/naqerl/yao/state"
)

// DefineReadFile defines the read tool on the given genkit instance.
func DefineReadFile(g *genkit.Genkit, s *state.State) *ai.ToolDef[readInput, readOutput] {
	return genkit.DefineTool(
		g, "read", `Read a file and track its content for safe editing.

Use this tool instead of 'cat' when reading files you plan to edit later.
The tool records a snapshot of the file content, allowing the edit tool
to detect if the file was modified by another process.

Use cat only for quick inspection when you don't plan to edit.`,
		func(ctx *ai.ToolContext, input readInput) (readOutput, error) {
			out, err := performRead(input, s)
			if err != nil {
				err = fmt.Errorf("read_file failed: %w", err)
			}
			return out, err
		})
}

type readInput struct {
	Path   string `json:"path" jsonschema_description:"Path to the file to read"`
	Offset int    `json:"offset,omitempty" jsonschema_description:"Line number to start reading from (1-indexed). 0 means read all."`
	Limit  int    `json:"limit,omitempty" jsonschema_description:"Maximum number of lines to read. 0 means read all."`
}

type readOutput struct {
	Content    string `json:"content"`
	TotalLines int    `json:"total_lines"`
	StartLine  int    `json:"start_line"`
	EndLine    int    `json:"end_line"`
}

func (o readOutput) String() string {
	var b strings.Builder

	lines := strings.Split(o.Content, "\n")
	for i, line := range lines {
		lineNum := o.StartLine + i
		b.WriteString(fmt.Sprintf("%4d | %s\n", lineNum, line))
	}

	if o.TotalLines > o.EndLine {
		b.WriteString(fmt.Sprintf("... (%d more lines)\n", o.TotalLines-o.EndLine))
	}

	return b.String()
}

func performRead(input readInput, s *state.State) (readOutput, error) {
	var out readOutput

	if input.Path == "" {
		return out, fmt.Errorf("path is required")
	}

	content, err := os.ReadFile(input.Path)
	if err != nil {
		return out, fmt.Errorf("cannot read file: %w", err)
	}

	allLines := strings.Split(string(content), "\n")
	// Handle trailing newline case
	if len(allLines) > 0 && allLines[len(allLines)-1] == "" {
		allLines = allLines[:len(allLines)-1]
	}
	out.TotalLines = len(allLines)

	// Determine which lines to return
	startIdx := 0
	if input.Offset > 0 {
		startIdx = input.Offset - 1 // Convert to 0-indexed
	}
	if startIdx < 0 {
		startIdx = 0
	}
	if startIdx > len(allLines) {
		startIdx = len(allLines)
	}

	endIdx := len(allLines)
	if input.Limit > 0 {
		endIdx = startIdx + input.Limit
	}
	if endIdx > len(allLines) {
		endIdx = len(allLines)
	}

	selectedLines := allLines[startIdx:endIdx]
	out.Content = strings.Join(selectedLines, "\n")
	out.StartLine = startIdx + 1
	out.EndLine = endIdx

	// Record snapshot for edit tracking
	if s.FileTracker != nil {
		s.FileTracker.RecordSnapshot(input.Path, content)
	}

	fmt.Printf("→ read %s (lines %d-%d of %d)\n", input.Path, out.StartLine, out.EndLine, out.TotalLines)

	return out, nil
}
