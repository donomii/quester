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
	Style        template.CSS
	Prefix       string
	CurrentURL   string
	Title        string
	Filter       string
	Sort         string
	RootPath     string
	CSRF         string
	Query        string
	Current      *TaskNode
	Summary      []*TaskNode
	Results      []*TaskNode
	DeletedTasks []*TaskNode
	History      *DocumentHistory
	Garbage      []*BlobNode
	Forums       []*ForumNode
	ForumID      string
	Notice       string
	Error        string
}

type ForumNode struct {
	ID          string
	Name        string
	Description string
	URL         string
	Active      bool
}

type TaskNode struct {
	ID          string
	Prefix      string
	Name        string
	Text        string
	Path        string
	Next        string
	CSRF        string
	Created     string
	DepthClass  string
	Checked     bool
	Track       bool
	Author      string
	AgentAuthor bool
	ForumID     string
	ForumName   string
	IsRoot      bool
	CanDelete   bool
	Attachments []*DocumentNode
	Documents   []*DocumentNode
	Children    []*TaskNode
}

// stampCSRF spreads the per-request form token over a rendered subtree; the
// token is minted at render time, after the nodes are built.
func stampCSRF(node *TaskNode, token string) {
	if node == nil {
		return
	}
	node.CSRF = token
	for _, child := range node.Children {
		stampCSRF(child, token)
	}
}

// BlobNode is one stored blob file prepared for the cleanup listing.
type BlobNode struct {
	Ref    string
	Short  string
	Size   string
	Stored string
}

func buildBlobNodes(blobs []BlobInfo) []*BlobNode {
	nodes := make([]*BlobNode, 0, len(blobs))
	for _, blob := range blobs {
		nodes = append(nodes, &BlobNode{
			Ref:    blob.Ref,
			Short:  shortRef(blob.Ref),
			Size:   humanSize(blob.Size),
			Stored: formatTaskTime(blob.ModTime),
		})
	}
	return nodes
}

// buildSearchResults flattens every visible node whose title or notes
// contain the query, case-insensitively, in tree order.
func buildSearchResults(root *Task, query, prefix, next string) []*TaskNode {
	needle := strings.ToLower(query)
	trail := newDocTrail(root)
	var results []*TaskNode
	var walk func(*Task)
	walk = func(parent *Task) {
		for _, task := range visibleChildren(parent) {
			if taskMatches(task, needle) {
				results = append(results, &TaskNode{
					ID:          task.Id,
					Prefix:      prefix,
					Name:        task.Name,
					Text:        task.Text,
					Path:        task.Id,
					Next:        next,
					Created:     formatTaskTime(task.TimeStamp),
					Checked:     task.Checked,
					Track:       task.Track,
					Author:      trail.authorName(task.AuthorId),
					AgentAuthor: trail.agentAuthor(task.AuthorId),
					ForumID:     task.ForumId,
					ForumName:   trail.forumName(task.ForumId),
				})
			}
			walk(task)
		}
	}
	walk(root)
	return results
}

func buildDeletedTaskNodes(root *Task, prefix string) []*TaskNode {
	trail := newDocTrail(root)
	var nodes []*TaskNode
	var walk func(*Task)
	walk = func(parent *Task) {
		for _, task := range parent.SubTasks {
			if task == nil {
				continue
			}
			if task.Deleted {
				nodes = append(nodes, &TaskNode{
					ID:          task.Id,
					Prefix:      prefix,
					Name:        task.Name,
					Text:        task.Text,
					Path:        task.Id,
					Created:     formatTaskTime(task.TimeStamp),
					Checked:     task.Checked,
					Track:       task.Track,
					Author:      trail.authorName(task.AuthorId),
					AgentAuthor: trail.agentAuthor(task.AuthorId),
					ForumID:     task.ForumId,
					ForumName:   trail.forumName(task.ForumId),
				})
			}
			walk(task)
		}
	}
	walk(root)
	return nodes
}

