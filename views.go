package main

import (
	_ "embed"
	"fmt"
	"html/template"
	"maps"
	"net/url"
	"slices"
	"strings"
	"time"
)

//go:embed style.css
var styleCSS string

type PageData struct {
	Style      template.CSS
	Prefix     string
	CurrentURL string
	Title      string
	Filter     string
	RootPath   string
	Current    *TaskNode
	Summary    []*TaskNode
	Error      string
}

type TaskNode struct {
	ID          string
	Prefix      string
	Name        string
	Text        string
	Path        string
	Next        string
	Created     string
	DepthClass  string
	Checked     bool
	CanDelete   bool
	Attachments []*DocumentNode
	Documents   []*DocumentNode
	Children    []*TaskNode
}

// DocumentNode is one attachment prepared for rendering. Version counts the
// attachments sharing this Name along the path from the root, so the highest
// version at a node is the document in effect there.
type DocumentNode struct {
	Name      string
	URL       string
	Size      string
	Version   int
	Ref       string
	Attached  string
	Origin    string
	OriginURL string
}

// docTrail accumulates document versions along a root-to-node path so the
// deepest attachment of each file name wins.
type docTrail struct {
	counts  map[string]int
	current map[string]*DocumentNode
}

func newDocTrail() *docTrail {
	return &docTrail{counts: map[string]int{}, current: map[string]*DocumentNode{}}
}

func (t *docTrail) clone() *docTrail {
	return &docTrail{counts: maps.Clone(t.counts), current: maps.Clone(t.current)}
}

func (t *docTrail) add(task *Task, path, prefix string) []*DocumentNode {
	var added []*DocumentNode
	for _, attachment := range task.Attachments {
		if attachment == nil {
			continue
		}
		t.counts[attachment.Name]++
		doc := &DocumentNode{
			Name:      attachment.Name,
			URL:       prefix + "document?q=" + url.QueryEscape(path) + "&doc=" + url.QueryEscape(attachment.Id),
			Size:      humanSize(attachment.Size),
			Version:   t.counts[attachment.Name],
			Ref:       shortRef(attachment.Blob),
			Attached:  formatTaskTime(attachment.TimeStamp),
			Origin:    task.Name,
			OriginURL: prefix + "detailed?q=" + url.QueryEscape(path),
		}
		t.current[attachment.Name] = doc
		added = append(added, doc)
	}
	return added
}

func (t *docTrail) snapshot() []*DocumentNode {
	docs := slices.Collect(maps.Values(t.current))
	slices.SortFunc(docs, func(a, b *DocumentNode) int { return strings.Compare(a.Name, b.Name) })
	return docs
}

func buildTaskNode(task *Task, path, prefix, next string, depth int) *TaskNode {
	return buildTaskNodeWithTrail(task, path, prefix, next, depth, newDocTrail())
}

// buildTaskNodeWithTrail renders a subtree. The trail carries document
// versions inherited from ancestors and is cloned per child so sibling
// branches stay isolated.
func buildTaskNodeWithTrail(task *Task, path, prefix, next string, depth int, trail *docTrail) *TaskNode {
	path = normalizedPath(path)
	node := &TaskNode{
		ID:         task.Id,
		Prefix:     prefix,
		Name:       task.Name,
		Text:       task.Text,
		Path:       path,
		Next:       next,
		Created:    formatTaskTime(task.TimeStamp),
		DepthClass: oddEven(depth) + "-depth",
		Checked:    task.Checked,
		CanDelete:  !isRootPath(path),
	}
	node.Attachments = trail.add(task, path, prefix)
	node.Documents = trail.snapshot()
	for _, child := range visibleChildren(task) {
		node.Children = append(node.Children, buildTaskNodeWithTrail(child, joinTaskPath(path, child.Id), prefix, next, depth+1, trail.clone()))
	}
	return node
}

// buildDetailNode renders the task at the end of chain with the document
// state inherited from its ancestors.
func buildDetailNode(chain []*Task, prefix, next string) *TaskNode {
	trail := newDocTrail()
	path := rootPath
	for i, task := range chain {
		if i > 0 {
			path = joinTaskPath(path, task.Id)
		}
		if i == len(chain)-1 {
			return buildTaskNodeWithTrail(task, path, prefix, next, 0, trail)
		}
		if !task.Deleted {
			trail.add(task, path, prefix)
		}
	}
	return nil
}

