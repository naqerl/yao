package tool

import (
	"fmt"
	"os"
	"strings"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"

	"github.com/naqerl/yao/state"
)

// writeOutput is the tool's response structure, defined here for clean separation.
type writeOutput struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// DefineWrite defines the write tool on the given genkit instance.
func DefineWrite(g *genkit.Genkit, s *state.State) *ai.ToolDef[writeInput, writeOutput] {
	return genkit.DefineTool(
		g, "write", "Write a file by replacing or inserting text.",
		func(ctx *ai.ToolContext, input writeInput) (writeOutput, error) {
			msg, stats, err := performWrite(input, s)
			var out writeOutput
			if err != nil {
				out.Message = err.Error()
				fmt.Printf("→ write %s (error)\n", input.Path)
			} else {
				out.Success = true
				out.Message = msg
				fmt.Printf("→ write %s %s\n", input.Path, stats)
			}

			return out, nil
		})
}

type writeInput struct {
	Path        string `json:"path" jsonschema_description:"Path to the file to write"`
	OldString   string `json:"old_string,omitempty" jsonschema_description:"For replace/insert_after: the anchor text. Not needed for insert_line/append."`
	NewString   string `json:"new_string" jsonschema_description:"The new text to insert or replace with"`
	ReplaceAll  bool   `json:"replace_all,omitempty" jsonschema_description:"Replace all occurrences (default: false, replaces first only)"`
	InsertAfter bool   `json:"insert_after,omitempty" jsonschema_description:"Insert new_string after old_string instead of replacing. Old_string is kept."`
	InsertLine  int    `json:"insert_line,omitempty" jsonschema_description:"Insert at this line number (1-indexed). 1 = beginning."`
	Append      bool   `json:"append,omitempty" jsonschema_description:"Append new_string to end of file"`
}

// validateOperationMode ensures exactly one operation mode is specified.
// Priority: Append > InsertAfter > InsertLine > Replace
func validateOperationMode(input writeInput) error {
	hasAppend := input.Append
	hasInsertAfter := input.InsertAfter
	hasOldString := input.OldString != ""
	// InsertLine >= 1 with 1 meaning "insert at beginning"
	// We distinguish "not set" (default 0) from "explicitly 1" by checking if other modes are unset
	hasInsertLine := input.InsertLine > 1 || (input.InsertLine == 1 && !hasAppend && !hasInsertAfter && !hasOldString)

	modes := 0
	if hasAppend {
		modes++
	}
	if hasInsertAfter {
		modes++
	}
	if hasInsertLine {
		modes++
	}
	if hasOldString && !hasInsertAfter {
		modes++
	}

	if modes == 0 {
		return fmt.Errorf("no operation mode specified: provide old_string, insert_line, append, or insert_after")
	}
	if modes > 1 {
		return fmt.Errorf("multiple operation modes specified: only one of old_string/replace, insert_line, append, or insert_after allowed")
	}
	return nil
}

