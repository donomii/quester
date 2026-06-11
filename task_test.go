package main

import "testing"

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
}