// DocumentNode is one attachment prepared for rendering. Version counts the
// attachments sharing this Name along the path from the root, so the highest
// version at a node is the document in effect there.
type DocumentNode struct {
	ID        string
	Name      string
	URL       string
	Size      string
	Version   int
	Ref       string
	Attached  string
	Origin    string
	OriginURL string
	Replaces  string
}

// docTrail accumulates document versions along a root-to-node path so the
// deepest attachment of each file name wins.
type docTrail struct {
	counts  map[string]int
	current map[string]*DocumentNode
	roots   map[string]string
	users   map[string]*User
	forums  map[string]*Forum
}

func newDocTrail(roots ...*Task) *docTrail {
	t := &docTrail{
		counts:  map[string]int{},
		current: map[string]*DocumentNode{},
		roots:   map[string]string{},
		users:   map[string]*User{},
		forums:  map[string]*Forum{},
	}
	if len(roots) > 0 && roots[0] != nil {
		for _, user := range roots[0].Users {
			if user != nil {
				t.users[user.Id] = user
			}
		}
		for _, forum := range roots[0].Forums {
			if forum != nil {
				t.forums[forum.Id] = forum
			}
		}
	}
	return t
}

func (t *docTrail) clone() *docTrail {
	return &docTrail{
		counts:  maps.Clone(t.counts),
		current: maps.Clone(t.current),
		roots:   maps.Clone(t.roots),
		users:   t.users,
		forums:  t.forums,
	}
}

func (t *docTrail) add(task *Task, path, prefix string) []*DocumentNode {
	var added []*DocumentNode
	for _, attachment := range task.Attachments {
		if attachment == nil {
			continue
		}
		rootID := attachment.Id
		if replacedRoot := t.roots[attachment.Replaces]; replacedRoot != "" {
			rootID = replacedRoot
		}
		t.roots[attachment.Id] = rootID
		t.counts[rootID]++
		doc := documentNode(attachment, task, path, prefix, t.counts[rootID])
		t.current[rootID] = doc
		added = append(added, doc)
	}
	return added
}

func documentNode(attachment *Attachment, task *Task, path, prefix string, version int) *DocumentNode {
	return &DocumentNode{
		ID:        attachment.Id,
		Name:      attachment.Name,
		URL:       prefix + "document?q=" + url.QueryEscape(path) + "&doc=" + url.QueryEscape(attachment.Id),
		Size:      humanSize(attachment.Size),
		Version:   version,
		Ref:       shortRef(attachment.Blob),
		Attached:  formatTaskTime(attachment.TimeStamp),
		Origin:    task.Name,
		OriginURL: prefix + "detailed?q=" + url.QueryEscape(path),
		Replaces:  attachment.Replaces,
	}
}

func (t *docTrail) snapshot() []*DocumentNode {
	docs := slices.Collect(maps.Values(t.current))
	slices.SortFunc(docs, func(a, b *DocumentNode) int { return strings.Compare(a.Name, b.Name) })
	return docs
}

func buildTaskNode(task *Task, path, prefix, next string, depth int) *TaskNode {
	return buildTaskNodeWithTrail(task, path, prefix, next, depth, newDocTrail(task))
}

// buildTaskNodeWithTrail renders a subtree. The trail carries document
// versions inherited from ancestors and is cloned per child so sibling
// branches stay isolated.
func buildTaskNodeWithTrail(task *Task, path, prefix, next string, depth int, trail *docTrail) *TaskNode {
	path = normalizedPath(path)
	node := &TaskNode{
		ID:          task.Id,
		Prefix:      prefix,
		Name:        task.Name,
		Text:        task.Text,
		Path:        task.Id,
		Next:        next,
		Created:     formatTaskTime(task.TimeStamp),
		DepthClass:  oddEven(depth) + "-depth",
		Checked:     task.Checked,
		Track:       task.Track,
		Author:      trail.authorName(task.AuthorId),
		AgentAuthor: trail.agentAuthor(task.AuthorId),
		ForumID:     task.ForumId,
		ForumName:   trail.forumName(task.ForumId),
		IsRoot:      task.Id == rootPath,
		CanDelete:   task.Id != rootPath,
	}
	node.Attachments = trail.add(task, task.Id, prefix)
	node.Documents = trail.snapshot()
	for _, child := range visibleChildren(task) {
		node.Children = append(node.Children, buildTaskNodeWithTrail(child, child.Id, prefix, next, depth+1, trail.clone()))
	}
	return node
}

