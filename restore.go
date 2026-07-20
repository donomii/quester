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
)

const (
	maxRestoreBytes          int64 = 10 << 20
	maxRestoreArchiveBytes   int64 = 1 << 30
	maxRestoreEntries              = 10_001
	maxRestoreBlobBytes      int64 = maxAttachmentBytes
	maxRestoreTotalBlobBytes int64 = 4 << 30
	maxRestoreBodyBytes      int64 = maxRestoreArchiveBytes + (1 << 20)
	restoreStagePrefix             = ".quester-restore-"
)

var errRestoreTooLarge = errors.New("backup exceeds restore limit")

type restoreLimits struct {
	archiveBytes   int64
	entries        int
	taskBytes      int64
	blobBytes      int64
	totalBlobBytes int64
}

var defaultRestoreLimits = restoreLimits{
	archiveBytes:   maxRestoreArchiveBytes,
	entries:        maxRestoreEntries,
	taskBytes:      maxRestoreBytes,
	blobBytes:      maxRestoreBlobBytes,
	totalBlobBytes: maxRestoreTotalBlobBytes,
}

type restorePlan struct {
	stageDir string
	fileID   string
	taskHash string
	blobRefs []string
	blobSize map[string]int64
	newBlobs []string
}

type restoreJournal struct {
	FileID   string   `json:"file_id"`
	TaskHash string   `json:"task_hash"`
	NewBlobs []string `json:"new_blobs"`
}

func (s *Store) Restore(userID string, data []byte) error {
	if int64(len(data)) > maxRestoreBytes {
		return fmt.Errorf("%w: tasks.json is larger than %d bytes", errRestoreTooLarge, maxRestoreBytes)
	}
	root, err := decodeRestoredTask(data)
	if err != nil {
		return err
	}
	if err := s.lock(); err != nil {
		return err
	}
	defer s.unlock()
	if err := s.recoverRestoreStagesLocked(); err != nil {
		return err
	}
	return s.saveUnlocked(safeUserID(userID), root)
}

func (s *Store) RestoreArchive(userID string, archive io.ReaderAt, size int64) error {
	if size < 0 {
		return fmt.Errorf("backup archive has invalid byte size %d", size)
	}
	if size > maxRestoreArchiveBytes {
		return fmt.Errorf("%w: archive is %d bytes; the maximum is %d", errRestoreTooLarge, size, maxRestoreArchiveBytes)
	}
	if err := s.lock(); err != nil {
		return err
	}
	defer s.unlock()
	if err := s.recoverRestoreStagesLocked(); err != nil {
		return err
	}
	cleanupStage := true
	plan, err := s.stageRestoreArchiveLocked(userID, archive, size, defaultRestoreLimits)
	if plan != nil {
		defer func() {
			if cleanupStage {
				if cleanupErr := s.cleanupRestoreStage(plan.stageDir); cleanupErr != nil {
					logRestoreCleanupFailed(plan.stageDir, cleanupErr)
				}
			}
		}()
	}
	if err != nil {
		return err
	}
	if err := s.prepareRestoreCommitLocked(plan); err != nil {
		return err
	}
	for _, ref := range plan.blobRefs {
		if err := s.installRestoreBlobLocked(plan, ref); err != nil {
			return s.rollbackRestoreErrorLocked(plan, err)
		}
	}
	if err := os.Rename(filepath.Join(plan.stageDir, "tasks.json"), s.path(plan.fileID)); err != nil {
		return s.rollbackRestoreErrorLocked(plan, fmt.Errorf("replace task tree: %w", err))
	}
	if err := syncDirectory(s.dir); err != nil {
		cleanupStage = false
		logRestoreSyncFailed(s.dir, err)
	}
	return nil
}

