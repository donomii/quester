package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type restoreArchiveEntry struct {
	name    string
	content []byte
}

func TestRestoreArchiveCommitsValidatedTaskTreeAndBlobs(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	storeTaskName(t, store, "Previous")
	root, ref, content := restoreTreeWithBlob("Restored", []byte("document content"))
	archive := makeRestoreArchive(t, restoreEntries(t, root, ref, content))

	if err := store.RestoreArchive(defaultUserID, bytes.NewReader(archive), int64(len(archive))); err != nil {
		t.Fatal(err)
	}
	restored, err := store.Load(defaultUserID)
	if err != nil {
		t.Fatal(err)
	}
	if len(restored.SubTasks) != 1 || restored.SubTasks[0].Name != "Restored" {
		t.Fatalf("restored task tree = %#v, want one task named Restored", restored.SubTasks)
	}
	file, info, err := store.OpenBlob(ref)
	if err != nil {
		t.Fatal(err)
	}
	got, readErr := io.ReadAll(file)
	closeErr := file.Close()
	if readErr != nil || closeErr != nil {
		t.Fatalf("read restored blob: read error %v, close error %v", readErr, closeErr)
	}
	if !bytes.Equal(got, content) || info.Size() != int64(len(content)) {
		t.Fatalf("restored blob = %q size %d, want %q size %d", got, info.Size(), content, len(content))
	}
	assertNoRestoreStages(t, dir)
}

func TestRestoreArchiveRejectsInvalidEntriesWithoutChangingLiveData(t *testing.T) {
	validRoot, validRef, validContent := restoreTreeWithBlob("Replacement", []byte("valid content"))
	validTasks := encodeRestoreTree(t, validRoot)
	corruptRef := contentRef([]byte("expected content"))
	corruptRoot, _, _ := restoreTreeWithBlobRef("Corrupt", corruptRef, int64(len("expected content")))
	cases := []struct {
		name    string
		entries []restoreArchiveEntry
	}{
		{
			name: "corrupt blob",
			entries: []restoreArchiveEntry{
				{name: "tasks.json", content: encodeRestoreTree(t, corruptRoot)},
				{name: blobDirName + "/" + corruptRef, content: []byte("different content")},
			},
		},
		{
			name:    "path traversal",
			entries: append(restoreEntries(t, validRoot, validRef, validContent), restoreArchiveEntry{name: "../tasks.json", content: validTasks}),
		},
		{
			name: "duplicate path",
			entries: []restoreArchiveEntry{
				{name: "tasks.json", content: validTasks},
				{name: "tasks.json", content: validTasks},
				{name: blobDirName + "/" + validRef, content: validContent},
			},
		},
		{
			name: "missing referenced blob",
			entries: []restoreArchiveEntry{
				{name: "tasks.json", content: validTasks},
			},
		},
		{
			name: "unreferenced blob",
			entries: []restoreArchiveEntry{
				{name: "tasks.json", content: encodeRestoreTree(t, defaultRoot())},
				{name: blobDirName + "/" + validRef, content: validContent},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			store, err := NewStore(dir)
			if err != nil {
				t.Fatal(err)
			}
			storeTaskName(t, store, "Previous")
			archive := makeRestoreArchive(t, tc.entries)
			if err := store.RestoreArchive(defaultUserID, bytes.NewReader(archive), int64(len(archive))); err == nil {
				t.Fatal("RestoreArchive accepted invalid archive")
			}
			assertStoredTaskName(t, store, "Previous")
			assertNoRestoreStages(t, dir)
		})
	}
}

