package main

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestFindTaskHandlesRootAndNestedPaths(t *testing.T) {
	root := &Task{
		Id:   rootPath,
		Name: "root",
		SubTasks: []*Task{
			{
				Id:   "a",
				Name: "A",
				SubTasks: []*Task{
					{Id: "b", Name: "B"},
				},
			},
		},
	}

	if got := FindTask("", root); got != root {
		t.Fatalf("empty path returned %p, want root %p", got, root)
	}
	if got := FindTask(rootPath, root); got != root {
		t.Fatalf("root path returned %p, want root %p", got, root)
	}
	if got := FindTask(rootPath+"/a/b", root); got == nil || got.Name != "B" {
		t.Fatalf("nested path returned %#v, want task B", got)
	}
	if got := FindTask("a/b", root); got == nil || got.Name != "B" {
		t.Fatalf("relative nested path returned %#v, want task B", got)
	}
	if got := FindTask(rootPath+"/missing", root); got != nil {
		t.Fatalf("missing path returned %#v, want nil", got)
	}
}

func TestParentPath(t *testing.T) {
	if got := parentPath(rootPath); got != rootPath {
		t.Fatalf("parentPath(root) = %q, want %q", got, rootPath)
	}
	if got := parentPath(rootPath + "/a"); got != rootPath {
		t.Fatalf("parentPath(root/a) = %q, want %q", got, rootPath)
	}
	if got := parentPath(rootPath + "/a/b"); got != rootPath+"/a" {
		t.Fatalf("parentPath(root/a/b) = %q, want %q", got, rootPath+"/a")
	}
}

func TestNormalizeTreeFillsMissingFields(t *testing.T) {
	root := normalizeTree(&Task{
		SubTasks: []*Task{{Text: "body"}},
	})

	if root.Id != rootPath {
		t.Fatalf("root id = %q, want %q", root.Id, rootPath)
	}
	if root.Name != "Quester" {
		t.Fatalf("root name = %q, want Quester", root.Name)
	}
	if len(root.SubTasks) != 1 {
		t.Fatalf("subtask count = %d, want 1", len(root.SubTasks))
	}
	if root.SubTasks[0].Id == "" {
		t.Fatal("subtask id was not filled")
	}
	if root.SubTasks[0].Name != "Untitled task" {
		t.Fatalf("subtask name = %q, want Untitled task", root.SubTasks[0].Name)
	}
	if root.Schema != currentSchema || len(root.Forums) != 1 || root.SubTasks[0].ForumId != defaultForumID {
		t.Fatalf("normalized forum metadata = schema %d forums %#v child forum %q", root.Schema, root.Forums, root.SubTasks[0].ForumId)
	}
	if !root.SubTasks[0].Track {
		t.Fatal("legacy task did not retain task status")
	}
}

func TestMoveTaskPreservesNodeAndSubtree(t *testing.T) {
	child := &Task{Id: "child", Name: "Child", ForumId: defaultForumID}
	node := &Task{Id: "node", Name: "Node", ForumId: defaultForumID, SubTasks: []*Task{child}}
	root := normalizeTree(&Task{
		Id:       rootPath,
		Schema:   currentSchema,
		Forums:   []*Forum{defaultForum(), {Id: "trips", Name: "Trips"}},
		Users:    []*User{defaultUser()},
		SubTasks: []*Task{node},
	})

	if err := moveTask(root, node.Id, "", "trips", "Node"); err != nil {
		t.Fatal(err)
	}
	if got := FindTask(node.Id, root); got != node || got.ForumId != "trips" || !got.Track || got.SubTasks[0] != child || child.ForumId != "trips" {
		t.Fatalf("moved node = %#v child = %#v", got, child)
	}
	if err := moveTask(root, node.Id, child.Id, defaultForumID, ""); err == nil {
		t.Fatal("moving a node beneath its own child succeeded")
	}
}

func TestTaskMetadataParsingAndValidation(t *testing.T) {
	due, priority, tags, err := parseTaskMetadata("2026-08-15", " HIGH ", "work, Home, work")
	if err != nil {
		t.Fatal(err)
	}
	if due != "2026-08-15" || priority != "high" || !reflect.DeepEqual(tags, []string{"work", "Home"}) {
		t.Fatalf("metadata = %q %q %#v", due, priority, tags)
	}
	for _, test := range []struct {
		due      string
		priority string
		tags     string
	}{
		{due: "15-08-2026", priority: "normal"},
		{priority: "immediate"},
		{priority: "normal", tags: strings.Repeat("x", maxTagLength+1)},
	} {
		if _, _, _, err := parseTaskMetadata(test.due, test.priority, test.tags); err == nil {
			t.Fatalf("parseTaskMetadata(%q, %q, %q) succeeded", test.due, test.priority, test.tags)
		}
	}
	if _, _, _, err := parseTaskMetadataTags("", "normal", []string{"one,two"}); err == nil {
		t.Fatal("array tag containing a comma was accepted")
	}
}

func TestOlderTaskMetadataGetsSensibleDefaults(t *testing.T) {
	root := normalizeTree(&Task{Id: rootPath, SubTasks: []*Task{{Id: "old", Name: "Old"}}})
	task := root.SubTasks[0]
	if task.Priority != defaultPriority || task.DueDate != "" || len(task.Tags) != 0 {
		t.Fatalf("normalized task metadata = priority %q due %q tags %#v", task.Priority, task.DueDate, task.Tags)
	}
}

func TestBulkActionAndSelectedExport(t *testing.T) {
	child := newTask("Child", "", defaultForumID, defaultUserID, true)
	parent := newTask("Parent", "", defaultForumID, defaultUserID, true)
	parent.SubTasks = append(parent.SubTasks, child)
	other := newTask("Other", "", defaultForumID, defaultUserID, true)
	root := defaultRoot()
	root.SubTasks = []*Task{parent, other}
	if err := applyBulkTaskAction(root, []string{parent.Id, other.Id}, "check"); err != nil {
		t.Fatal(err)
	}
	if !parent.Checked || !other.Checked {
		t.Fatalf("bulk check states = %t %t", parent.Checked, other.Checked)
	}
	exported, err := selectedTaskExport(root, []string{parent.Id, child.Id})
	if err != nil {
		t.Fatal(err)
	}
	if len(exported.SubTasks) != 1 || exported.SubTasks[0].Id != parent.Id || len(exported.SubTasks[0].SubTasks) != 1 {
		t.Fatalf("selected export tasks = %#v", exported.SubTasks)
	}
	if err := applyBulkTaskAction(root, nil, "delete"); !errors.Is(err, errNoTasksSelected) {
		t.Fatalf("empty bulk action error = %v", err)
	}
}