func (s *Store) stageRestoreArchiveLocked(userID string, archive io.ReaderAt, size int64, limits restoreLimits) (*restorePlan, error) {
	if err := validateRestoreLimits(size, limits); err != nil {
		return nil, err
	}
	reader, err := zip.NewReader(archive, size)
	if err != nil {
		return nil, fmt.Errorf("backup is not a readable zip archive: %w", err)
	}
	if len(reader.File) > limits.entries {
		return nil, fmt.Errorf("%w: archive has %d entries; the maximum is %d", errRestoreTooLarge, len(reader.File), limits.entries)
	}
	stageDir, err := os.MkdirTemp(s.dir, restoreStagePrefix)
	if err != nil {
		return nil, fmt.Errorf("create restore staging directory: %w", err)
	}
	plan := &restorePlan{stageDir: stageDir, fileID: safeUserID(userID), blobSize: map[string]int64{}}
	if err := os.Mkdir(filepath.Join(stageDir, blobDirName), 0o700); err != nil {
		return plan, fmt.Errorf("create staged blob directory: %w", err)
	}
	seen := map[string]bool{}
	var tasksJSON []byte
	var totalBlobBytes int64
	for _, entry := range reader.File {
		if err := validateRestoreEntryName(entry.Name); err != nil {
			return plan, err
		}
		if seen[entry.Name] {
			return plan, fmt.Errorf("backup archive contains duplicate entry %q", entry.Name)
		}
		seen[entry.Name] = true
		if !entry.FileInfo().Mode().IsRegular() {
			return plan, fmt.Errorf("backup entry %q is not a regular file", entry.Name)
		}
		if entry.Name == "tasks.json" {
			tasksJSON, err = readRestoreEntry(entry, limits.taskBytes)
		} else {
			ref := strings.TrimPrefix(entry.Name, blobDirName+"/")
			var blobBytes int64
			blobBytes, err = stageRestoreBlob(entry, filepath.Join(stageDir, blobDirName, ref), ref, limits.blobBytes)
			if err == nil && blobBytes > limits.totalBlobBytes-totalBlobBytes {
				err = fmt.Errorf("%w: expanded blob content exceeds %d bytes", errRestoreTooLarge, limits.totalBlobBytes)
			}
			if err == nil {
				totalBlobBytes += blobBytes
				plan.blobSize[ref] = blobBytes
			}
		}
		if err != nil {
			return plan, err
		}
	}
	if tasksJSON == nil {
		return plan, errors.New("backup archive does not contain tasks.json")
	}
	root, err := decodeRestoredTask(tasksJSON)
	if err != nil {
		return plan, err
	}
	refs := map[string]bool{}
	collectBlobRefs(root, refs)
	if err := validateRestoredAttachments(root, plan.blobSize); err != nil {
		return plan, err
	}
	for ref := range plan.blobSize {
		if !refs[ref] {
			return plan, fmt.Errorf("backup archive contains unreferenced blob %s", ref)
		}
	}
	for ref := range refs {
		if _, exists := plan.blobSize[ref]; !exists {
			return plan, fmt.Errorf("backup archive is missing referenced blob %s", ref)
		}
		plan.blobRefs = append(plan.blobRefs, ref)
	}
	slices.Sort(plan.blobRefs)
	taskHash, err := writeStagedTask(filepath.Join(stageDir, "tasks.json"), root)
	if err != nil {
		return plan, err
	}
	plan.taskHash = taskHash
	return plan, nil
}

func validateRestoreLimits(size int64, limits restoreLimits) error {
	if limits.archiveBytes <= 0 || limits.entries <= 0 || limits.taskBytes <= 0 || limits.blobBytes <= 0 || limits.totalBlobBytes <= 0 {
		return errors.New("restore limits must all be positive")
	}
	if size < 0 {
		return fmt.Errorf("backup archive has invalid byte size %d", size)
	}
	if size > limits.archiveBytes {
		return fmt.Errorf("%w: archive is %d bytes; the maximum is %d", errRestoreTooLarge, size, limits.archiveBytes)
	}
	return nil
}

func validateRestoreEntryName(name string) error {
	if name == "" || path.IsAbs(name) || path.Clean(name) != name || strings.Contains(name, "\\") {
		return fmt.Errorf("backup archive contains unsafe entry name %q", name)
	}
	if name == "tasks.json" {
		return nil
	}
	if strings.HasPrefix(name, blobDirName+"/") && isBlobRef(strings.TrimPrefix(name, blobDirName+"/")) {
		return nil
	}
	return fmt.Errorf("backup archive contains unexpected entry %q", name)
}

