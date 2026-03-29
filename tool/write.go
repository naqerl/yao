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
		g, "write", `Write or edit a file.

Modes:
- "replace" (default): Replace old_string with new_string. Requires old_string.
- "overwrite": Write content as the entire file. Replaces all existing content.
- "append": Add content to the end of the file.

For surgical edits (replace mode), use old_string/new_string.
For full file writes (overwrite mode), use content.
For adding to end (append mode), use content.`,
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
	Mode        string `json:"mode,omitempty" jsonschema_description:"Operation mode: 'replace' (default), 'overwrite', or 'append'"`
	Content     string `json:"content,omitempty" jsonschema_description:"For overwrite/append: the full content to write"`
	OldString   string `json:"old_string,omitempty" jsonschema_description:"For replace mode: the anchor text to replace"`
	NewString   string `json:"new_string,omitempty" jsonschema_description:"For replace mode: the replacement text"`
	ReplaceAll  bool   `json:"replace_all,omitempty" jsonschema_description:"Replace all occurrences (default: false, replaces first only)"`
	InsertAfter bool   `json:"insert_after,omitempty" jsonschema_description:"Insert new_string after old_string instead of replacing"`
	InsertLine  int    `json:"insert_line,omitempty" jsonschema_description:"Insert at this line number (1-indexed). 1 = beginning"`
}

// performWrite executes the write operation based on mode.
func performWrite(input writeInput, s *state.State) (string, string, error) {
	// Normalize mode - default to "replace" for backward compatibility
	if input.Mode == "" {
		input.Mode = "replace"
	}

	switch input.Mode {
	case "overwrite":
		return performOverwrite(input, s)
	case "append":
		return performAppend(input, s)
	case "replace":
		return performReplace(input, s)
	default:
		return "", "", fmt.Errorf("invalid mode: %s (use 'replace', 'overwrite', or 'append')", input.Mode)
	}
}

// performOverwrite writes content as the entire file.
func performOverwrite(input writeInput, s *state.State) (string, string, error) {
	// For overwrite, we don't need to read existing content (except for stats)
	var oldLines []string
	fileInfo, err := os.Stat(input.Path)
	if err == nil {
		// File exists, read it for stats
		content, _ := os.ReadFile(input.Path)
		oldLines = strings.Split(string(content), "\n")
	} else {
		// New file
		fileInfo = nil
	}

	// Validate against snapshot if file exists
	if s.FileTracker != nil && fileInfo != nil {
		content, _ := os.ReadFile(input.Path)
		changed, _, _ := s.FileTracker.CheckContent(input.Path, content)
		if changed {
			return "", "", fmt.Errorf("FILE CHANGED: file modified after last read, use cat -n %s to see current content", input.Path)
		}
	}

	newLines := strings.Split(input.Content, "\n")
	removed := len(oldLines)
	added := len(newLines)

	// Determine file mode for new files
	mode := os.FileMode(0644)
	if fileInfo != nil {
		mode = fileInfo.Mode()
	}

	if err := os.WriteFile(input.Path, []byte(input.Content), mode); err != nil {
		return "", "", fmt.Errorf("cannot write file: %w", err)
	}

	s.FileTracker.RecordSnapshot(input.Path, []byte(input.Content))

	msg := "File overwritten"
	if fileInfo == nil {
		msg = "File created"
		removed = 0
	}

	return msg, formatDiffStats(removed, added), nil
}

// performAppend adds content to the end of the file.
func performAppend(input writeInput, s *state.State) (string, string, error) {
	content, err := os.ReadFile(input.Path)
	if err != nil {
		return "", "", fmt.Errorf("cannot read file: %w", err)
	}

	// Validate against snapshot
	if s.FileTracker != nil {
		changed, _, _ := s.FileTracker.CheckContent(input.Path, content)
		if changed {
			return "", "", fmt.Errorf("FILE CHANGED: file modified after last read, use cat -n %s to see current content", input.Path)
		}
	}

	fileInfo, err := os.Stat(input.Path)
	if err != nil {
		return "", "", fmt.Errorf("cannot stat file: %w", err)
	}

	appendLines := strings.Split(input.Content, "\n")

	// Ensure newline before append if file doesn't end with one
	separator := ""
	if len(content) > 0 && !strings.HasSuffix(string(content), "\n") {
		separator = "\n"
	}

	newContent := string(content) + separator + input.Content

	removed := 0
	added := len(appendLines)
	if separator != "" {
		added++ // Count the separator newline
	}

	if err := os.WriteFile(input.Path, []byte(newContent), fileInfo.Mode()); err != nil {
		return "", "", fmt.Errorf("cannot write file: %w", err)
	}

	s.FileTracker.RecordSnapshot(input.Path, []byte(newContent))

	return fmt.Sprintf("Appended %d lines", len(appendLines)), formatDiffStats(removed, added), nil
}

