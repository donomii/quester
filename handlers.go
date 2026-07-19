package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	maxRestoreBytes    = 10 << 20
	maxAttachmentBytes = 100 << 20
)

var (
	errTaskNotFound     = errors.New("task not found")
	errCannotDeleteRoot = errors.New("cannot delete the root task")
	errForumNotFound    = errors.New("forum not found")
	errInvalidMove      = errors.New("node cannot be moved beneath itself or one of its replies")
	errDocumentNotFound = errors.New("document not found")
	errTitleRequired    = errors.New("a title is required for a forum post")
)

type App struct {
	store     *Store
	base      string
	prefix    string
	templates *template.Template
}

func NewApp(store *Store, prefix string) *App {
	base, normalized := normalizeRoutePrefix(prefix)
	return &App{
		store:     store,
		base:      base,
		prefix:    normalized,
		templates: newTemplates(),
	}
}

func normalizeRoutePrefix(prefix string) (base string, normalized string) {
	prefix = trimPrefix(prefix)
	if prefix == "/" {
		return "", "/"
	}
	return prefix, prefix + "/"
}

func (a *App) Register(router *gin.Engine) {
	router.MaxMultipartMemory = maxRestoreBytes + (1 << 20)
	if a.base == "" {
		router.GET("/", a.redirectHome)
	} else {
		router.GET(a.base, a.redirectHome)
	}
	if a.prefix != "/" {
		router.GET(a.prefix, a.redirectHome)
	}
	router.GET(a.prefix+"summary", a.authed(a.summary))
	router.GET(a.prefix+"downloadAll", a.authed(a.downloadAll))
	router.GET(a.prefix+"restoreAllPage", a.authed(a.restoreAllDisplay))
	router.POST(a.prefix+"restoreAll", a.mutating(a.restoreAll))
	router.GET(a.prefix+"detailed", a.authed(a.detailed))
	router.GET(a.prefix+"document", a.authed(a.document))
	router.GET(a.prefix+"documentHistory", a.authed(a.documentHistory))
	router.POST(a.prefix+"attachDocument", a.mutating(a.attachDocument))
	router.POST(a.prefix+"addForum", a.mutating(a.addForum))
	router.POST(a.prefix+"addWaypoint", a.mutating(a.addWaypoint))
	router.POST(a.prefix+"deleteWaypoint", a.mutating(a.deleteWaypoint))
	router.POST(a.prefix+"editWaypoint", a.mutating(a.editWaypoint))
	router.POST(a.prefix+"moveWaypoint", a.mutating(a.moveWaypoint))
	router.POST(a.prefix+"toggle", a.mutating(a.toggle))
	a.registerAPI(router)
}

type authedHandler func(*gin.Context, string)

func (a *App) authed(handler authedHandler) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.Request.Header.Get("authentigate-id")
		if strings.TrimSpace(userID) == "" {
			userID = defaultUserID
		}
		handler(c, userID)
	}
}

func requestActor(c *gin.Context) (id, name string, isAgent bool) {
	id = cleanUserID(c.GetHeader("X-Quester-User"))
	name = strings.TrimSpace(c.GetHeader("X-Quester-Name"))
	isAgent = strings.EqualFold(strings.TrimSpace(c.GetHeader("X-Quester-Agent")), "true")
	return id, name, isAgent
}

func (a *App) mutating(handler authedHandler) gin.HandlerFunc {
	authed := a.authed(handler)
	return func(c *gin.Context) {
		if !a.validMutationOrigin(c) {
			a.renderError(c, http.StatusForbidden, "Cross-origin form submissions are not allowed.")
			return
		}
		if strings.HasPrefix(c.ContentType(), "multipart/") {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxAttachmentBytes+(1<<20))
		}
		authed(c)
	}
}