func readRestoreEntry(entry *zip.File, limit int64) ([]byte, error) {
	if entry.UncompressedSize64 > uint64(limit) {
		return nil, fmt.Errorf("%w: entry %s is larger than %d bytes", errRestoreTooLarge, entry.Name, limit)
	}
	file, err := entry.Open()
	if err != nil {
		return nil, fmt.Errorf("open backup entry %s: %w", entry.Name, err)
	}
	data, readErr := io.ReadAll(io.LimitReader(file, limit+1))
	closeErr := file.Close()
	if readErr != nil {
		return nil, fmt.Errorf("read backup entry %s: %w", entry.Name, readErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close backup entry %s: %w", entry.Name, closeErr)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%w: entry %s is larger than %d bytes", errRestoreTooLarge, entry.Name, limit)
	}
	return data, nil
}

func stageRestoreBlob(entry *zip.File, destination, expectedRef string, limit int64) (int64, error) {
	if entry.UncompressedSize64 > uint64(limit) {
		return 0, fmt.Errorf("%w: blob %s is larger than %d bytes", errRestoreTooLarge, expectedRef, limit)
	}
	source, err := entry.Open()
	if err != nil {
		return 0, fmt.Errorf("open backup blob %s: %w", expectedRef, err)
	}
	destinationFile, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		source.Close()
		return 0, fmt.Errorf("create staged blob %s: %w", expectedRef, err)
	}
	hash := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(destinationFile, hash), io.LimitReader(source, limit+1))
	syncErr := destinationFile.Sync()
	closeDestinationErr := destinationFile.Close()
	closeSourceErr := source.Close()
	if copyErr != nil {
		return 0, fmt.Errorf("read backup blob %s: %w", expectedRef, copyErr)
	}
	if size > limit {
		return 0, fmt.Errorf("%w: blob %s is larger than %d bytes", errRestoreTooLarge, expectedRef, limit)
	}
	if syncErr != nil {
		return 0, fmt.Errorf("sync staged blob %s: %w", expectedRef, syncErr)
	}
	if closeDestinationErr != nil {
		return 0, fmt.Errorf("close staged blob %s: %w", expectedRef, closeDestinationErr)
	}
	if closeSourceErr != nil {
		return 0, fmt.Errorf("close backup blob %s: %w", expectedRef, closeSourceErr)
	}
	actualRef := hex.EncodeToString(hash.Sum(nil))
	if actualRef != expectedRef {
		return 0, fmt.Errorf("backup blob %s content hashes to %s; the archive is corrupt", expectedRef, actualRef)
	}
	return size, nil
}

func decodeRestoredTask(data []byte) (*Task, error) {
	var root *Task
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("backup is not valid task JSON: %w", err)
	}
	if root == nil {
		return nil, errors.New("backup is not valid task JSON: expected a task object, received null")
	}
	normalized := normalizeTree(root)
	if err := validateTaskTree(normalized); err != nil {
		return nil, fmt.Errorf("backup contains invalid task data: %w", err)
	}
	return normalized, nil
}

func validateRestoredAttachments(task *Task, blobSizes map[string]int64) error {
	for _, attachment := range task.Attachments {
		actualSize, exists := blobSizes[attachment.Blob]
		if !exists {
			return fmt.Errorf("backup archive is missing referenced blob %s", attachment.Blob)
		}
		if attachment.Size != actualSize {
			return fmt.Errorf("backup attachment %q records %d bytes, but blob %s contains %d bytes", attachment.Name, attachment.Size, attachment.Blob, actualSize)
		}
	}
	for _, child := range task.SubTasks {
		if err := validateRestoredAttachments(child, blobSizes); err != nil {
			return err
		}
	}
	return nil
}

