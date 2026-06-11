package main

import (
	_ "embed"
	"html/template"
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
	ID         string
	Prefix     string
	Name       string
	Text       string
	Path       string
	Next       string
	Created    string
	DepthClass string
	Checked    bool
	CanDelete  bool
	Children   []*TaskNode
}

func buildTaskNode(task *Task, path, prefix, next string, depth int) *TaskNode {
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
	for _, child := range visibleChildren(task) {
		node.Children = append(node.Children, buildTaskNode(child, joinTaskPath(path, child.Id), prefix, next, depth+1))
	}
	return node
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
	{{template "taskTree" .Current}}
	{{template "taskForms" .}}
{{template "footer" .}}
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
			<p class="meta">submitted {{.Created}} <a class="comments" href="{{.Prefix}}detailed?q={{.Path}}">{{len .Children}} comments</a></p>
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
		<form action="{{.Prefix}}addWaypoint" method="post">
			<h2>Add task</h2>
			<input type="hidden" name="q" value="{{.Current.Path}}">
			<label for="title">Title</label>
			<input id="title" name="title" type="text" autocomplete="off" required>
			<label for="content">Notes</label>
			<textarea id="content" name="content" rows="3"></textarea>
			<button type="submit">Add</button>
		</form>

		<form action="{{.Prefix}}editWaypoint" method="post">
			<h2>Edit current task</h2>
			<input type="hidden" name="q" value="{{.Current.Path}}">
			<label for="edit-title">Title</label>
			<input id="edit-title" name="title" type="text" value="{{.Current.Name}}" autocomplete="off" required>
			<label for="edit-content">Notes</label>
			<textarea id="edit-content" name="content" rows="3">{{.Current.Text}}</textarea>
			<button type="submit">Update</button>
		</form>

		{{if .Current.CanDelete}}
		<form action="{{.Prefix}}deleteWaypoint" method="post">
			<h2>Delete current task</h2>
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
