package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

const blobDirName = "blobs"

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

// RestoreArchive restores a zip backup produced by downloadAll: blob content
// lands in the blob store first, then tasks.json replaces the user's tree.
// Every blob entry must hash to its content-address name, so a corrupted
// archive fails before any attachment record can point at wrong bytes.
func (s *Store) RestoreArchive(userID string, archive io.ReaderAt, size int64) error {
	reader, err := zip.NewReader(archive, size)
	if err != nil {
		return fmt.Errorf("backup is not a readable zip archive: %w", err)
	}
	var tasksJSON []byte
	for _, entry := range reader.File {
		name := path.Clean(entry.Name)
		switch {
		case name == "tasks.json":
			tasksJSON, err = readArchiveEntry(entry, maxRestoreBytes)
			if err != nil {
				return err
			}
		case strings.HasPrefix(name, blobDirName+"/"):
			if err := s.restoreArchiveBlob(entry); err != nil {
				return err
			}
		}
	}
	if tasksJSON == nil {
		return errors.New("backup archive does not contain tasks.json")
	}
	return s.Restore(userID, tasksJSON)
}

func readArchiveEntry(entry *zip.File, limit int64) ([]byte, error) {
	file, err := entry.Open()
	if err != nil {
		return nil, fmt.Errorf("open backup entry %s: %w", entry.Name, err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read backup entry %s: %w", entry.Name, err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("backup entry %s exceeds the %d byte limit", entry.Name, limit)
	}
	return data, nil
}

func (s *Store) restoreArchiveBlob(entry *zip.File) error {
	ref := path.Base(path.Clean(entry.Name))
	if !isBlobRef(ref) {
		return fmt.Errorf("backup blob %q does not have a content-address name", entry.Name)
	}
	file, err := entry.Open()
	if err != nil {
		return fmt.Errorf("open backup blob %s: %w", entry.Name, err)
	}
	defer file.Close()
	stored, _, err := s.SaveBlob(file)
	if err != nil {
		return fmt.Errorf("restore backup blob %s: %w", entry.Name, err)
	}
	if stored != ref {
		return fmt.Errorf("backup blob %s content hashes to %s; the archive is corrupt", ref, stored)
	}
	return nil
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

// SaveBlob stores content under its SHA-256 and returns the reference.
// Identical content shares one file, so writes need no lock: the rename is
// atomic and any concurrent writer produces the same bytes.
func (s *Store) SaveBlob(content io.Reader) (ref string, size int64, err error) {
	blobDir := filepath.Join(s.dir, blobDirName)
	if err := os.MkdirAll(blobDir, 0o700); err != nil {
		return "", 0, fmt.Errorf("create blob directory: %w", err)
	}

	temp, err := os.CreateTemp(blobDir, "blob.*.tmp")
	if err != nil {
		return "", 0, fmt.Errorf("create temporary blob file: %w", err)
	}
	tempName := temp.Name()
	defer os.Remove(tempName)

	hash := sha256.New()
	size, err = io.Copy(io.MultiWriter(temp, hash), content)
	if err != nil {
		temp.Close()
		return "", 0, fmt.Errorf("write blob: %w", err)
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return "", 0, fmt.Errorf("sync blob: %w", err)
	}
	if err := temp.Close(); err != nil {
		return "", 0, fmt.Errorf("close blob: %w", err)
	}

	ref = hex.EncodeToString(hash.Sum(nil))
	final := s.blobPath(ref)
	if _, err := os.Stat(final); err == nil {
		return ref, size, nil
	}
	if err := os.Rename(tempName, final); err != nil {
		return "", 0, fmt.Errorf("store blob: %w", err)
	}
	return ref, size, nil
}

// OpenBlob opens stored blob content; the caller must close the file.
func (s *Store) OpenBlob(ref string) (*os.File, os.FileInfo, error) {
	if !isBlobRef(ref) {
		return nil, nil, fmt.Errorf("invalid blob reference %q", ref)
	}
	file, err := os.Open(s.blobPath(ref))
	if err != nil {
		return nil, nil, fmt.Errorf("open blob: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, nil, fmt.Errorf("stat blob: %w", err)
	}
	return file, info, nil
}

func (s *Store) blobPath(ref string) string {
	return filepath.Join(s.dir, blobDirName, ref)
}

// BlobInfo describes one stored blob file for backup and cleanup listings.
type BlobInfo struct {
	Ref     string
	Size    int64
	ModTime time.Time
}

// UnreferencedBlobs lists blob files that no attachment record in any stored
// task tree references. Blobs newer than minAge are never reported: an upload
// stores its blob before the attachment record lands, so a fresh blob may be
// referenced a moment later.
func (s *Store) UnreferencedBlobs(minAge time.Duration) ([]BlobInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.unreferencedBlobsLocked(minAge)
}

// DeleteUnreferencedBlobs removes what UnreferencedBlobs reports, recomputed
// under the store lock so a reference written in between is honored.
func (s *Store) DeleteUnreferencedBlobs(minAge time.Duration) ([]BlobInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	garbage, err := s.unreferencedBlobsLocked(minAge)
	if err != nil {
		return nil, err
	}
	for _, blob := range garbage {
		if err := os.Remove(s.blobPath(blob.Ref)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("delete blob %s: %w", blob.Ref, err)
		}
	}
	return garbage, nil
}

func (s *Store) unreferencedBlobsLocked(minAge time.Duration) ([]BlobInfo, error) {
	referenced, err := s.referencedBlobRefsLocked()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(s.dir, blobDirName))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read blob directory: %w", err)
	}
	cutoff := time.Now().Add(-minAge)
	var garbage []BlobInfo
	for _, entry := range entries {
		if !isBlobRef(entry.Name()) || referenced[entry.Name()] {
			continue
		}
		info, err := entry.Info()
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("stat blob %s: %w", entry.Name(), err)
		}
		if info.ModTime().After(cutoff) {
			continue
		}
		garbage = append(garbage, BlobInfo{Ref: entry.Name(), Size: info.Size(), ModTime: info.ModTime()})
	}
	slices.SortFunc(garbage, func(a, b BlobInfo) int { return strings.Compare(a.Ref, b.Ref) })
	return garbage, nil
}

// referencedBlobRefsLocked walks every stored task tree, including soft
// deleted nodes. A tree that fails to load aborts the sweep: references that
// cannot be read must never make their blobs look deletable.
func (s *Store) referencedBlobRefsLocked() (map[string]bool, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, fmt.Errorf("read data directory: %w", err)
	}
	referenced := map[string]bool{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		root, err := s.loadUnlocked(strings.TrimSuffix(name, ".json"))
		if err != nil {
			return nil, fmt.Errorf("task file %s: %w", name, err)
		}
		collectBlobRefs(root, referenced)
	}
	return referenced, nil
}

func isBlobRef(ref string) bool {
	return isHexToken(ref, sha256.Size*2)
}

// isHexToken reports whether value is exactly length lower-case hex digits.
func isHexToken(value string, length int) bool {
	if len(value) != length {
		return false
	}
	for _, r := range value {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'f':
		default:
			return false
		}
	}
	return true
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