func buildForumNodes(root *Task, activeID, prefix string) []*ForumNode {
	forums := make([]*ForumNode, 0, len(root.Forums))
	for _, forum := range root.Forums {
		if forum == nil {
			continue
		}
		forums = append(forums, &ForumNode{
			ID:          forum.Id,
			Name:        forum.Name,
			Description: forum.Description,
			URL:         prefix + "summary?forum=" + url.QueryEscape(forum.Id),
			Active:      forum.Id == activeID,
		})
	}
	return forums
}

func (t *docTrail) authorName(id string) string {
	if user := t.users[id]; user != nil {
		return user.Name
	}
	return cleanUserID(id)
}

func (t *docTrail) agentAuthor(id string) bool {
	return t.users[id] != nil && t.users[id].IsAgent
}

func (t *docTrail) forumName(id string) string {
	if forum := t.forums[id]; forum != nil {
		return forum.Name
	}
	return cleanForumID(id)
}

// DocumentHistory follows explicit Replaces links. Chain ends at the selected
// attachment; Below contains revisions that descend from it.
type DocumentHistory struct {
	Name     string
	TaskName string
	TaskURL  string
	Chain    []*DocumentNode
	Below    []*DocumentNode
}

type attachedRecord struct {
	task       *Task
	attachment *Attachment
}

func buildDocumentHistory(root *Task, attachmentID, prefix string) *DocumentHistory {
	records := collectAttachmentRecords(root)
	byID := map[string]attachedRecord{}
	for _, record := range records {
		byID[record.attachment.Id] = record
	}
	target, found := byID[attachmentID]
	if !found {
		return nil
	}

	var lineage []attachedRecord
	seen := map[string]bool{}
	current := target
	for {
		if seen[current.attachment.Id] {
			return nil
		}
		seen[current.attachment.Id] = true
		lineage = append([]attachedRecord{current}, lineage...)
		if current.attachment.Replaces == "" {
			break
		}
		previous, exists := byID[current.attachment.Replaces]
		if !exists {
			break
		}
		current = previous
	}

	history := &DocumentHistory{
		Name:     target.attachment.Name,
		TaskName: target.task.Name,
		TaskURL:  prefix + "detailed?q=" + url.QueryEscape(target.task.Id),
	}
	versions := map[string]int{}
	for index, record := range lineage {
		version := index + 1
		versions[record.attachment.Id] = version
		history.Chain = append(history.Chain, documentNode(record.attachment, record.task, record.task.Id, prefix, version))
	}

	for added := true; added; {
		added = false
		for _, record := range records {
			if _, exists := versions[record.attachment.Id]; exists {
				continue
			}
			parentVersion, descends := versions[record.attachment.Replaces]
			if !descends {
				continue
			}
			version := parentVersion + 1
			versions[record.attachment.Id] = version
			history.Below = append(history.Below, documentNode(record.attachment, record.task, record.task.Id, prefix, version))
			added = true
		}
	}
	return history
}

func collectAttachmentRecords(root *Task) []attachedRecord {
	var records []attachedRecord
	var walk func(*Task)
	walk = func(task *Task) {
		if task == nil || task.Deleted {
			return
		}
		for _, attachment := range task.Attachments {
			if attachment != nil {
				records = append(records, attachedRecord{task: task, attachment: attachment})
			}
		}
		for _, child := range task.SubTasks {
			walk(child)
		}
	}
	walk(root)
	return records
}

