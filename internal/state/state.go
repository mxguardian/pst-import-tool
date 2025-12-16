package state

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/bits-and-blooms/bloom/v3"
)

const (
	// Default capacity for bloom filter (100,000 messages)
	// Can handle most PST files; larger ones just have slightly higher false positive rate
	defaultBloomCapacity = 100_000
	// Target false positive rate (0.1% = 1 in 1,000)
	// Acceptable for resume: worst case is skipping a message that wasn't uploaded
	defaultFalsePositiveRate = 0.001
)

// ImportState tracks the progress of a PST import for resume capability
type ImportState struct {
	PSTPath         string          `json:"pst_path"`
	PSTHash         string          `json:"pst_hash"`          // SHA256 of first 1MB of PST
	Username        string          `json:"username"`          // IMAP username
	BloomData       string          `json:"bloom_data"`        // Base64-encoded bloom filter
	UploadedCount   int             `json:"uploaded_count"`
	TotalCount      int             `json:"total_count"`
	CompletedFolder map[string]bool `json:"completed_folders"` // Folders fully uploaded

	// Runtime fields (not serialized)
	bloomFilter *bloom.BloomFilter
	statePath   string
	isResuming  bool // True if we loaded existing progress
	mu          sync.Mutex
}

// NewImportState creates a new import state for a PST file
func NewImportState(pstPath, username string) (*ImportState, error) {
	absPath, err := filepath.Abs(pstPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	hash, err := hashPSTFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to hash PST file: %w", err)
	}

	state := &ImportState{
		PSTPath:         absPath,
		PSTHash:         hash,
		Username:        username,
		CompletedFolder: make(map[string]bool),
		bloomFilter:     bloom.NewWithEstimates(defaultBloomCapacity, defaultFalsePositiveRate),
	}

	// State file is stored next to the PST file
	state.statePath = absPath + ".import-state.json"

	return state, nil
}

// Load loads existing state from disk if available
func (s *ImportState) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.statePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No existing state, starting fresh
		}
		return fmt.Errorf("failed to read state file: %w", err)
	}

	var loaded ImportState
	if err := json.Unmarshal(data, &loaded); err != nil {
		return fmt.Errorf("failed to parse state file: %w", err)
	}

	// Verify the state matches this PST file and user
	if loaded.PSTHash != s.PSTHash {
		// PST file has changed, ignore old state
		return nil
	}
	if loaded.Username != s.Username {
		// Different user, ignore old state
		return nil
	}

	// Restore bloom filter from base64 data
	if loaded.BloomData != "" {
		bloomBytes, err := base64.StdEncoding.DecodeString(loaded.BloomData)
		if err != nil {
			return fmt.Errorf("failed to decode bloom filter: %w", err)
		}

		s.bloomFilter = &bloom.BloomFilter{}
		if err := s.bloomFilter.UnmarshalBinary(bloomBytes); err != nil {
			return fmt.Errorf("failed to unmarshal bloom filter: %w", err)
		}
	}

	// Restore other state
	s.UploadedCount = loaded.UploadedCount
	s.TotalCount = loaded.TotalCount
	s.CompletedFolder = loaded.CompletedFolder

	if s.CompletedFolder == nil {
		s.CompletedFolder = make(map[string]bool)
	}

	// Mark that we're resuming a previous import
	if s.UploadedCount > 0 || len(s.CompletedFolder) > 0 {
		s.isResuming = true
	}

	return nil
}

// Save persists the current state to disk
func (s *ImportState) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Serialize bloom filter to base64
	bloomBytes, err := s.bloomFilter.MarshalBinary()
	if err != nil {
		return fmt.Errorf("failed to marshal bloom filter: %w", err)
	}
	s.BloomData = base64.StdEncoding.EncodeToString(bloomBytes)

	data, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("failed to serialize state: %w", err)
	}

	// Write to temp file first, then rename for atomic update
	tmpPath := s.statePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write state file: %w", err)
	}

	if err := os.Rename(tmpPath, s.statePath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to save state file: %w", err)
	}

	return nil
}

// MarkUploaded marks a message as uploaded
func (s *ImportState) MarkUploaded(messageID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.bloomFilter.TestString(messageID) {
		s.bloomFilter.AddString(messageID)
		s.UploadedCount++
	}
}

// IsUploaded checks if a message has already been uploaded
// Note: May return false positives (bloom filter property) but never false negatives
// Optimization: returns false immediately if not resuming (no prior state to check)
func (s *ImportState) IsUploaded(messageID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	// On fresh import, nothing has been uploaded yet
	if !s.isResuming {
		return false
	}

	return s.bloomFilter.TestString(messageID)
}

// MarkFolderComplete marks a folder as fully uploaded
func (s *ImportState) MarkFolderComplete(folderName string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.CompletedFolder[folderName] = true
}

// IsFolderComplete checks if a folder has been fully uploaded
// Optimization: returns false immediately if not resuming
func (s *ImportState) IsFolderComplete(folderName string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.isResuming {
		return false
	}

	return s.CompletedFolder[folderName]
}

// SetTotal sets the total message count
func (s *ImportState) SetTotal(total int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.TotalCount = total
}

// GetProgress returns current progress
func (s *ImportState) GetProgress() (uploaded, total int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.UploadedCount, s.TotalCount
}

// HasExistingProgress returns true if there's resumable progress
func (s *ImportState) HasExistingProgress() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.UploadedCount > 0
}

// Clear removes the state file
func (s *ImportState) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	err := os.Remove(s.statePath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	// Reset in-memory state
	s.bloomFilter = bloom.NewWithEstimates(defaultBloomCapacity, defaultFalsePositiveRate)
	s.UploadedCount = 0
	s.CompletedFolder = make(map[string]bool)

	return nil
}

// hashPSTFile returns a SHA256 hash of the first 1MB of the PST file
// This is fast even for large files and sufficient to detect changes
func hashPSTFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	// Read first 1MB
	_, err = io.CopyN(h, f, 1024*1024)
	if err != nil && err != io.EOF {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// StatePath returns the path to the state file
func (s *ImportState) StatePath() string {
	return s.statePath
}

// BloomStats returns statistics about the bloom filter for debugging
func (s *ImportState) BloomStats() (capacity uint, sizeBytes int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, _ := s.bloomFilter.MarshalBinary()
	return s.bloomFilter.Cap(), len(data)
}
