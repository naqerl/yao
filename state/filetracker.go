package state

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"sync"
)

// FileTracker tracks file content to detect external modifications.
type FileTracker struct {
	mu        sync.RWMutex
	snapshots map[string]string // path -> content hash
}

// NewFileTracker creates a new file tracker.
func NewFileTracker() *FileTracker {
	return &FileTracker{
		snapshots: make(map[string]string),
	}
}

// RecordSnapshot records the current hash of a file.
// Call this after reading a file or after successfully editing it.
func (ft *FileTracker) RecordSnapshot(path string, content []byte) {
	ft.mu.Lock()
	defer ft.mu.Unlock()
	hash := sha256.Sum256(content)
	ft.snapshots[path] = hex.EncodeToString(hash[:])
}

// RecordSnapshotFromFile reads the file and records its snapshot.
func (ft *FileTracker) RecordSnapshotFromFile(path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	ft.RecordSnapshot(path, content)
	return nil
}

// HasChanged checks if the file has been modified since the last snapshot.
// Returns true if the file has changed or if there's no snapshot recorded.
func (ft *FileTracker) HasChanged(path string) (bool, error) {
	ft.mu.RLock()
	storedHash, exists := ft.snapshots[path]
	ft.mu.RUnlock()

	if !exists {
		// No snapshot recorded - consider as "changed" to force initial read
		return true, nil
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("cannot read file to check for changes: %w", err)
	}

	hash := sha256.Sum256(content)
	currentHash := hex.EncodeToString(hash[:])

	return currentHash != storedHash, nil
}

// GetChangeInfo returns detailed info about what changed.
func (ft *FileTracker) GetChangeInfo(path string) (changed bool, hasSnapshot bool, err error) {
	ft.mu.RLock()
	storedHash, exists := ft.snapshots[path]
	ft.mu.RUnlock()

	if !exists {
		return true, false, nil
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return false, true, err
	}

	hash := sha256.Sum256(content)
	currentHash := hex.EncodeToString(hash[:])

	return currentHash != storedHash, true, nil
}
