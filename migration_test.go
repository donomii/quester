package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrateLegacyDataNormalizesEveryJSONFile(t *testing.T) {
	parent := t.TempDir()
	sourceDir := filepath.Join(parent, "quester")
	destinationDir := filepath.Join(parent, ".quester-data")
	if err := os.Mkdir(sourceDir, 0o700); err != nil {
		t.Fatal(err)
	}
	personalData := []byte(`{"Name":"Old Quester","SubTasks":[{"Id":"old-task","Name":"Old task"}]}`)
	unsafeData := []byte(`{"Name":"Second user"}`)
	if err := os.WriteFile(filepath.Join(sourceDir, defaultUserID+".json"), personalData, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "person@example.com.json"), unsafeData, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "notes.txt"), []byte("not task data"), 0o600); err != nil {
		t.Fatal(err)
	}

	count, err := migrateLegacyData(sourceDir, destinationDir)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("migrated file count = %d, want 2", count)
	}
	unchanged, err := os.ReadFile(filepath.Join(sourceDir, defaultUserID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(unchanged) != string(personalData) {
		t.Fatalf("legacy source changed to %q, want %q", unchanged, personalData)
	}

	store, err := NewStore(destinationDir)
	if err != nil {
		t.Fatal(err)
	}
	root, err := store.Load(defaultUserID)
	if err != nil {
		t.Fatal(err)
	}
	if root.Schema != currentSchema || len(root.Forums) != 1 || len(root.Users) != 1 {
		t.Fatalf("migrated metadata = schema %d forums %d users %d", root.Schema, len(root.Forums), len(root.Users))
	}
	if len(root.SubTasks) != 1 || !root.SubTasks[0].Track {
		t.Fatalf("migrated legacy tasks = %#v, want one tracked task", root.SubTasks)
	}
	unsafeRoot, err := store.Load("person@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if unsafeRoot.Name != "Second user" {
		t.Fatalf("migrated unsafe user root name = %q, want Second user", unsafeRoot.Name)
	}
}

func TestMigrateLegacyDataValidatesAllFilesBeforeCreatingDestination(t *testing.T) {
	parent := t.TempDir()
	sourceDir := filepath.Join(parent, "quester")
	destinationDir := filepath.Join(parent, ".quester-data")
	if err := os.Mkdir(sourceDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "a.json"), []byte(`{"Name":"valid"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "b.json"), []byte(`{"Name":`), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := migrateLegacyData(sourceDir, destinationDir); err == nil || !strings.Contains(err.Error(), "b.json") {
		t.Fatalf("migration error = %v, want invalid b.json error", err)
	}
	if _, err := os.Stat(destinationDir); !os.IsNotExist(err) {
		t.Fatalf("destination exists after validation failure: %v", err)
	}
}

func TestMigrateLegacyDataRefusesExistingDestination(t *testing.T) {
	parent := t.TempDir()
	sourceDir := filepath.Join(parent, "quester")
	destinationDir := filepath.Join(parent, ".quester-data")
	if err := os.Mkdir(sourceDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "a.json"), []byte(`{"Name":"valid"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(destinationDir, 0o700); err != nil {
		t.Fatal(err)
	}
	markerPath := filepath.Join(destinationDir, "keep")
	if err := os.WriteFile(markerPath, []byte("existing data"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := migrateLegacyData(sourceDir, destinationDir); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("migration error = %v, want existing destination error", err)
	}
	marker, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(marker) != "existing data" {
		t.Fatalf("existing destination marker = %q, want existing data", marker)
	}
}

func TestMigrateLegacyDataRefusesDestinationInsideSource(t *testing.T) {
	sourceDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(sourceDir, "a.json"), []byte(`{"Name":"valid"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	destinationDir := filepath.Join(sourceDir, "new-data")

	if _, err := migrateLegacyData(sourceDir, destinationDir); err == nil || !strings.Contains(err.Error(), "must not be inside") {
		t.Fatalf("migration error = %v, want nested destination error", err)
	}
	if _, err := os.Stat(destinationDir); !os.IsNotExist(err) {
		t.Fatalf("nested destination exists after rejection: %v", err)
	}
}

func TestMigrateLegacyDataRejectsNullAndEmptySources(t *testing.T) {
	for _, content := range []string{"null", ""} {
		parent := t.TempDir()
		sourceDir := filepath.Join(parent, "quester")
		if err := os.Mkdir(sourceDir, 0o700); err != nil {
			t.Fatal(err)
		}
		if content != "" {
			if err := os.WriteFile(filepath.Join(sourceDir, "user.json"), []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := migrateLegacyData(sourceDir, filepath.Join(parent, ".quester-data")); err == nil {
			t.Fatalf("migration accepted source content %q", content)
		}
	}
}
