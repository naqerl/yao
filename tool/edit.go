package tool

import (
	"fmt"
	"os"
	"strings"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"

	"github.com/naqerl/yao/state"
)

// DefineEdit defines the edit tool on the given genkit instance.
func DefineEdit(g *genkit.Genkit, s *state.State) *ai.ToolDef[editInput, editOutput] {
	return genkit.DefineTool(
		g, "write", "Edit a file by replacing or inserting text.",
		func(ctx *ai.ToolContext, input editInput) (editOutput, error) {
			// Determine operation type for display
			op := "replace"
			switch {
			case input.InsertLine > 0:
				op = fmt.Sprintf("insert@L%d", input.InsertLine)
			case input.Append:
				op = "append"
			case input.InsertAfter:
				op = "insert-after"
			case input.ReplaceAll:
				op = "replace-all"
			}
			fmt.Printf("→ write %s [%s]\n", input.Path, op)

			out, err := performEdit(input, s)
			if err != nil {
				// Return error to LLM in output, not as tool error
				out.Success = false
				out.Message = err.Error()
				return out, nil
			}
			return out, nil
		})
}

type editInput struct {
	Path        string `json:"path" jsonschema_description:"Path to the file to edit"`
	OldString   string `json:"old_string,omitempty" jsonschema_description:"For replace/insert_after: the anchor text. Not needed for insert_line/append."`
	NewString   string `json:"new_string" jsonschema_description:"The new text to insert or replace with"`
	ReplaceAll  bool   `json:"replace_all,omitempty" jsonschema_description:"Replace all occurrences (default: false, replaces first only)"`
	InsertAfter bool   `json:"insert_after,omitempty" jsonschema_description:"Insert new_string after old_string instead of replacing. Old_string is kept."`
	InsertLine  int    `json:"insert_line,omitempty" jsonschema_description:"Insert at this line number (1-indexed). 1 = beginning, large number = end."`
	Append      bool   `json:"append,omitempty" jsonschema_description:"Append new_string to end of file (alternative to large insert_line)"`
}

type editOutput struct {
	Success        bool   `json:"success"`
	Message        string `json:"message"`
	Diff           string `json:"diff,omitempty"`
	RequiresReread bool   `json:"requires_reread,omitempty"`
}

func performEdit(input editInput, s *state.State) (editOutput, error) {
	var out editOutput

	if input.Path == "" {
		return out, fmt.Errorf("path is required")
	}
	if input.NewString == "" {
		return out, fmt.Errorf("new_string is required")
	}

	// Read file first to avoid TOCTOU race condition
	content, err := os.ReadFile(input.Path)
	if err != nil {
		return out, fmt.Errorf("cannot read file: %w", err)
	}

	// Get file info to preserve permissions
	fileInfo, err := os.Stat(input.Path)
	if err != nil {
		return out, fmt.Errorf("cannot stat file: %w", err)
	}

	// Validate against snapshot after reading (prevents race condition)
	if s.FileTracker != nil {
		changed, hasSnapshot, err := s.FileTracker.CheckContent(input.Path, content)
		if err != nil {
			return out, fmt.Errorf("cannot check file state: %w", err)
		}
		if changed {
			out.RequiresReread = true
			if hasSnapshot {
				return out, fmt.Errorf("FILE CHANGED: file modified after last read, use cat -n %s to see current content", input.Path)
			}
		}
	}

	modes := 0
	if input.InsertLine > 0 {
		modes++
	}
	if input.Append {
		modes++
	}
	if input.InsertAfter {
		modes++
	}
	if input.OldString != "" && !input.InsertAfter && input.InsertLine == 0 && !input.Append {
		modes++
	}
	if modes > 1 {
		return out, fmt.Errorf("only one operation mode allowed")
	}

	original := string(content)
	var newContent string
	var msg string

	switch {
	case input.InsertLine > 0 || input.Append:
		lines := strings.Split(original, "\n")
		insertAt := input.InsertLine
		if input.Append || insertAt > len(lines) {
			insertAt = len(lines)
			if insertAt == 0 {
				insertAt = 1
			}
		}
		if insertAt < 1 {
			insertAt = 1
		}
		idx := insertAt - 1
		if idx > len(lines) {
			idx = len(lines)
		}
		newLines := strings.Split(input.NewString, "\n")
		result := make([]string, 0, len(lines)+len(newLines))
		result = append(result, lines[:idx]...)
		result = append(result, newLines...)
		result = append(result, lines[idx:]...)
		newContent = strings.Join(result, "\n")
		if input.Append || (input.InsertLine > 0 && input.InsertLine >= len(lines)) {
			msg = fmt.Sprintf("Appended %d lines", len(newLines))
		} else {
			msg = fmt.Sprintf("Inserted %d lines at line %d", len(newLines), insertAt)
		}

	case input.InsertAfter:
		if input.OldString == "" {
			return out, fmt.Errorf("old_string required")
		}
		if !strings.Contains(original, input.OldString) {
			return out, fmt.Errorf("old_string not found")
		}
		count := strings.Count(original, input.OldString)
		if count > 1 {
			return out, fmt.Errorf("old_string appears %d times", count)
		}
		idx := strings.Index(original, input.OldString)
		insertPoint := idx + len(input.OldString)
		newContent = original[:insertPoint] + input.NewString + original[insertPoint:]
		msg = "Inserted after anchor"

	default:
		if input.OldString == "" {
			return out, fmt.Errorf("old_string required")
		}
		if !strings.Contains(original, input.OldString) {
			return out, fmt.Errorf("old_string not found")
		}
		count := strings.Count(original, input.OldString)
		if !input.ReplaceAll && count > 1 {
			return out, fmt.Errorf("old_string appears %d times, use replace_all", count)
		}
		if input.ReplaceAll {
			newContent = strings.ReplaceAll(original, input.OldString, input.NewString)
			msg = fmt.Sprintf("Replaced %d occurrences", count)
		} else {
			newContent = strings.Replace(original, input.OldString, input.NewString, 1)
			msg = "Replaced 1 occurrence"
		}
	}

	if newContent == original {
		return out, fmt.Errorf("no changes")
	}

	// Preserve original file permissions
	if err := os.WriteFile(input.Path, []byte(newContent), fileInfo.Mode()); err != nil {
		return out, fmt.Errorf("cannot write: %w", err)
	}

	if s.FileTracker != nil {
		s.FileTracker.RecordSnapshot(input.Path, []byte(newContent))
	}

	out.Success = true
	out.Message = msg
	out.Diff = generateDiff(original, newContent)
	return out, nil
}

func generateDiff(oldContent, newContent string) string {
	var b strings.Builder
	oldLines := strings.Split(oldContent, "\n")
	newLines := strings.Split(newContent, "\n")
	maxLines := len(oldLines)
	if len(newLines) > maxLines {
		maxLines = len(newLines)
	}
	changed := 0
	for i := 0; i < maxLines && changed < 20; i++ {
		oldLine, newLine := "", ""
		if i < len(oldLines) {
			oldLine = oldLines[i]
		}
		if i < len(newLines) {
			newLine = newLines[i]
		}
		if oldLine != newLine {
			b.WriteString(fmt.Sprintf("-%s\n+%s\n", oldLine, newLine))
			changed++
		}
	}
	if changed >= 20 {
		b.WriteString("...\n")
	}
	return b.String()
}