func (a *App) validMutationOrigin(c *gin.Context) bool {
	origin := strings.TrimSpace(c.GetHeader("Origin"))
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" {
		return false
	}
	return (parsed.Scheme == "http" || parsed.Scheme == "https") && strings.EqualFold(parsed.Host, c.Request.Host)
}

func (a *App) redirectHome(c *gin.Context) {
	c.Redirect(http.StatusFound, a.prefix+"summary")
}

func (a *App) summary(c *gin.Context, userID string) {
	root, err := a.store.Load(userID)
	if err != nil {
		a.renderError(c, http.StatusInternalServerError, "Could not load tasks.")
		return
	}

	forumID := cleanForumID(c.Query("forum"))
	forum := findForum(root, forumID)
	if forum == nil {
		a.renderError(c, http.StatusNotFound, "Forum not found.")
		return
	}
	filter := c.Query("filter")
	next := c.Request.URL.RequestURI()
	current := buildTaskNode(root, rootPath, a.prefix, next, 0)
	rootTrail := newDocTrail(root)
	rootTrail.add(root, rootPath, a.prefix)
	summary := make([]*TaskNode, 0, len(root.SubTasks))
	for _, child := range visibleChildren(root) {
		if child.ForumId != forumID {
			continue
		}
		if filter == "new" && child.Checked {
			continue
		}
		summary = append(summary, buildTaskNodeWithTrail(child, joinTaskPath(rootPath, child.Id), a.prefix, next, 0, rootTrail.clone()))
	}

	a.render(c, http.StatusOK, "summary", PageData{
		Title:   forum.Name + " - Unfinished Business",
		Filter:  filter,
		Current: current,
		Summary: summary,
		Forums:  buildForumNodes(root, forumID, a.prefix),
		ForumID: forumID,
	})
}

func (a *App) detailed(c *gin.Context, userID string) {
	path := normalizedPath(c.Query("q"))
	root, err := a.store.Load(userID)
	if err != nil {
		a.renderError(c, http.StatusInternalServerError, "Could not load tasks.")
		return
	}

	chain := visibleTaskChain(path, root)
	if len(chain) == 0 {
		a.renderError(c, http.StatusNotFound, "Task not found.")
		return
	}

	title := chain[len(chain)-1].Name
	if strings.TrimSpace(title) == "" {
		title = "Reply"
	}
	a.render(c, http.StatusOK, "detail", PageData{
		Title:   title + " - Unfinished Business",
		Current: buildDetailNode(chain, a.prefix, c.Request.URL.RequestURI()),
		Forums:  buildForumNodes(root, chain[len(chain)-1].ForumId, a.prefix),
		ForumID: chain[len(chain)-1].ForumId,
	})
}

func (a *App) documentHistory(c *gin.Context, userID string) {
	path := normalizedPath(c.Query("q"))
	docID := strings.TrimSpace(c.Query("doc"))
	if docID == "" {
		a.renderError(c, http.StatusBadRequest, "A document id is required.")
		return
	}

	root, err := a.store.Load(userID)
	if err != nil {
		a.renderError(c, http.StatusInternalServerError, "Could not load tasks.")
		return
	}
	chain := visibleTaskChain(path, root)
	if len(chain) == 0 {
		a.renderError(c, http.StatusNotFound, "Task not found.")
		return
	}

	history := buildDocumentHistory(root, docID, a.prefix)
	if history == nil {
		a.renderError(c, http.StatusNotFound, "Document not found.")
		return
	}
	a.render(c, http.StatusOK, "history", PageData{
		Title:   history.Name + " history - Unfinished Business",
		History: history,
		Forums:  buildForumNodes(root, chain[len(chain)-1].ForumId, a.prefix),
		ForumID: chain[len(chain)-1].ForumId,
	})
}

