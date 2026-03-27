package system

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

//go:embed default.sh
var defaultScript string

func Eval(path string) (string, error) {
	path = filepath.Clean(path)
	cmd := exec.Command("bash", path)

	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	err := cmd.Run()
	out := output.String()
	if err != nil {
		return out, fmt.Errorf("script execution failed: %w\nOutput:\n%s", err, out)
	}

	return out, nil
}

func Default() (string, error) {
	tmpFile, err := os.CreateTemp("", "yao-system-*.sh")
	if err != nil {
		return "", fmt.Errorf("create temp system script: %w", err)
	}

	name := tmpFile.Name()
	cleanup := func() {
		_ = os.Remove(name)
	}
	defer cleanup()

	if _, err := tmpFile.WriteString(defaultScript); err != nil {
		_ = tmpFile.Close()
		return "", fmt.Errorf("write temp system script: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return "", fmt.Errorf("close temp system script: %w", err)
	}
	if err := os.Chmod(name, 0o700); err != nil {
		return "", fmt.Errorf("chmod temp system script: %w", err)
	}

	return Eval(name)
}