// performReplace handles old_string/new_string replacement and other legacy modes.
func performReplace(input writeInput, s *state.State) (string, string, error) {
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

	// Validate against snapshot after reading
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
	if err := validateReplaceMode(input); err != nil {
		return "", "", err
	}

	original := string(content)
	oldLines := strings.Split(original, "\n")
	var newContent, msg string
	removed, added := 0, 0

	switch {
	case input.InsertLine >= 1:
		// Convert 1-indexed InsertLine to 0-indexed insert position
		insertAt := input.InsertLine - 1
		if insertAt < 0 {
			insertAt = 0
		}
		if insertAt > len(oldLines) {
			insertAt = len(oldLines)
		}
		newLinesList := strings.Split(input.NewString, "\n")

		result := insertLinesAt(oldLines, insertAt, newLinesList)
		newContent = strings.Join(result, "\n")

		added = len(newLinesList)
		removed = 0
		msg = fmt.Sprintf("Inserted %d lines at line %d", len(newLinesList), input.InsertLine)

	case input.InsertAfter:
		if input.OldString == "" {
			return "", "", fmt.Errorf("old_string is empty: provide the exact anchor text from the file")
		}
		if !strings.Contains(original, input.OldString) {
			return "", "", fmt.Errorf("old_string not found in %s: the file was probably changed, call read tool to see current content and update your old_string", input.Path)
		}
		count := strings.Count(original, input.OldString)
		if count > 1 {
			return "", "", fmt.Errorf("old_string appears %d times", count)
		}
		idx := strings.Index(original, input.OldString)
		insertPoint := idx + len(input.OldString)
		newContent = original[:insertPoint] + input.NewString + original[insertPoint:]
		msg = "Inserted after anchor"
		added = len(strings.Split(input.NewString, "\n"))
		removed = 0

	default:
		// Standard replace mode
		if input.OldString == "" {
			return "", "", fmt.Errorf("old_string is empty: provide the exact anchor text from the file")
		}
		if !strings.Contains(original, input.OldString) {
			return "", "", fmt.Errorf("old_string not found in %s: the file was probably changed, call read tool to see current content and update your old_string", input.Path)
		}
		count := strings.Count(original, input.OldString)
		if !input.ReplaceAll && count > 1 {
			return "", "", fmt.Errorf("old_string appears %d times, use replace_all", count)
		}
		if input.ReplaceAll {
			newContent = strings.ReplaceAll(original, input.OldString, input.NewString)
			msg = fmt.Sprintf("Replaced %d occurrences", count)
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

	return msg, formatDiffStats(removed, added), nil
}

// validateReplaceMode ensures exactly one replace sub-mode is specified.
func validateReplaceMode(input writeInput) error {
	hasInsertAfter := input.InsertAfter
	hasOldString := input.OldString != ""
	hasInsertLine := input.InsertLine >= 1
	hasNewString := input.NewString != ""

	// Count modes
	modes := 0
	if hasInsertLine {
		modes++
	}
	if hasInsertAfter {
		modes++
	}
	if hasOldString && !hasInsertAfter {
		modes++
	}

	if modes == 0 {
		return fmt.Errorf("no replace mode specified: provide old_string, insert_line, or insert_after")
	}
	if modes > 1 {
		return fmt.Errorf("multiple modes specified: only one of old_string/replace, insert_line, or insert_after allowed")
	}

	// Validate new_string is provided when needed
	if hasInsertLine || hasInsertAfter || (hasOldString && !hasInsertAfter) {
		if !hasNewString {
			return fmt.Errorf("new_string is required for the specified operation")
		}
	}

	return nil
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