func (a *App) document(c *gin.Context, userID string) {
	path := normalizedPath(c.Query("q"))
	docID := strings.TrimSpace(c.Query("doc"))

	root, err := a.store.Load(userID)
	if err != nil {
		a.renderError(c, http.StatusInternalServerError, "Could not load tasks.")
		return
	}
	chain := visibleTaskChain(path, root)
	if len(chain) == 0 {
		a.renderError(c, http.StatusNotFound, "Task not found.")
		return
	}
	task := chain[len(chain)-1]

	var attachment *Attachment
	for _, candidate := range task.Attachments {
		if candidate != nil && candidate.Id == docID {
			attachment = candidate
			break
		}
	}
	if attachment == nil {
		a.renderError(c, http.StatusNotFound, "Document not found.")
		return
	}

	file, info, err := a.store.OpenBlob(attachment.Blob)
	if err != nil {
		a.renderError(c, http.StatusNotFound, "Document content is missing.")
		return
	}
	defer file.Close()

	contentType, disposition := documentContentType(attachment.Name)
	c.Header("Content-Type", contentType)
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": attachment.Name}))
	c.Header("Cache-Control", "private, max-age=31536000, immutable")
	http.ServeContent(c.Writer, c.Request, "", info.ModTime(), file)
}

// extraDocTypes covers extensions the Go mime table misses; consulted first
// so the result does not depend on the host's /etc mime configuration.
var extraDocTypes = map[string]string{
	".md":       "text/markdown; charset=utf-8",
	".markdown": "text/markdown; charset=utf-8",
}

// documentContentType picks the served type and whether the browser may
// render it inline. Anything that could script on our origin (HTML, SVG,
// unknown types) is forced to download.
func documentContentType(name string) (contentType, disposition string) {
	ext := strings.ToLower(filepath.Ext(name))
	contentType = extraDocTypes[ext]
	if contentType == "" {
		contentType = mime.TypeByExtension(ext)
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return "application/octet-stream", "attachment"
	}
	switch {
	case mediaType == "application/pdf",
		mediaType == "text/plain",
		mediaType == "text/markdown":
		return contentType, "inline"
	case mediaType == "image/svg+xml":
		return contentType, "attachment"
	case strings.HasPrefix(mediaType, "image/"),
		strings.HasPrefix(mediaType, "video/"),
		strings.HasPrefix(mediaType, "audio/"):
		return contentType, "inline"
	}
	return contentType, "attachment"
}

// collectAttachments stores every "document" upload in the blob store and
// returns records to hang on a task. Orphan blobs from a failed mutation are
// harmless: content-addressed and reused on the next upload.
func (a *App) collectAttachments(c *gin.Context, replaces string) ([]*Attachment, error) {
	if !strings.HasPrefix(c.ContentType(), "multipart/") {
		return nil, nil
	}
	form, err := c.MultipartForm()
	if err != nil {
		return nil, err
	}
	var attachments []*Attachment
	var totalSize int64
	for _, header := range form.File["document"] {
		totalSize += header.Size
	}
	if totalSize > maxAttachmentBytes {
		return nil, attachmentSizeError{received: totalSize}
	}
	if replaces != "" && len(form.File["document"]) != 1 {
		return nil, errors.New("a document revision must contain exactly one file")
	}
	for _, header := range form.File["document"] {
		if header.Filename == "" {
			continue
		}
		file, err := header.Open()
		if err != nil {
			return nil, err
		}
		ref, size, err := a.store.SaveBlob(file)
		file.Close()
		if err != nil {
			return nil, err
		}
		attachments = append(attachments, newAttachment(header.Filename, ref, size, replaces))
	}
	return attachments, nil
}

type attachmentSizeError struct {
	received int64
}

func (e attachmentSizeError) Error() string {
	return fmt.Sprintf("attachments total %d bytes; maximum is %d bytes", e.received, maxAttachmentBytes)
}