// shortRef abbreviates a blob reference for display, like an abbreviated git
// hash. Version numbers are per-branch, so parallel branches can each carry a
// "v2" of the same name; the content id is what tells them apart — and equal
// ids mean identical bytes.
func shortRef(ref string) string {
	if len(ref) > 8 {
		return ref[:8]
	}
	return ref
}

func humanSize(size int64) string {
	switch {
	case size < 1<<10:
		return fmt.Sprintf("%d B", size)
	case size < 1<<20:
		return fmt.Sprintf("%d KB", size>>10)
	case size < 1<<30:
		return fmt.Sprintf("%.1f MB", float64(size)/(1<<20))
	default:
		return fmt.Sprintf("%.1f GB", float64(size)/(1<<30))
	}
}

func formatTaskTime(t time.Time) string {
	if t.IsZero() {
		return "not dated"
	}
	return t.Local().Format("2006-01-02 15:04")
}

func oddEven(i int) string {
	if i%2 == 0 {
		return "even"
	}
	return "odd"
}

func newTemplates() *template.Template {
	return template.Must(template.New("quester").Parse(pageTemplates))
}

const pageTemplates = `
{{define "header"}}
<!doctype html>
<html lang="en">
<head>
	<meta charset="utf-8">
	<meta name="viewport" content="width=device-width, initial-scale=1">
	<title>{{.Title}}</title>
	<style>{{.Style}}</style>
</head>
<body>
	<nav class="topbar">
		<a class="brand" href="{{.Prefix}}summary">unfinished business</a>
		<div class="nav-actions">
			<a href="{{.Prefix}}downloadAll">Backup</a>
			<a href="{{.Prefix}}restoreAllPage">Restore</a>
		</div>
	</nav>
	<header class="masthead">
		<a class="main" href="{{.Prefix}}summary"><h1>unfinished business</h1></a>
		<ul class="tabmenu">
			<li {{if eq .Filter ""}}class="active"{{end}}><a href="{{.Prefix}}summary">all</a></li>
			<li {{if eq .Filter "new"}}class="active"{{end}}><a href="{{.Prefix}}summary?filter=new">open</a></li>
		</ul>
	</header>
	<section id="intro">
		<h2>Hierarchical task tracking in a comment-thread shape.</h2>
	</section>
	{{if .Error}}<div class="notice">{{.Error}}</div>{{end}}
	<main class="sr" id="links">
{{end}}

{{define "footer"}}
	</main>
</body>
</html>
{{end}}

{{define "summary"}}
{{template "header" .}}
	{{if .Summary}}
		{{range .Summary}}{{template "summaryItem" .}}{{end}}
	{{else}}
		<p class="empty">No tasks yet.</p>
	{{end}}
	{{template "taskForms" .}}
{{template "footer" .}}
{{end}}

{{define "detail"}}
{{template "header" .}}
	{{template "documents" .Current}}
	{{template "taskTree" .Current}}
	{{template "taskForms" .}}
{{template "footer" .}}
{{end}}

{{define "documents"}}
	{{if .Documents}}
	<section class="panel documents">
		<h2>Documents at &ldquo;{{.Name}}&rdquo;</h2>
		<ul class="attachments">
			{{range .Documents}}
			<li><a href="{{.URL}}">{{.Name}}</a> <span class="meta">v{{.Version}} · {{.Ref}} · {{.Size}} · attached to <a href="{{.OriginURL}}">{{.Origin}}</a> · {{.Attached}}</span></li>
			{{end}}
		</ul>
	</section>
	{{end}}
{{end}}

{{define "restore"}}
{{template "header" .}}
	<section class="panel">
		<h2>Restore from backup</h2>
		<p>Restoring replaces the current task tree for this user.</p>
		<form action="{{.Prefix}}restoreAll" method="post" enctype="multipart/form-data">
			<label for="content">Backup JSON</label>
			<input type="file" id="content" name="content" accept="application/json,.json" required>
			<button type="submit">Restore</button>
		</form>
	</section>
{{template "footer" .}}
{{end}}

{{define "error"}}
{{template "header" .}}
	<section class="panel">
		<h2>{{.Title}}</h2>
		<p>{{.Error}}</p>
	</section>
{{template "footer" .}}
{{end}}

{{define "summaryItem"}}
	<article class="link {{if .Checked}}is-checked{{end}}">
		<form class="vote toggle-form" action="{{.Prefix}}toggle" method="post">
			<input type="hidden" name="path" value="{{.Path}}">
			<input type="hidden" name="next" value="{{.Next}}">
			<button type="submit" aria-label="Toggle {{.Name}}">{{if .Checked}}done{{else}}open{{end}}</button>
		</form>
		<div class="entry">
			<h2><a href="{{.Prefix}}detailed?q={{.Path}}">{{.Name}}</a></h2>
			{{if .Text}}<p class="selftext">{{.Text}}</p>{{end}}
			<p class="meta">submitted {{.Created}} <a class="comments" href="{{.Prefix}}detailed?q={{.Path}}">{{len .Children}} comments</a>{{if .Attachments}} <span class="comments">{{len .Attachments}} attached</span>{{end}}</p>
		</div>
	</article>
{{end}}

{{define "taskTree"}}
	<article class="comment {{.DepthClass}} {{if .Checked}}is-checked{{end}}">
		<details open>
			<summary>{{.Name}}</summary>
			<div class="body">
				<form class="toggle-form" action="{{.Prefix}}toggle" method="post">
					<input type="hidden" name="path" value="{{.Path}}">
					<input type="hidden" name="next" value="{{.Next}}">
					<button type="submit">{{if .Checked}}done{{else}}open{{end}}</button>
				</form>
				<div class="md">
					<h2>{{.Name}}</h2>
					{{if .Text}}<p>{{.Text}}</p>{{end}}
				</div>
				{{if .Attachments}}
				<ul class="attachments">
					{{range .Attachments}}
					<li><a href="{{.URL}}">{{.Name}}</a> <span class="meta">v{{.Version}} · {{.Ref}} · {{.Size}}</span></li>
					{{end}}
				</ul>
				{{end}}
				{{if .Documents}}
				<details class="docstate">
					<summary>Documents in effect: {{len .Documents}}</summary>
					<ul class="attachments">
						{{range .Documents}}
						<li><a href="{{.URL}}">{{.Name}}</a> <span class="meta">v{{.Version}} · {{.Ref}} · {{.Size}} · attached to <a href="{{.OriginURL}}">{{.Origin}}</a></span></li>
						{{end}}
					</ul>
				</details>
				{{end}}
				<div class="links">
					<a href="{{.Prefix}}detailed?q={{.Path}}">Open</a>
					{{if .CanDelete}}
					<form class="inline-form" action="{{.Prefix}}deleteWaypoint" method="post">
						<input type="hidden" name="q" value="{{.Path}}">
						<button type="submit">Delete</button>
					</form>
					{{end}}
				</div>
				{{range .Children}}{{template "taskTree" .}}{{end}}
			</div>
		</details>
	</article>
{{end}}

{{define "taskForms"}}
	<section class="forms">
		<form action="{{.Prefix}}addWaypoint" method="post" enctype="multipart/form-data">
			<h2>Add task under &ldquo;{{.Current.Name}}&rdquo;</h2>
			<input type="hidden" name="q" value="{{.Current.Path}}">
			<label for="title">Title</label>
			<input id="title" name="title" type="text" autocomplete="off" required>
			<label for="content">Notes</label>
			<textarea id="content" name="content" rows="3"></textarea>
			<label for="document">Attach</label>
			<input id="document" name="document" type="file" multiple>
			<button type="submit">Add</button>
		</form>

		<form action="{{.Prefix}}attachDocument" method="post" enctype="multipart/form-data">
			<h2>Attach documents to &ldquo;{{.Current.Name}}&rdquo;</h2>
			<input type="hidden" name="q" value="{{.Current.Path}}">
			<label for="attach-document">Files</label>
			<input id="attach-document" name="document" type="file" multiple required>
			<button type="submit">Attach</button>
			<p class="meta">Reusing a file name adds a new version for this task and everything under it.</p>
		</form>

		<form action="{{.Prefix}}editWaypoint" method="post">
			<h2>Edit &ldquo;{{.Current.Name}}&rdquo;</h2>
			<input type="hidden" name="q" value="{{.Current.Path}}">
			<label for="edit-title">Title</label>
			<input id="edit-title" name="title" type="text" value="{{.Current.Name}}" autocomplete="off" required>
			<label for="edit-content">Notes</label>
			<textarea id="edit-content" name="content" rows="3">{{.Current.Text}}</textarea>
			<button type="submit">Update</button>
		</form>

		{{if .Current.CanDelete}}
		<form action="{{.Prefix}}deleteWaypoint" method="post">
			<h2>Delete &ldquo;{{.Current.Name}}&rdquo;</h2>
			<input type="hidden" name="q" value="{{.Current.Path}}">
			<button class="danger" type="submit">Delete</button>
		</form>
		{{end}}
	</section>
{{end}}
`

func trimPrefix(prefix string) string {
	return "/" + strings.Trim(prefix, "/")
}
