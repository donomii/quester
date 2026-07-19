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