func TestRestoreArchiveEnforcesEveryResourceLimit(t *testing.T) {
	root, ref, content := restoreTreeWithBlob("Replacement", []byte("12345678"))
	entries := restoreEntries(t, root, ref, content)
	archive := makeRestoreArchive(t, entries)
	tasksSize := int64(len(entries[0].content))
	base := restoreLimits{
		archiveBytes:   int64(len(archive)) + 1,
		entries:        len(entries) + 1,
		taskBytes:      tasksSize + 1,
		blobBytes:      int64(len(content)) + 1,
		totalBlobBytes: int64(len(content)) + 1,
	}
	cases := []struct {
		name    string
		archive []byte
		limits  restoreLimits
	}{
		{name: "archive bytes", archive: archive, limits: withArchiveLimit(base, int64(len(archive))-1)},
		{name: "entry count", archive: archive, limits: withEntryLimit(base, len(entries)-1)},
		{name: "tasks.json bytes", archive: archive, limits: withTaskLimit(base, tasksSize-1)},
		{name: "single blob bytes", archive: archive, limits: withBlobLimit(base, int64(len(content))-1)},
		{name: "total blob bytes", archive: archive, limits: withTotalBlobLimit(base, int64(len(content))-1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store, err := NewStore(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			err = stageRestoreForTest(store, tc.archive, tc.limits)
			if !errors.Is(err, errRestoreTooLarge) {
				t.Fatalf("restore error = %v, want resource-limit error", err)
			}
		})
	}
}

func TestRestoreMultipartBodyLimitStopsHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := NewApp(store, "/quester/")
	router := gin.New()
	called := false
	router.POST("/limited", app.formMutation(func(c *gin.Context, userID string) { called = true }, 64))
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("content", "backup.zip")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(bytes.Repeat([]byte("x"), 128)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/limited", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("limited restore status = %d, want 413", recorder.Code)
	}
	if called {
		t.Fatal("limited restore invoked its handler")
	}
}

func TestRestoreRecoveryRollsBackInterruptedPreCommitInstall(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	storeTaskName(t, store, "Previous")
	root, ref, content := restoreTreeWithBlob("Replacement", []byte("interrupted content"))
	archive := makeRestoreArchive(t, restoreEntries(t, root, ref, content))
	plan := stagePreparedRestore(t, store, archive)
	if err := store.installRestoreBlobLocked(plan, ref); err != nil {
		store.unlock()
		t.Fatal(err)
	}
	store.unlock()
	if _, err := os.Stat(store.blobPath(ref)); err != nil {
		t.Fatalf("installed blob before recovery: %v", err)
	}

	reopened, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	assertStoredTaskName(t, reopened, "Previous")
	if _, err := os.Stat(reopened.blobPath(ref)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("uncommitted blob still exists after recovery: %v", err)
	}
	assertNoRestoreStages(t, dir)
}

func TestRestoreRecoveryFinishesInterruptedPostCommitCleanup(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	storeTaskName(t, store, "Previous")
	root, ref, content := restoreTreeWithBlob("Replacement", []byte("committed content"))
	archive := makeRestoreArchive(t, restoreEntries(t, root, ref, content))
	plan := stagePreparedRestore(t, store, archive)
	if err := store.installRestoreBlobLocked(plan, ref); err != nil {
		store.unlock()
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(plan.stageDir, "tasks.json"), store.path(plan.fileID)); err != nil {
		store.unlock()
		t.Fatal(err)
	}
	store.unlock()

	reopened, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	assertStoredTaskName(t, reopened, "Replacement")
	file, _, err := reopened.OpenBlob(ref)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	assertNoRestoreStages(t, dir)
}