func attachmentError(err error) (status int, message string) {
	var maxBytesError *http.MaxBytesError
	var sizeError attachmentSizeError
	switch {
	case errors.As(err, &maxBytesError):
		return http.StatusRequestEntityTooLarge, fmt.Sprintf("Attachments exceed the %d byte upload limit.", maxAttachmentBytes)
	case errors.As(err, &sizeError):
		return http.StatusRequestEntityTooLarge, fmt.Sprintf("Attachments total %d bytes; the maximum is %d bytes.", sizeError.received, maxAttachmentBytes)
	default:
		return http.StatusBadRequest, "Could not read the attached documents: " + err.Error()
	}
}

func (a *App) attachDocument(c *gin.Context, userID string) {
	path := normalizedPath(c.PostForm("q"))
	replaces := strings.TrimSpace(c.PostForm("replaces"))
	attachments, err := a.collectAttachments(c, replaces)
	if err != nil {
		status, message := attachmentError(err)
		a.renderError(c, status, message)
		return
	}
	if len(attachments) == 0 {
		a.renderError(c, http.StatusBadRequest, "Choose at least one document to attach.")
		return
	}

	err = a.store.Update(userID, func(root *Task) error {
		chain := visibleTaskChain(path, root)
		if len(chain) == 0 {
			return errTaskNotFound
		}
		task := chain[len(chain)-1]
		if replaces != "" {
			_, replaced := findAttachment(root, replaces)
			if replaced == nil {
				return errDocumentNotFound
			}
		}
		task.Attachments = append(task.Attachments, attachments...)
		task.UpdatedAt = time.Now().UTC()
		return nil
	})
	if err != nil {
		a.handleMutationError(c, err)
		return
	}
	c.Redirect(http.StatusSeeOther, a.detailURL(path))
}

func (a *App) downloadAll(c *gin.Context, userID string) {
	root, err := a.store.Load(userID)
	if err != nil {
		a.renderError(c, http.StatusInternalServerError, "Could not load tasks.")
		return
	}

	payload, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		a.renderError(c, http.StatusInternalServerError, "Could not encode tasks.")
		return
	}

	c.Header("Content-Disposition", `attachment; filename="tasks.json"`)
	c.Data(http.StatusOK, "application/json; charset=utf-8", payload)
}

func (a *App) restoreAllDisplay(c *gin.Context, userID string) {
	root, err := a.store.Load(userID)
	if err != nil {
		a.renderError(c, http.StatusInternalServerError, "Could not load tasks.")
		return
	}
	a.render(c, http.StatusOK, "restore", PageData{
		Title:   "Restore - Unfinished Business",
		Current: buildTaskNode(root, rootPath, a.prefix, c.Request.URL.RequestURI(), 0),
		Forums:  buildForumNodes(root, "", a.prefix),
	})
}

func (a *App) restoreAll(c *gin.Context, userID string) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxRestoreBytes+(1<<20))

	fileHeader, err := c.FormFile("content")
	if err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			a.renderError(c, http.StatusRequestEntityTooLarge, "Backup file is too large.")
			return
		}
		a.renderError(c, http.StatusBadRequest, "Choose a backup JSON file to restore.")
		return
	}
	if fileHeader.Size > maxRestoreBytes {
		a.renderError(c, http.StatusRequestEntityTooLarge, "Backup file is too large.")
		return
	}

	file, err := fileHeader.Open()
	if err != nil {
		a.renderError(c, http.StatusBadRequest, "Could not open the backup file.")
		return
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxRestoreBytes+1))
	if err != nil {
		a.renderError(c, http.StatusBadRequest, "Could not read the backup file.")
		return
	}
	if len(data) > maxRestoreBytes {
		a.renderError(c, http.StatusRequestEntityTooLarge, "Backup file is too large.")
		return
	}

	if err := a.store.Restore(userID, data); err != nil {
		a.renderError(c, http.StatusBadRequest, err.Error())
		return
	}
	c.Redirect(http.StatusSeeOther, a.prefix+"summary")
}