// buildDetailNode renders the task at the end of chain with the document
// state inherited from its ancestors.
func buildDetailNode(chain []*Task, prefix, next string) *TaskNode {
	trail := newDocTrail(chain[0])
	for i, task := range chain {
		if i == len(chain)-1 {
			return buildTaskNodeWithTrail(task, task.Id, prefix, next, 0, trail)
		}
		if !task.Deleted {
			trail.add(task, task.Id, prefix)
		}
	}
	return nil
}

// shortRef abbreviates a blob reference for display.
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
				<a href="{{.Prefix}}search">Search</a>
				<a href="{{.Prefix}}deleted">Deleted</a>
				<a href="{{.Prefix}}cleanupPage">Cleanup</a>
				<a href="{{.Prefix}}api/">API</a>
				<a href="{{.Prefix}}downloadAll">Backup</a>
			<a href="{{.Prefix}}restoreAllPage">Restore</a>
		</div>
	</nav>
		<header class="masthead">
		<a class="main" href="{{.Prefix}}summary"><h1>unfinished business</h1></a>
			<ul class="tabmenu">
				<li {{if eq .Filter ""}}class="active"{{end}}><a href="{{.Prefix}}summary?forum={{.ForumID}}&sort={{.Sort}}">all</a></li>
				<li {{if eq .Filter "new"}}class="active"{{end}}><a href="{{.Prefix}}summary?forum={{.ForumID}}&filter=new&sort={{.Sort}}">open</a></li>
			</ul>
		</header>
		{{if .Forums}}
		<nav class="forumbar" aria-label="Forums">
			{{range .Forums}}<a {{if .Active}}class="active"{{end}} href="{{.URL}}">{{.Name}}</a>{{end}}
		</nav>
		{{end}}
		<section id="intro">
			{{range .Forums}}{{if .Active}}<h2>{{.Name}}{{if .Description}} — {{.Description}}{{end}}</h2>{{end}}{{end}}
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
	<form class="sort-form" action="{{.Prefix}}summary" method="get">
		<input type="hidden" name="forum" value="{{.ForumID}}">
		<input type="hidden" name="filter" value="{{.Filter}}">
		<label for="sort">Sort tasks</label>
		<select id="sort" name="sort">
			<option value="created" {{if eq .Sort "created"}}selected{{end}}>Created time</option>
			<option value="completion" {{if eq .Sort "completion"}}selected{{end}}>Completion</option>
			<option value="title" {{if eq .Sort "title"}}selected{{end}}>Title</option>
		</select>
		<button type="submit">Sort</button>
		<p class="meta">Created time shows newest first; completion shows open tasks first; title sorts alphabetically.</p>
	</form>
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
			{{$node := .}}
			{{range .Documents}}
				<li><a href="{{.URL}}">{{.Name}}</a> <span class="meta">v{{.Version}} · {{.Ref}} · {{.Size}} · attached to <a href="{{.OriginURL}}">{{.Origin}}</a> · {{.Attached}} · <a href="{{$node.Prefix}}documentHistory?q={{$node.Path}}&doc={{.ID}}">history</a></span></li>
			{{end}}
		</ul>
	</section>
	{{end}}
{{end}}