func TestRestoreRecoveryDoesNotRemoveBlobInstalledByAnotherWriter(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	storeTaskName(t, store, "Previous")
	root, ref, content := restoreTreeWithBlob("Replacement", []byte("shared content"))
	archive := makeRestoreArchive(t, restoreEntries(t, root, ref, content))
	stagePreparedRestore(t, store, archive)
	if err := os.WriteFile(store.blobPath(ref), content, 0o600); err != nil {
		store.unlock()
		t.Fatal(err)
	}
	store.unlock()

	reopened, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	assertStoredTaskName(t, reopened, "Previous")
	file, _, err := reopened.OpenBlob(ref)
	if err != nil {
		t.Fatalf("recovery removed another writer's blob: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	assertNoRestoreStages(t, dir)
}

func stagePreparedRestore(t *testing.T, store *Store, archive []byte) *restorePlan {
	t.Helper()
	if err := store.lock(); err != nil {
		t.Fatal(err)
	}
	plan, err := store.stageRestoreArchiveLocked(defaultUserID, bytes.NewReader(archive), int64(len(archive)), defaultRestoreLimits)
	if err != nil {
		store.unlock()
		t.Fatal(err)
	}
	if err := store.prepareRestoreCommitLocked(plan); err != nil {
		store.unlock()
		t.Fatal(err)
	}
	return plan
}

func stageRestoreForTest(store *Store, archive []byte, limits restoreLimits) error {
	if err := store.lock(); err != nil {
		return err
	}
	defer store.unlock()
	plan, err := store.stageRestoreArchiveLocked(defaultUserID, bytes.NewReader(archive), int64(len(archive)), limits)
	if plan != nil {
		if cleanupErr := store.cleanupRestoreStage(plan.stageDir); err == nil {
			err = cleanupErr
		}
	}
	return err
}

func restoreTreeWithBlob(name string, content []byte) (*Task, string, []byte) {
	ref := contentRef(content)
	root, _, _ := restoreTreeWithBlobRef(name, ref, int64(len(content)))
	return root, ref, content
}

func restoreTreeWithBlobRef(name, ref string, size int64) (*Task, string, []byte) {
	root := defaultRoot()
	task := newTask(name, "", defaultForumID, defaultUserID, true)
	task.Attachments = append(task.Attachments, newAttachment("document.txt", ref, size, ""))
	root.SubTasks = append(root.SubTasks, task)
	return root, ref, nil
}

func contentRef(content []byte) string {
	hash := sha256.Sum256(content)
	return hex.EncodeToString(hash[:])
}

func restoreEntries(t *testing.T, root *Task, ref string, content []byte) []restoreArchiveEntry {
	t.Helper()
	return []restoreArchiveEntry{
		{name: "tasks.json", content: encodeRestoreTree(t, root)},
		{name: blobDirName + "/" + ref, content: content},
	}
}

func encodeRestoreTree(t *testing.T, root *Task) []byte {
	t.Helper()
	data, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func makeRestoreArchive(t *testing.T, entries []restoreArchiveEntry) []byte {
	t.Helper()
	var output bytes.Buffer
	archive := zip.NewWriter(&output)
	for _, entry := range entries {
		file, err := archive.Create(entry.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write(entry.content); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func storeTaskName(t *testing.T, store *Store, name string) {
	t.Helper()
	if err := store.Update(defaultUserID, func(root *Task) error {
		root.SubTasks = []*Task{newTask(name, "", defaultForumID, defaultUserID, true)}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func assertStoredTaskName(t *testing.T, store *Store, want string) {
	t.Helper()
	root, err := store.Load(defaultUserID)
	if err != nil {
		t.Fatal(err)
	}
	if len(root.SubTasks) != 1 || root.SubTasks[0].Name != want {
		t.Fatalf("stored task tree = %#v, want one task named %s", root.SubTasks, want)
	}
}

func assertNoRestoreStages(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), restoreStagePrefix) {
			t.Fatalf("restore staging entry remains: %s", entry.Name())
		}
	}
}

func withArchiveLimit(limits restoreLimits, value int64) restoreLimits {
	limits.archiveBytes = value
	return limits
}

func withEntryLimit(limits restoreLimits, value int) restoreLimits {
	limits.entries = value
	return limits
}

func withTaskLimit(limits restoreLimits, value int64) restoreLimits {
	limits.taskBytes = value
	return limits
}

func withBlobLimit(limits restoreLimits, value int64) restoreLimits {
	limits.blobBytes = value
	return limits
}

func withTotalBlobLimit(limits restoreLimits, value int64) restoreLimits {
	limits.totalBlobBytes = value
	return limits
}
