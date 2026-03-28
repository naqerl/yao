package state

import (
	"crypto/sha256"
	"encoding/hex"
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
	changed, _, err := ft.GetChangeInfo(path)
	return changed, err
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

// CheckContent checks if the provided content matches the stored snapshot.
// This is used to avoid TOCTOU race conditions by checking content after reading.
// Returns true if content has changed compared to snapshot, or if no snapshot exists.
func (ft *FileTracker) CheckContent(path string, content []byte) (changed bool, hasSnapshot bool, err error) {
	ft.mu.RLock()
	storedHash, exists := ft.snapshots[path]
	ft.mu.RUnlock()

	if !exists {
		return true, false, nil
	}

	hash := sha256.Sum256(content)
	currentHash := hex.EncodeToString(hash[:])

	return currentHash != storedHash, true, nil
}
