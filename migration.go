package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type legacyMigrationFile struct {
	sourceName string
	targetName string
	data       []byte
}

func migrateLegacyData(sourceDir, destinationDir string) (int, error) {
	sourceDir = strings.TrimSpace(sourceDir)
	destinationDir = strings.TrimSpace(destinationDir)
	if sourceDir == "" {
		return 0, errors.New("legacy data directory is required")
	}
	if destinationDir == "" {
		return 0, errors.New("migration destination data directory is required")
	}
	insideSource, err := migrationDestinationInsideSource(sourceDir, destinationDir)
	if err != nil {
		return 0, err
	}
	if insideSource {
		return 0, fmt.Errorf("migration destination %q must not be inside legacy data directory %q", destinationDir, sourceDir)
	}

	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return 0, fmt.Errorf("read legacy data directory %q: %w", sourceDir, err)
	}
	files, err := readLegacyMigrationFiles(sourceDir, entries)
	if err != nil {
		return 0, err
	}
	if len(files) == 0 {
		return 0, fmt.Errorf("legacy data directory %q contains no .json task files", sourceDir)
	}

	if _, err := os.Lstat(destinationDir); err == nil {
		return 0, fmt.Errorf("migration destination %q already exists; choose an unused -data-dir", destinationDir)
	} else if !errors.Is(err, os.ErrNotExist) {
		return 0, fmt.Errorf("inspect migration destination %q: %w", destinationDir, err)
	}
	if err := os.MkdirAll(filepath.Dir(destinationDir), 0o700); err != nil {
		return 0, fmt.Errorf("create migration destination parent for %q: %w", destinationDir, err)
	}
	if err := os.Mkdir(destinationDir, 0o700); err != nil {
		return 0, fmt.Errorf("create migration destination %q: %w", destinationDir, err)
	}

	written := make([]string, 0, len(files))
	for _, file := range files {
		targetPath := filepath.Join(destinationDir, file.targetName)
		written = append(written, targetPath)
		if err := writeMigrationFile(targetPath, file.data); err != nil {
			cleanupErr := cleanupMigrationDestination(destinationDir, written)
			return 0, errors.Join(fmt.Errorf("migrate legacy task file %q to %q: %w", file.sourceName, targetPath, err), cleanupErr)
		}
	}
	return len(files), nil
}

func migrationDestinationInsideSource(sourceDir, destinationDir string) (bool, error) {
	sourcePath, err := filepath.Abs(sourceDir)
	if err != nil {
		return false, fmt.Errorf("resolve legacy data directory %q: %w", sourceDir, err)
	}
	if resolved, err := filepath.EvalSymlinks(sourcePath); err == nil {
		sourcePath = resolved
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("resolve legacy data directory %q: %w", sourceDir, err)
	}
	destinationPath, err := filepath.Abs(destinationDir)
	if err != nil {
		return false, fmt.Errorf("resolve migration destination %q: %w", destinationDir, err)
	}
	if resolvedParent, err := filepath.EvalSymlinks(filepath.Dir(destinationPath)); err == nil {
		destinationPath = filepath.Join(resolvedParent, filepath.Base(destinationPath))
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("resolve migration destination parent %q: %w", filepath.Dir(destinationDir), err)
	}
	relative, err := filepath.Rel(sourcePath, destinationPath)
	if err != nil {
		return false, fmt.Errorf("compare legacy data directory %q with migration destination %q: %w", sourceDir, destinationDir, err)
	}
	return relative == "." || relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator)), nil
}

func readLegacyMigrationFiles(sourceDir string, entries []os.DirEntry) ([]legacyMigrationFile, error) {
	files := make([]legacyMigrationFile, 0, len(entries))
	targetSources := map[string]string{}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, fmt.Errorf("inspect legacy task file %q: %w", filepath.Join(sourceDir, entry.Name()), err)
		}
		if entry.Type()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("legacy task path %q is not a regular file", filepath.Join(sourceDir, entry.Name()))
		}
		userID := strings.TrimSuffix(entry.Name(), ".json")
		if userID == "" {
			return nil, fmt.Errorf("legacy task file %q has an empty user identifier", filepath.Join(sourceDir, entry.Name()))
		}
		targetName := safeUserID(userID) + ".json"
		if previous, exists := targetSources[targetName]; exists {
			return nil, fmt.Errorf("legacy task files %q and %q both map to destination file %q", previous, entry.Name(), targetName)
		}
		data, err := os.ReadFile(filepath.Join(sourceDir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read legacy task file %q: %w", filepath.Join(sourceDir, entry.Name()), err)
		}
		normalized, err := normalizeLegacyData(data, entry.Name())
		if err != nil {
			return nil, err
		}
		targetSources[targetName] = entry.Name()
		files = append(files, legacyMigrationFile{sourceName: entry.Name(), targetName: targetName, data: normalized})
	}
	return files, nil
}

func normalizeLegacyData(data []byte, sourceName string) ([]byte, error) {
	var root *Task
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("parse legacy task file %q: %w", sourceName, err)
	}
	if root == nil {
		return nil, fmt.Errorf("parse legacy task file %q: expected a task object, received null", sourceName)
	}
	root = normalizeTree(root)
	if err := validateTaskTree(root); err != nil {
		return nil, fmt.Errorf("validate legacy task file %q: %w", sourceName, err)
	}
	normalized, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode migrated task file %q: %w", sourceName, err)
	}
	return append(normalized, '\n'), nil
}

func writeMigrationFile(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create destination file: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return fmt.Errorf("write destination file: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return fmt.Errorf("sync destination file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close destination file: %w", err)
	}
	return nil
}

func cleanupMigrationDestination(destinationDir string, written []string) error {
	var cleanupErr error
	for _, path := range written {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("remove incomplete migration file %q: %w", path, err))
		}
	}
	if err := os.Remove(destinationDir); err != nil && !errors.Is(err, os.ErrNotExist) {
		cleanupErr = errors.Join(cleanupErr, fmt.Errorf("remove incomplete migration destination %q: %w", destinationDir, err))
	}
	return cleanupErr
}
