package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var files []os.DirEntry
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".json") {
			files = append(files, entry)
		}
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

func TestStoresSharingDirectorySerializeUpdates(t *testing.T) {
	dir := t.TempDir()
	first, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	stores := []*Store{first, second}
	errors := make(chan error, 20)
	var wait sync.WaitGroup
	for index := 0; index < 20; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			errors <- stores[index%len(stores)].Update(defaultUserID, func(root *Task) error {
				root.SubTasks = append(root.SubTasks, newTask(fmt.Sprintf("Task %d", index), "", defaultForumID, defaultUserID, true))
				return nil
			})
		}(index)
	}
	wait.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	root, err := first.Load(defaultUserID)
	if err != nil {
		t.Fatal(err)
	}
	if len(root.SubTasks) != 20 {
		t.Fatalf("stored tasks = %d, want 20", len(root.SubTasks))
	}
}

func TestSaveBlobIsContentAddressedAndDeduped(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	const helloSHA256 = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	ref, size, err := store.SaveBlob(strings.NewReader("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if ref != helloSHA256 || size != 5 {
		t.Fatalf("SaveBlob = %q size %d, want %q size 5", ref, size, helloSHA256)
	}

	again, _, err := store.SaveBlob(strings.NewReader("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if again != ref {
		t.Fatalf("second SaveBlob = %q, want %q", again, ref)
	}
	files, err := os.ReadDir(filepath.Join(dir, blobDirName))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("blob files = %d, want 1", len(files))
	}

	file, info, err := store.OpenBlob(ref)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	content, err := io.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "hello" || info.Size() != 5 {
		t.Fatalf("blob content = %q size %d, want hello size 5", content, info.Size())
	}
}

func TestOpenBlobRejectsUnsafeRefs(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range []string{
		"",
		"..",
		"../../etc/passwd",
		strings.Repeat("A", 64),
		strings.Repeat("a", 63),
		strings.Repeat("g", 64),
	} {
		if _, _, err := store.OpenBlob(ref); err == nil {
			t.Fatalf("OpenBlob(%q) succeeded, want error", ref)
		}
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
	if err := store.Restore(defaultUserID, []byte("null")); err == nil {
		t.Fatal("Restore accepted a null task tree")
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