func (a *App) addWaypoint(c *gin.Context, userID string) {
	parent := normalizedPath(c.PostForm("q"))
	if parent == rootPath && strings.TrimSpace(c.PostForm("title")) == "" {
		a.renderError(c, http.StatusBadRequest, "A title is required for a forum post.")
		return
	}
	forumID := cleanForumID(c.PostForm("forum"))
	authorID, authorName, authorIsAgent := requestActor(c)
	track := parent == rootPath || c.PostForm("track") == "on"
	task := newTask(c.PostForm("title"), c.PostForm("content"), forumID, authorID, track)
	if parent == rootPath {
		task.Name = cleanTitle(task.Name)
	}

	replaces := strings.TrimSpace(c.PostForm("replaces"))
	attachments, err := a.collectAttachments(c, replaces)
	if err != nil {
		status, message := attachmentError(err)
		a.renderError(c, status, message)
		return
	}
	task.Attachments = attachments

	err = a.store.Update(userID, func(root *Task) error {
		chain := visibleTaskChain(parent, root)
		if len(chain) == 0 {
			return errTaskNotFound
		}
		parentTask := chain[len(chain)-1]
		if parentTask != root {
			task.ForumId = parentTask.ForumId
		} else if findForum(root, task.ForumId) == nil {
			return errForumNotFound
		}
		if replaces != "" {
			_, replaced := findAttachment(root, replaces)
			if replaced == nil {
				return errDocumentNotFound
			}
		}
		ensureUser(root, authorID, authorName, authorIsAgent)
		parentTask.SubTasks = append(parentTask.SubTasks, task)
		return nil
	})
	if err != nil {
		a.handleMutationError(c, err)
		return
	}
	c.Redirect(http.StatusSeeOther, a.detailURL(parent))
}

func (a *App) addForum(c *gin.Context, userID string) {
	name := strings.TrimSpace(c.PostForm("name"))
	if name == "" {
		a.renderError(c, http.StatusBadRequest, "A forum name is required.")
		return
	}
	description := strings.TrimSpace(c.PostForm("description"))
	forumID := newForumID(name)

	err := a.store.Update(userID, func(root *Task) error {
		candidate := forumID
		if findForum(root, candidate) != nil {
			candidate = candidate + "-" + newTaskID()[:8]
		}
		forumID = candidate
		root.Forums = append(root.Forums, &Forum{Id: forumID, Name: name, Description: description})
		return nil
	})
	if err != nil {
		a.handleMutationError(c, err)
		return
	}
	c.Redirect(http.StatusSeeOther, a.prefix+"summary?forum="+url.QueryEscape(forumID))
}

func (a *App) moveWaypoint(c *gin.Context, userID string) {
	id := normalizedPath(c.PostForm("q"))
	newParentID := strings.TrimSpace(c.PostForm("parent"))
	forumID := cleanForumID(c.PostForm("forum"))
	title := strings.TrimSpace(c.PostForm("title"))

	err := a.store.Update(userID, func(root *Task) error { return moveTask(root, id, newParentID, forumID, title) })
	if err != nil {
		a.handleMutationError(c, err)
		return
	}
	c.Redirect(http.StatusSeeOther, a.detailURL(id))
}

func (a *App) deleteWaypoint(c *gin.Context, userID string) {
	path := normalizedPath(c.PostForm("q"))
	if isRootPath(path) {
		a.handleMutationError(c, errCannotDeleteRoot)
		return
	}
	parent := rootPath

	err := a.store.Update(userID, func(root *Task) error {
		chain := visibleTaskChain(path, root)
		if len(chain) == 0 {
			return errTaskNotFound
		}
		task := chain[len(chain)-1]
		if len(chain) > 1 {
			parent = chain[len(chain)-2].Id
		}
		task.Deleted = true
		task.UpdatedAt = time.Now().UTC()
		return nil
	})
	if err != nil {
		a.handleMutationError(c, err)
		return
	}
	c.Redirect(http.StatusSeeOther, a.detailURL(parent))
}