{{define "history"}}
{{template "header" .}}
	<section class="panel documents">
		<h2>&ldquo;{{.History.Name}}&rdquo; at &ldquo;{{.History.TaskName}}&rdquo;</h2>
		<p class="meta">Explicit revision chain ending at the selected file on <a href="{{.History.TaskURL}}">{{.History.TaskName}}</a>.</p>
		{{if .History.Chain}}
		<ul class="attachments">
			{{range .History.Chain}}
			<li><a href="{{.URL}}">{{.Name}}</a> <span class="meta">v{{.Version}} · {{.Ref}} · {{.Size}} · attached to <a href="{{.OriginURL}}">{{.Origin}}</a> · {{.Attached}}</span></li>
			{{end}}
		</ul>
		{{else}}
		<p class="meta">None above or at this task.</p>
		{{end}}
		<h2>Later revisions</h2>
		<p class="meta">Files that explicitly replace the selected revision or one of its descendants.</p>
		{{if .History.Below}}
		<ul class="attachments">
			{{range .History.Below}}
			<li><a href="{{.URL}}">{{.Name}}</a> <span class="meta">v{{.Version}} · {{.Ref}} · {{.Size}} · attached to <a href="{{.OriginURL}}">{{.Origin}}</a> · {{.Attached}}</span></li>
			{{end}}
		</ul>
		{{else}}
		<p class="meta">None.</p>
		{{end}}
	</section>
{{template "footer" .}}
{{end}}

{{define "restore"}}
{{template "header" .}}
	<section class="panel">
		<h2>Restore from backup</h2>
		<p>Restoring replaces the current task tree for this user.</p>
		<form action="{{.Prefix}}restoreAll" method="post" enctype="multipart/form-data">
			<input type="hidden" name="csrf" value="{{.CSRF}}">
			<label for="content">Backup file</label>
			<input type="file" id="content" name="content" accept="application/zip,.zip,application/json,.json" required>
			<p class="meta">Choose a self-contained Quester zip backup or a legacy tasks.json file.</p>
			<button type="submit">Restore</button>
		</form>
	</section>
{{template "footer" .}}
{{end}}

{{define "search"}}
{{template "header" .}}
	<section class="panel">
		<h2>Search tasks</h2>
		<form action="{{.Prefix}}search" method="get">
			<label for="query">Title or notes</label>
			<input id="query" name="q" type="text" value="{{.Query}}" autocomplete="off">
			<button type="submit">Search</button>
		</form>
	</section>
	{{range .Results}}{{template "summaryItem" .}}{{else}}{{if .Query}}<p class="empty">No matching tasks.</p>{{end}}{{end}}
{{template "footer" .}}
{{end}}

{{define "deleted"}}
{{template "header" .}}
	<section class="panel">
		<h2>Deleted tasks</h2>
		<p>Restoring a task makes it and its replies visible again. A task beneath a deleted parent remains hidden until that parent is also restored.</p>
		{{range .DeletedTasks}}
		<form action="{{.Prefix}}restoreWaypoint" method="post">
			<input type="hidden" name="csrf" value="{{.CSRF}}">
			<input type="hidden" name="q" value="{{.Path}}">
			<p><strong>{{if .Name}}{{.Name}}{{else}}Untitled reply{{end}}</strong> <span class="meta">{{.Created}} · {{.ForumName}} · node {{.ID}}</span></p>
			{{if .Text}}<p>{{.Text}}</p>{{end}}
			<button type="submit">Restore</button>
		</form>
		{{else}}<p class="empty">No deleted tasks.</p>{{end}}
	</section>
{{template "footer" .}}
{{end}}