// performWrite executes the write operation and returns a message or error.
// The calling tool definition constructs the writeOutput for clean separation.
func performWrite(input writeInput, s *state.State) (string, string, error) {
	// Read current file content
	content, err := os.ReadFile(input.Path)
	if err != nil {
		return "", "", fmt.Errorf("cannot read file: %w", err)
	}

	// Get file info to preserve permissions
	fileInfo, err := os.Stat(input.Path)
	if err != nil {
		return "", "", fmt.Errorf("cannot stat file: %w", err)
	}

	// Validate against snapshot after reading (prevents race condition)
	if s.FileTracker != nil {
		changed, _, err := s.FileTracker.CheckContent(input.Path, content)
		if err != nil {
			return "", "", fmt.Errorf("cannot check file state: %w", err)
		}
		if changed {
			return "", "", fmt.Errorf("FILE CHANGED: file modified after last read, use cat -n %s to see current content", input.Path)
		}
	}

	// Validate exactly one operation mode is specified
	if err := validateOperationMode(input); err != nil {
		return "", "", err
	}

	original := string(content)
	oldLines := strings.Split(original, "\n")
	var newContent, msg string
	removed, added := 0, 0

	switch {
	case input.InsertLine >= 1 || input.Append:
		// Convert 1-indexed InsertLine to 0-indexed insert position
		insertAt := input.InsertLine - 1
		if input.Append {
			insertAt = len(oldLines)
		}
		// Clamp to valid range
		if insertAt < 0 {
			insertAt = 0
		}
		if insertAt > len(oldLines) {
			insertAt = len(oldLines)
		}
		newLines := strings.Split(input.NewString, "\n")

		// Ensure proper newline handling when appending to file without trailing newline
		lines := oldLines
		if input.Append && len(lines) > 0 && len(lines[len(lines)-1]) > 0 {
			lines[len(lines)-1] += "\n"
		}

		result := insertLinesAt(lines, insertAt, newLines)
		newContent = strings.Join(result, "\n")

		added = len(newLines)
		// For insertion, nothing is removed (we're adding between existing lines)
		removed = 0

		if input.Append {
			msg = fmt.Sprintf("Appended %d lines", len(newLines))
		} else {
			msg = fmt.Sprintf("Inserted %d lines at line %d", len(newLines), input.InsertLine)
		}

	case input.InsertAfter:
		if input.OldString == "" {
			return "", "", fmt.Errorf("old_string is empty: provide the exact anchor text from the file")
		}
		if !strings.Contains(original, input.OldString) {
			return "", "", fmt.Errorf("old_string not found in %s: the anchor text may have changed from a previous edit; use read tool to see current content and update your old_string", input.Path)
		}
		count := strings.Count(original, input.OldString)
		if count > 1 {
			return "", "", fmt.Errorf("old_string appears %d times", count)
		}
		idx := strings.Index(original, input.OldString)
		insertPoint := idx + len(input.OldString)
		newContent = original[:insertPoint] + input.NewString + original[insertPoint:]
		msg = "Inserted after anchor"
		// For insert_after, we add new lines but don't remove anything
		added = len(strings.Split(input.NewString, "\n"))
		removed = 0

	default:
		if input.OldString == "" {
			return "", "", fmt.Errorf("old_string is empty: provide the exact anchor text from the file")
		}
		if !strings.Contains(original, input.OldString) {
			return "", "", fmt.Errorf("old_string not found in %s: the anchor text may have changed from a previous edit; use read tool to see current content and update your old_string", input.Path)
		}
		count := strings.Count(original, input.OldString)
		if !input.ReplaceAll && count > 1 {
			return "", "", fmt.Errorf("old_string appears %d times, use replace_all", count)
		}
		if input.ReplaceAll {
			newContent = strings.ReplaceAll(original, input.OldString, input.NewString)
			msg = fmt.Sprintf("Replaced %d occurrences", count)
			// For replace_all, count all occurrences
			added = len(strings.Split(input.NewString, "\n")) * count
			removed = len(strings.Split(input.OldString, "\n")) * count
		} else {
			newContent = strings.Replace(original, input.OldString, input.NewString, 1)
			msg = "Replaced 1 occurrence"
			added = len(strings.Split(input.NewString, "\n"))
			removed = len(strings.Split(input.OldString, "\n"))
		}
	}

	if newContent == original {
		return "", "", fmt.Errorf("no changes")
	}

	// Preserve original file permissions
	if err := os.WriteFile(input.Path, []byte(newContent), fileInfo.Mode()); err != nil {
		return "", "", fmt.Errorf("cannot write: %w", err)
	}

	s.FileTracker.RecordSnapshot(input.Path, []byte(newContent))

	stats := formatDiffStats(removed, added)

	return msg, stats, nil
}

// insertLinesAt inserts newLines at position at in the original slice.
func insertLinesAt(original []string, at int, newLines []string) []string {
	result := make([]string, 0, len(original)+len(newLines))
	result = append(result, original[:at]...)
	result = append(result, newLines...)
	result = append(result, original[at:]...)
	return result
}

// formatDiffStats returns a git-style line change summary like "+5/-3" or "+10"
// Shows actual lines added and removed (not just net change)
func formatDiffStats(removed, added int) string {
	if removed == 0 && added == 0 {
		return "0"
	}
	if removed == 0 {
		return fmt.Sprintf("+%d", added)
	}
	if added == 0 {
		return fmt.Sprintf("-%d", removed)
	}
	return fmt.Sprintf("+%d/-%d", added, removed)
}
