package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type Store struct {
	dir string
	mu  sync.Mutex
}

func NewStore(dir string) (*Store, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, errors.New("data directory is required")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	return &Store{dir: dir}, nil
}

func (s *Store) Load(userID string) (*Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadUnlocked(safeUserID(userID))
}

func (s *Store) Update(userID string, update func(*Task) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	fileID := safeUserID(userID)
	root, err := s.loadUnlocked(fileID)
	if err != nil {
		return err
	}
	if err := update(root); err != nil {
		return err
	}
	return s.saveUnlocked(fileID, normalizeTree(root))
}

func (s *Store) Restore(userID string, data []byte) error {
	var root Task
	if err := json.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("backup is not valid task JSON: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveUnlocked(safeUserID(userID), normalizeTree(&root))
}

func (s *Store) loadUnlocked(fileID string) (*Task, error) {
	data, err := os.ReadFile(s.path(fileID))
	if errors.Is(err, os.ErrNotExist) {
		return defaultRoot(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read tasks: %w", err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return defaultRoot(), nil
	}

	var root Task
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("parse tasks: %w", err)
	}
	return normalizeTree(&root), nil
}

func (s *Store) saveUnlocked(fileID string, root *Task) error {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}

	temp, err := os.CreateTemp(s.dir, fileID+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary task file: %w", err)
	}
	tempName := temp.Name()
	defer os.Remove(tempName)

	encoder := json.NewEncoder(temp)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(root); err != nil {
		temp.Close()
		return fmt.Errorf("encode tasks: %w", err)
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return fmt.Errorf("sync task file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close task file: %w", err)
	}
	if err := os.Rename(tempName, s.path(fileID)); err != nil {
		return fmt.Errorf("replace task file: %w", err)
	}
	return nil
}

func (s *Store) path(fileID string) string {
	return filepath.Join(s.dir, fileID+".json")
}

func safeUserID(userID string) string {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		userID = defaultUserID
	}
	if isSafeFileID(userID) {
		return userID
	}

	sum := sha256.Sum256([]byte(userID))
	return "user-" + hex.EncodeToString(sum[:])
}

func isSafeFileID(value string) bool {
	if value == "." || value == ".." {
		return false
	}
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-', r == '_', r == '.':
		default:
			return false
		}
	}
	return value != ""
}