{{define "cleanup"}}
{{template "header" .}}
	<section class="panel">
		<h2>Unreferenced attachment files</h2>
		{{if .Notice}}<p class="notice">{{.Notice}}</p>{{end}}
		{{if .Garbage}}
		<ul class="attachments">{{range .Garbage}}<li>{{.Short}} · {{.Size}} · stored {{.Stored}}</li>{{end}}</ul>
		<form action="{{.Prefix}}cleanup" method="post">
			<input type="hidden" name="csrf" value="{{.CSRF}}">
			<p>Cleanup permanently deletes the listed files. Files stored within the last hour are excluded so an upload can finish linking them to a task.</p>
			<button class="danger" type="submit">Delete listed files</button>
		</form>
		{{else}}<p>No unreferenced attachment files older than one hour.</p>{{end}}
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
			{{if .Track}}
			<form class="vote toggle-form" action="{{.Prefix}}toggle" method="post">
			<input type="hidden" name="csrf" value="{{.CSRF}}">
			<input type="hidden" name="path" value="{{.Path}}">
			<input type="hidden" name="next" value="{{.Next}}">
				<button type="submit" aria-label="Toggle {{.Name}}">{{if .Checked}}done{{else}}open{{end}}</button>
			</form>
			{{end}}
			<div class="entry">
				<h2><a href="{{.Prefix}}detailed?q={{.Path}}">{{.Name}}</a></h2>
				{{if .Text}}<p class="selftext">{{.Text}}</p>{{end}}
				<p class="meta">submitted by {{.Author}}{{if .AgentAuthor}} (AI){{end}} {{.Created}} <a class="comments" href="{{.Prefix}}detailed?q={{.Path}}">{{len .Children}} comments</a>{{if .Attachments}} <span class="comments">{{len .Attachments}} attached</span>{{end}}</p>
		</div>
	</article>
{{end}}

