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
			{Id: "v2", Name: "renamed-spec.md", Blob: strings.Repeat("b", 64), Size: 2, Replaces: "v1"},
		},
		SubTasks: []*Task{
			{Id: "a", Name: "A", Attachments: []*Attachment{{Id: "a1", Name: "agent-output.md", Blob: strings.Repeat("c", 64), Size: 3, Replaces: "v2"}}},
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
	if doc.Version != 2 || doc.Origin != "root" || doc.Name != "renamed-spec.md" {
		t.Fatalf("branch B doc = %#v, want v2 from root", doc)
	}
	if !strings.Contains(doc.URL, "doc=v2") || !strings.Contains(doc.URL, "q="+rootPath) {
		t.Fatalf("branch B doc URL = %q, want link to v2 on the root task", doc.URL)
	}
}

func TestMatchingFilenamesRemainSeparateWithoutReplacementLink(t *testing.T) {
	root := &Task{
		Id:   rootPath,
		Name: "root",
		Attachments: []*Attachment{
			{Id: "first", Name: "ticket.pdf", Blob: strings.Repeat("1", 64), Size: 1},
			{Id: "second", Name: "ticket.pdf", Blob: strings.Repeat("2", 64), Size: 2},
		},
	}

	node := buildTaskNode(root, rootPath, "/quester/", "/next", 0)
	if len(node.Documents) != 2 || node.Documents[0].Version != 1 || node.Documents[1].Version != 1 {
		t.Fatalf("matching filenames resolved as %#v, want two independent documents", node.Documents)
	}
}

func TestParallelVersionsShowDistinctRefs(t *testing.T) {
	blob := func(digit string) string { return strings.Repeat(digit, 64) }
	root := &Task{
		Id:          rootPath,
		Name:        "root",
		Attachments: []*Attachment{{Id: "v1", Name: "spec.md", Blob: blob("1"), Size: 1}},
		SubTasks: []*Task{
			{Id: "a", Name: "A", Attachments: []*Attachment{{Id: "a1", Name: "a.md", Blob: blob("2"), Size: 2, Replaces: "v1"}}},
			{Id: "b", Name: "B", Attachments: []*Attachment{{Id: "b1", Name: "b.md", Blob: blob("3"), Size: 3, Replaces: "v1"}}},
			{Id: "c", Name: "C", Attachments: []*Attachment{{Id: "c1", Name: "c.md", Blob: blob("2"), Size: 2, Replaces: "v1"}}},
		},
	}

	node := buildTaskNode(root, rootPath, "/quester/", "/next", 0)
	a, b, c := node.Children[0], node.Children[1], node.Children[2]

	// Sibling branches both carry a "v2", so the ref is what tells them apart.
	if a.Attachments[0].Version != 2 || b.Attachments[0].Version != 2 {
		t.Fatalf("sibling versions = %d, %d, want 2 and 2", a.Attachments[0].Version, b.Attachments[0].Version)
	}
	if a.Attachments[0].Ref == b.Attachments[0].Ref {
		t.Fatalf("different content shares ref %q", a.Attachments[0].Ref)
	}
	if a.Attachments[0].Ref != "22222222" {
		t.Fatalf("ref = %q, want first 8 blob chars", a.Attachments[0].Ref)
	}
	// Identical content shows the identical ref, wherever it is attached.
	if a.Attachments[0].Ref != c.Attachments[0].Ref {
		t.Fatalf("identical content refs differ: %q vs %q", a.Attachments[0].Ref, c.Attachments[0].Ref)
	}

	// The resolved set is available on every tree node for the in-effect line.
	if len(node.Documents) != 1 || node.Documents[0].Ref != "11111111" {
		t.Fatalf("root documents = %#v, want v1 ref 11111111", node.Documents)
	}
	if len(a.Documents) != 1 || a.Documents[0].Ref != "22222222" {
		t.Fatalf("branch A documents = %#v, want its own v2", a.Documents)
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

func TestBuildDocumentHistoryChainAndBelow(t *testing.T) {
	blob := func(digit string) string { return strings.Repeat(digit, 64) }
	grandchild := &Task{Id: "c", Name: "C", Attachments: []*Attachment{{Id: "c1", Name: "final.md", Blob: blob("3"), Size: 3, Replaces: "a1"}}}
	childA := &Task{Id: "a", Name: "A", Attachments: []*Attachment{{Id: "a1", Name: "draft.md", Blob: blob("2"), Size: 2, Replaces: "r1"}}, SubTasks: []*Task{grandchild}}
	childB := &Task{Id: "b", Name: "B"}
	root := &Task{
		Id:   rootPath,
		Name: "root",
		Attachments: []*Attachment{
			{Id: "r1", Name: "spec.md", Blob: blob("1"), Size: 1},
			{Id: "r2", Name: "notes.txt", Blob: blob("9"), Size: 9},
		},
		SubTasks: []*Task{childA, childB},
	}

	atA := buildDocumentHistory(root, "a1", "/quester/")
	if len(atA.Chain) != 2 || atA.Chain[0].Ref != "11111111" || atA.Chain[1].Ref != "22222222" || atA.Chain[1].Version != 2 {
		t.Fatalf("chain at A = %#v, want root v1 then A v2", atA.Chain)
	}
	if len(atA.Below) != 1 || atA.Below[0].Ref != "33333333" || atA.Below[0].Version != 3 || atA.Below[0].Origin != "C" {
		t.Fatalf("below A = %#v, want C v3", atA.Below)
	}

	atB := buildDocumentHistory(root, "r2", "/quester/")
	if len(atB.Chain) != 1 || atB.Chain[0].Ref != "99999999" || len(atB.Below) != 0 {
		t.Fatalf("history for independent document = %#v, want only notes", atB)
	}

	atRoot := buildDocumentHistory(root, "r1", "/quester/")
	if len(atRoot.Chain) != 1 || len(atRoot.Below) != 2 {
		t.Fatalf("history at root = %#v, want v1 above and A+C below", atRoot)
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
