package main

import (
	"strings"
	"testing"
)

func testAttachment(id, name string) *Attachment {
	return &Attachment{Id: id, Name: name, Blob: strings.Repeat("a", 64), Size: 1}
}

func TestDocumentResolutionDeepestWinsPerBranch(t *testing.T) {
	root := &Task{
		Id:   rootPath,
		Name: "root",
		Attachments: []*Attachment{
			testAttachment("v1", "spec.md"),
			testAttachment("v2", "spec.md"),
		},
		SubTasks: []*Task{
			{Id: "a", Name: "A", Attachments: []*Attachment{testAttachment("a1", "spec.md")}},
			{Id: "b", Name: "B"},
		},
	}

	nodeA := buildDetailNode(FindTaskChain(rootPath+"/a", root), "/quester/", "/next")
	if len(nodeA.Documents) != 1 {
		t.Fatalf("branch A documents = %#v, want 1", nodeA.Documents)
	}
	if doc := nodeA.Documents[0]; doc.Version != 3 || doc.Origin != "A" {
		t.Fatalf("branch A doc = %#v, want v3 from A", doc)
	}

	nodeB := buildDetailNode(FindTaskChain(rootPath+"/b", root), "/quester/", "/next")
	if len(nodeB.Documents) != 1 {
		t.Fatalf("branch B documents = %#v, want 1", nodeB.Documents)
	}
	doc := nodeB.Documents[0]
	if doc.Version != 2 || doc.Origin != "root" {
		t.Fatalf("branch B doc = %#v, want v2 from root", doc)
	}
	if !strings.Contains(doc.URL, "doc=v2") || !strings.Contains(doc.URL, "q="+rootPath) {
		t.Fatalf("branch B doc URL = %q, want link to v2 on the root task", doc.URL)
	}
}

func TestBuildDetailNodeSkipsDeletedAncestorDocuments(t *testing.T) {
	root := &Task{
		Id:          rootPath,
		Name:        "root",
		Attachments: []*Attachment{testAttachment("v1", "spec.md")},
		SubTasks: []*Task{
			{
				Id:          "a",
				Name:        "A",
				Deleted:     true,
				Attachments: []*Attachment{testAttachment("a1", "spec.md")},
				SubTasks:    []*Task{{Id: "c", Name: "C"}},
			},
		},
	}

	node := buildDetailNode(FindTaskChain(rootPath+"/a/c", root), "/quester/", "/next")
	if len(node.Documents) != 1 || node.Documents[0].Version != 1 || node.Documents[0].Origin != "root" {
		t.Fatalf("documents under deleted ancestor = %#v, want v1 from root", node.Documents)
	}
}

func TestHumanSize(t *testing.T) {
	cases := map[int64]string{
		512:     "512 B",
		2048:    "2 KB",
		5 << 20: "5.0 MB",
		3 << 30: "3.0 GB",
	}
	for size, want := range cases {
		if got := humanSize(size); got != want {
			t.Fatalf("humanSize(%d) = %q, want %q", size, got, want)
		}
	}
}