{{define "taskTree"}}
	<article class="comment {{.DepthClass}} {{if .Checked}}is-checked{{end}}">
		<details open>
			<summary>{{if .Name}}{{.Name}}{{else}}reply by {{.Author}}{{end}}</summary>
			<div class="body">
					{{if .Track}}<form class="toggle-form" action="{{.Prefix}}toggle" method="post">
					<input type="hidden" name="csrf" value="{{.CSRF}}">
					<input type="hidden" name="path" value="{{.Path}}">
					<input type="hidden" name="next" value="{{.Next}}">
					<button type="submit">{{if .Checked}}done{{else}}open{{end}}</button>
					</form>{{end}}
					<div class="md">
						{{if .Name}}<h2>{{.Name}}</h2>{{end}}
						{{if .Text}}<p>{{.Text}}</p>{{end}}
						<p class="meta">{{.Author}}{{if .AgentAuthor}} (AI){{end}} · {{.Created}} · {{.ForumName}} · node {{.ID}}</p>
				</div>
				{{if .Attachments}}
				<ul class="attachments">
					{{range .Attachments}}
						<li><a href="{{.URL}}">{{.Name}}</a> <span class="meta">v{{.Version}} · {{.Ref}} · {{.Size}} · <a href="{{$.Prefix}}documentHistory?q={{$.Path}}&doc={{.ID}}">history</a></span></li>
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
						<input type="hidden" name="csrf" value="{{.CSRF}}">
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
				<input type="hidden" name="csrf" value="{{.CSRF}}">
				{{if .Current.IsRoot}}<h2>Add post</h2>{{else}}<h2>Reply to &ldquo;{{.Current.Name}}&rdquo;</h2>{{end}}
				<input type="hidden" name="q" value="{{.Current.Path}}">
				<input type="hidden" name="forum" value="{{.ForumID}}">
				<label for="title">Title</label>
				<input id="title" name="title" type="text" autocomplete="off" {{if .Current.IsRoot}}required{{end}}>
				<p class="meta">Required for a forum post; optional for a reply.</p>
				<label for="content">Text</label>
				<textarea id="content" name="content" rows="3"></textarea>
				<label for="document">Attach</label>
				<input id="document" name="document" type="file" multiple>
				<p class="meta">Attach files to this post or reply. The combined upload limit is 100 MB.</p>
				{{if not .Current.IsRoot}}
				<label><input name="track" type="checkbox"> Track as task</label>
				<p class="meta">Adds open/done task state to this reply.</p>
				{{end}}
				{{if .Current.Documents}}
				<label for="reply-replaces">Revision of</label>
				<select id="reply-replaces" name="replaces">
					<option value="">New document</option>
					{{range .Current.Documents}}<option value="{{.ID}}">{{.Name}} v{{.Version}}</option>{{end}}
				</select>
				<p class="meta">Choose a document only when uploading exactly one file that replaces it.</p>
				{{end}}
				<button type="submit">{{if .Current.IsRoot}}Post{{else}}Reply{{end}}</button>
			</form>

			<form action="{{.Prefix}}attachDocument" method="post" enctype="multipart/form-data">
				<input type="hidden" name="csrf" value="{{.CSRF}}">
				<h2>Attach documents to &ldquo;{{.Current.Name}}&rdquo;</h2>
			<input type="hidden" name="q" value="{{.Current.Path}}">
				<label for="attach-document">Files</label>
				<input id="attach-document" name="document" type="file" multiple required>
				<label for="attach-replaces">Revision of</label>
				<select id="attach-replaces" name="replaces">
					<option value="">New document</option>
					{{range .Current.Documents}}<option value="{{.ID}}">{{.Name}} v{{.Version}}</option>{{end}}
				</select>
				<button type="submit">Attach</button>
				<p class="meta">Choose a document only when uploading exactly one file that explicitly replaces it. Filenames do not create revisions. The combined upload limit is 100 MB.</p>
			</form>

		<form action="{{.Prefix}}editWaypoint" method="post">
			<input type="hidden" name="csrf" value="{{.CSRF}}">
			<h2>Edit &ldquo;{{.Current.Name}}&rdquo;</h2>
			<input type="hidden" name="q" value="{{.Current.Path}}">
			<label for="edit-title">Title</label>
				<input id="edit-title" name="title" type="text" value="{{.Current.Name}}" autocomplete="off">
			<label for="edit-content">Notes</label>
			<textarea id="edit-content" name="content" rows="3">{{.Current.Text}}</textarea>
			<button type="submit">Update</button>
			</form>

			{{if .Current.CanDelete}}
			<form action="{{.Prefix}}moveWaypoint" method="post">
				<input type="hidden" name="csrf" value="{{.CSRF}}">
				<h2>Move or promote this node</h2>
				<input type="hidden" name="q" value="{{.Current.Path}}">
				<label for="move-forum">Forum</label>
				<select id="move-forum" name="forum">
					{{range .Forums}}<option value="{{.ID}}" {{if eq .ID $.Current.ForumID}}selected{{end}}>{{.Name}}</option>{{end}}
				</select>
				<p class="meta">Used when moving the node to the top level. Its replies move with it.</p>
				<label for="move-title">Forum post title</label>
				<input id="move-title" name="title" type="text" value="{{.Current.Name}}" autocomplete="off">
				<p class="meta">Required when promoting this node to a top-level forum post.</p>
				<label for="move-parent">Parent node id</label>
				<input id="move-parent" name="parent" type="text" autocomplete="off">
				<p class="meta">Leave blank to promote it to a forum post, or enter another node id to make it a reply there.</p>
				<button type="submit">Move</button>
			</form>

			<form action="{{.Prefix}}deleteWaypoint" method="post">
			<input type="hidden" name="csrf" value="{{.CSRF}}">
			<h2>Delete &ldquo;{{.Current.Name}}&rdquo;</h2>
			<input type="hidden" name="q" value="{{.Current.Path}}">
			<button class="danger" type="submit">Delete</button>
			</form>
			{{end}}

			{{if .Current.IsRoot}}
			<form action="{{.Prefix}}addForum" method="post">
				<input type="hidden" name="csrf" value="{{.CSRF}}">
				<h2>Create forum</h2>
				<label for="forum-name">Name</label>
				<input id="forum-name" name="name" type="text" autocomplete="off" required>
				<label for="forum-description">Description</label>
				<textarea id="forum-description" name="description" rows="2"></textarea>
				<p class="meta">Forums group related private posts and their conversations.</p>
				<button type="submit">Create forum</button>
			</form>
			{{end}}
		</section>
{{end}}
`

func trimPrefix(prefix string) string {
	return "/" + strings.Trim(prefix, "/")
}