func writeStagedTask(destination string, root *Task) (string, error) {
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", fmt.Errorf("create staged task tree: %w", err)
	}
	hash := sha256.New()
	encoder := json.NewEncoder(io.MultiWriter(file, hash))
	encoder.SetIndent("", "  ")
	encodeErr := encoder.Encode(root)
	syncErr := file.Sync()
	closeErr := file.Close()
	if encodeErr != nil {
		return "", fmt.Errorf("encode staged task tree: %w", encodeErr)
	}
	if syncErr != nil {
		return "", fmt.Errorf("sync staged task tree: %w", syncErr)
	}
	if closeErr != nil {
		return "", fmt.Errorf("close staged task tree: %w", closeErr)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func (s *Store) prepareRestoreCommitLocked(plan *restorePlan) error {
	if err := os.MkdirAll(filepath.Join(s.dir, blobDirName), 0o700); err != nil {
		return fmt.Errorf("create blob directory: %w", err)
	}
	for _, ref := range plan.blobRefs {
		final := s.blobPath(ref)
		if _, err := os.Stat(final); err == nil {
			if err := verifyBlobFile(final, ref, plan.blobSize[ref]); err != nil {
				return fmt.Errorf("existing blob %s is invalid: %w", ref, err)
			}
		} else if errors.Is(err, os.ErrNotExist) {
			plan.newBlobs = append(plan.newBlobs, ref)
		} else {
			return fmt.Errorf("stat destination blob %s: %w", ref, err)
		}
	}
	journal := restoreJournal{FileID: plan.fileID, TaskHash: plan.taskHash, NewBlobs: plan.newBlobs}
	pendingJournal := filepath.Join(plan.stageDir, "journal.pending")
	file, err := os.OpenFile(pendingJournal, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create restore journal: %w", err)
	}
	encodeErr := json.NewEncoder(file).Encode(journal)
	syncErr := file.Sync()
	closeErr := file.Close()
	if encodeErr != nil {
		return fmt.Errorf("encode restore journal: %w", encodeErr)
	}
	if syncErr != nil {
		return fmt.Errorf("sync restore journal: %w", syncErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close restore journal: %w", closeErr)
	}
	if err := os.Rename(pendingJournal, filepath.Join(plan.stageDir, "journal.json")); err != nil {
		return fmt.Errorf("install restore journal: %w", err)
	}
	return syncDirectory(plan.stageDir)
}

func (s *Store) installRestoreBlobLocked(plan *restorePlan, ref string) error {
	staged := filepath.Join(plan.stageDir, blobDirName, ref)
	if err := verifyBlobFile(staged, ref, plan.blobSize[ref]); err != nil {
		return fmt.Errorf("staged blob %s is invalid: %w", ref, err)
	}
	final := s.blobPath(ref)
	if _, err := os.Stat(final); err == nil {
		return verifyBlobFile(final, ref, plan.blobSize[ref])
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat destination blob %s: %w", ref, err)
	}
	if err := os.Link(staged, final); err != nil {
		if _, statErr := os.Stat(final); statErr == nil {
			return verifyBlobFile(final, ref, plan.blobSize[ref])
		}
		return fmt.Errorf("install blob %s: %w", ref, err)
	}
	return syncDirectory(filepath.Join(s.dir, blobDirName))
}

func verifyBlobFile(filename, expectedRef string, expectedSize int64) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	hash := sha256.New()
	size, readErr := io.Copy(hash, io.LimitReader(file, expectedSize+1))
	closeErr := file.Close()
	if readErr != nil {
		return fmt.Errorf("read content: %w", readErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close content: %w", closeErr)
	}
	if size != expectedSize {
		return fmt.Errorf("contains %d bytes; expected %d", size, expectedSize)
	}
	actualRef := hex.EncodeToString(hash.Sum(nil))
	if actualRef != expectedRef {
		return fmt.Errorf("content hashes to %s; expected %s", actualRef, expectedRef)
	}
	return nil
}

func (s *Store) rollbackRestoreErrorLocked(plan *restorePlan, restoreErr error) error {
	if rollbackErr := s.recoverRestoreStageLocked(plan.stageDir); rollbackErr != nil {
		return fmt.Errorf("restore failed: %v; recovery also failed: %w", restoreErr, rollbackErr)
	}
	return restoreErr
}

func (s *Store) recoverRestoreStages() error {
	if err := s.lock(); err != nil {
		return err
	}
	defer s.unlock()
	return s.recoverRestoreStagesLocked()
}

func (s *Store) recoverRestoreStagesLocked() error {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return fmt.Errorf("read data directory while recovering restores: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), restoreStagePrefix) {
			continue
		}
		if err := s.recoverRestoreStageLocked(filepath.Join(s.dir, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) recoverRestoreStageLocked(stageDir string) error {
	journalData, err := os.ReadFile(filepath.Join(stageDir, "journal.json"))
	if errors.Is(err, os.ErrNotExist) {
		return s.cleanupRestoreStage(stageDir)
	}
	if err != nil {
		return fmt.Errorf("read restore journal in %q: %w", stageDir, err)
	}
	var journal restoreJournal
	if err := json.Unmarshal(journalData, &journal); err != nil {
		return fmt.Errorf("parse restore journal in %q: %w", stageDir, err)
	}
	if !isSafeFileID(journal.FileID) || !isBlobRef(journal.TaskHash) {
		return fmt.Errorf("restore journal in %q has invalid task destination or hash", stageDir)
	}
	currentHash, hashErr := hashFile(s.path(journal.FileID))
	if hashErr == nil && currentHash == journal.TaskHash {
		return s.cleanupRestoreStage(stageDir)
	}
	if hashErr != nil && !errors.Is(hashErr, os.ErrNotExist) {
		return fmt.Errorf("read current task tree while recovering %q: %w", stageDir, hashErr)
	}
	referenced, err := s.referencedBlobRefsLocked()
	if err != nil {
		return fmt.Errorf("find referenced blobs while recovering %q: %w", stageDir, err)
	}
	seen := map[string]bool{}
	for _, ref := range journal.NewBlobs {
		if !isBlobRef(ref) || seen[ref] {
			return fmt.Errorf("restore journal in %q has invalid or duplicate blob %q", stageDir, ref)
		}
		seen[ref] = true
		if referenced[ref] {
			continue
		}
		stagedInfo, stagedErr := os.Stat(filepath.Join(stageDir, blobDirName, ref))
		if errors.Is(stagedErr, os.ErrNotExist) {
			continue
		}
		if stagedErr != nil {
			return fmt.Errorf("stat staged blob %s while recovering %q: %w", ref, stageDir, stagedErr)
		}
		finalInfo, finalErr := os.Stat(s.blobPath(ref))
		if errors.Is(finalErr, os.ErrNotExist) {
			continue
		}
		if finalErr != nil {
			return fmt.Errorf("stat installed blob %s while recovering %q: %w", ref, stageDir, finalErr)
		}
		if !os.SameFile(stagedInfo, finalInfo) {
			continue
		}
		if err := os.Remove(s.blobPath(ref)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove uncommitted blob %s while recovering %q: %w", ref, stageDir, err)
		}
	}
	return s.cleanupRestoreStage(stageDir)
}

func hashFile(filename string) (string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	_, readErr := io.Copy(hash, file)
	closeErr := file.Close()
	if readErr != nil {
		return "", readErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func (s *Store) cleanupRestoreStage(stageDir string) error {
	cleanDir := filepath.Clean(stageDir)
	if filepath.Dir(cleanDir) != filepath.Clean(s.dir) || !strings.HasPrefix(filepath.Base(cleanDir), restoreStagePrefix) {
		return fmt.Errorf("refuse to clean invalid restore staging path %q", stageDir)
	}
	entries, err := os.ReadDir(cleanDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read restore staging directory %q: %w", cleanDir, err)
	}
	for _, entry := range entries {
		entryPath := filepath.Join(cleanDir, entry.Name())
		if entry.Name() == blobDirName && entry.IsDir() {
			blobs, err := os.ReadDir(entryPath)
			if err != nil {
				return fmt.Errorf("read staged blobs in %q: %w", cleanDir, err)
			}
			for _, blob := range blobs {
				if blob.IsDir() {
					return fmt.Errorf("restore staging directory %q contains unexpected nested directory %q", cleanDir, blob.Name())
				}
				if err := os.Remove(filepath.Join(entryPath, blob.Name())); err != nil {
					return fmt.Errorf("remove staged blob %q: %w", blob.Name(), err)
				}
			}
			if err := os.Remove(entryPath); err != nil {
				return fmt.Errorf("remove staged blob directory %q: %w", entryPath, err)
			}
		} else if (entry.Name() == "tasks.json" || entry.Name() == "journal.json" || entry.Name() == "journal.pending") && !entry.IsDir() {
			if err := os.Remove(entryPath); err != nil {
				return fmt.Errorf("remove staged file %q: %w", entryPath, err)
			}
		} else {
			return fmt.Errorf("restore staging directory %q contains unexpected entry %q", cleanDir, entry.Name())
		}
	}
	if err := os.Remove(cleanDir); err != nil {
		return fmt.Errorf("remove restore staging directory %q: %w", cleanDir, err)
	}
	return nil
}

func syncDirectory(dir string) error {
	file, err := os.Open(dir)
	if err != nil {
		return err
	}
	syncErr := file.Sync()
	closeErr := file.Close()
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}