func (a *App) editWaypoint(c *gin.Context, userID string) {
	path := normalizedPath(c.PostForm("q"))
	title := c.PostForm("title")
	content := c.PostForm("content")

	err := a.store.Update(userID, func(root *Task) error {
		chain := visibleTaskChain(path, root)
		if len(chain) == 0 {
			return errTaskNotFound
		}
		task := chain[len(chain)-1]
		if len(chain) == 2 {
			task.Name = cleanTitle(title)
		} else {
			task.Name = cleanOptionalTitle(title)
		}
		task.Text = strings.TrimSpace(content)
		task.UpdatedAt = time.Now().UTC()
		return nil
	})
	if err != nil {
		a.handleMutationError(c, err)
		return
	}
	c.Redirect(http.StatusSeeOther, a.detailURL(path))
}

func (a *App) toggle(c *gin.Context, userID string) {
	path := normalizedPath(c.PostForm("path"))

	err := a.store.Update(userID, func(root *Task) error {
		chain := visibleTaskChain(path, root)
		if len(chain) == 0 {
			return errTaskNotFound
		}
		task := chain[len(chain)-1]
		task.Track = true
		task.Checked = !task.Checked
		task.UpdatedAt = time.Now().UTC()
		return nil
	})
	if err != nil {
		a.handleMutationError(c, err)
		return
	}
	a.redirectBack(c, a.detailURL(path))
}

func (a *App) handleMutationError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, errTaskNotFound):
		a.renderError(c, http.StatusNotFound, "Task not found.")
	case errors.Is(err, errCannotDeleteRoot):
		a.renderError(c, http.StatusBadRequest, "The root task cannot be deleted.")
	case errors.Is(err, errForumNotFound):
		a.renderError(c, http.StatusNotFound, "Forum not found.")
	case errors.Is(err, errInvalidMove):
		a.renderError(c, http.StatusBadRequest, err.Error()+".")
	case errors.Is(err, errDocumentNotFound):
		a.renderError(c, http.StatusBadRequest, "The document being replaced was not found.")
	case errors.Is(err, errTitleRequired):
		a.renderError(c, http.StatusBadRequest, "A title is required when promoting a reply to a forum post.")
	default:
		logMutationFailed(err)
		a.renderError(c, http.StatusInternalServerError, "Could not save tasks.")
	}
}

func (a *App) redirectBack(c *gin.Context, fallback string) {
	next := c.PostForm("next")
	if a.safeLocalPath(next) {
		c.Redirect(http.StatusSeeOther, next)
		return
	}
	c.Redirect(http.StatusSeeOther, fallback)
}

func (a *App) safeLocalPath(path string) bool {
	if path == "" || strings.Contains(path, "\n") || strings.Contains(path, "\r") {
		return false
	}
	if strings.HasPrefix(path, "//") {
		return false
	}
	return strings.HasPrefix(path, a.prefix)
}

func (a *App) detailURL(path string) string {
	return a.prefix + "detailed?q=" + url.QueryEscape(normalizedPath(path))
}

func (a *App) render(c *gin.Context, status int, templateName string, data PageData) {
	data.Style = template.CSS(styleCSS)
	data.Prefix = a.prefix
	data.RootPath = rootPath
	if data.Title == "" {
		data.Title = "Unfinished Business"
	}
	if data.CurrentURL == "" {
		data.CurrentURL = c.Request.URL.RequestURI()
	}
	if data.Current == nil {
		data.Current = buildTaskNode(defaultRoot(), rootPath, a.prefix, data.CurrentURL, 0)
	}

	c.Status(status)
	c.Header("Content-Type", "text/html; charset=utf-8")
	if err := a.templates.ExecuteTemplate(c.Writer, templateName, data); err != nil {
		logRenderFailed(templateName, err)
	}
}

func (a *App) renderError(c *gin.Context, status int, message string) {
	a.render(c, status, "error", PageData{
		Title: fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Error: message,
	})
}
