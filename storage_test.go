package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreLoadMissingReturnsDefaultRoot(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	root, err := store.Load("")
	if err != nil {
		t.Fatal(err)
	}
	if root.Id != rootPath {
		t.Fatalf("root id = %q, want %q", root.Id, rootPath)
	}
	if root.Name != "Quester" {
		t.Fatalf("root name = %q, want Quester", root.Name)
	}
}

func TestStoreUpdatePersistsAtomicallyNamedFile(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	const unsafeUser = "../someone"
	if err := store.Update(unsafeUser, func(root *Task) error {
		root.SubTasks = append(root.SubTasks, &Task{Id: "abc", Name: "A"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("stored files = %d, want 1", len(files))
	}
	if strings.Contains(files[0].Name(), "..") || strings.Contains(files[0].Name(), "/") {
		t.Fatalf("unsafe file name was used: %q", files[0].Name())
	}

	root, err := store.Load(unsafeUser)
	if err != nil {
		t.Fatal(err)
	}
	if got := FindTask(rootPath+"/abc", root); got == nil || got.Name != "A" {
		t.Fatalf("stored task = %#v, want A", got)
	}
}

func TestStoreRestoreRejectsInvalidJSON(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	if err := store.Restore(defaultUserID, []byte("{not-json")); err == nil {
		t.Fatal("Restore accepted invalid JSON")
	}
}

func TestStoreLoadReportsInvalidExistingJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, defaultUserID+".json"), []byte("{not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.Load(defaultUserID); err == nil {
		t.Fatal("Load accepted invalid JSON")
	}
}
