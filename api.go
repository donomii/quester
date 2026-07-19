package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type apiAttachment struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
	Replaces  string    `json:"replaces,omitempty"`
	URL       string    `json:"url"`
}

type apiNode struct {
	ID          string          `json:"id"`
	ForumID     string          `json:"forum_id"`
	ParentID    string          `json:"parent_id,omitempty"`
	AuthorID    string          `json:"author_id"`
	Title       string          `json:"title,omitempty"`
	Body        string          `json:"body,omitempty"`
	Status      string          `json:"status,omitempty"`
	DueDate     string          `json:"due_date,omitempty"`
	Priority    string          `json:"priority"`
	Tags        []string        `json:"tags,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	Deleted     bool            `json:"deleted"`
	Attachments []apiAttachment `json:"attachments,omitempty"`
	Replies     []apiNode       `json:"replies,omitempty"`
}

type apiCreateNodeRequest struct {
	ForumID  string   `json:"forum_id"`
	ParentID string   `json:"parent_id"`
	Title    string   `json:"title"`
	Body     string   `json:"body"`
	Track    bool     `json:"track"`
	DueDate  string   `json:"due_date"`
	Priority string   `json:"priority"`
	Tags     []string `json:"tags"`
}

type apiStatusRequest struct {
	Status string `json:"status"`
}

type apiMoveRequest struct {
	ForumID  string `json:"forum_id"`
	ParentID string `json:"parent_id"`
	Title    string `json:"title"`
}

type apiForumRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type apiErrorResponse struct {
	Error string `json:"error"`
}

type apiIndexResponse struct {
	Description string   `json:"description"`
	Identity    []string `json:"identity_headers"`
	Endpoints   []string `json:"endpoints"`
}

func (a *App) registerAPI(router *gin.Engine) {
	base := a.prefix + "api/"
	router.GET(strings.TrimSuffix(base, "/"), a.authed(a.apiIndex))
	router.GET(base, a.authed(a.apiIndex))
	router.GET(base+"forums", a.authed(a.apiForums))
	router.GET(base+"users", a.authed(a.apiUsers))
	router.GET(base+"forums/:id/nodes", a.authed(a.apiForumNodes))
	router.GET(base+"nodes/:id", a.authed(a.apiGetNode))
	router.GET(base+"changes", a.authed(a.apiChanges))
	router.GET(base+"mentions/:id", a.authed(a.apiMentions))
	router.POST(base+"forums", a.apiMutating(a.apiCreateForum))
	router.POST(base+"nodes", a.apiMutating(a.apiCreateNode))
	router.POST(base+"nodes/:id/status", a.apiMutating(a.apiSetStatus))
	router.POST(base+"nodes/:id/move", a.apiMutating(a.apiMoveNode))
	router.POST(base+"nodes/:id/attachments", a.apiMutating(a.apiAttachDocument))
}

func (a *App) apiIndex(c *gin.Context, userID string) {
	c.JSON(http.StatusOK, apiIndexResponse{
		Description: "Private forum API. Nodes are posts or replies; every node can carry files and optional open/done task state.",
		Identity: []string{
			"X-Quester-User: stable author id; defaults to personalusermode",
			"X-Quester-Name: displayed author name",
			"X-Quester-Agent: true when the author is an AI user",
			"authentigate-id: optional workspace id; all contributors to one workspace must use the same value",
		},
		Endpoints: []string{
			"GET api/forums",
			"POST api/forums with JSON name and description",
			"GET api/users",
			"GET api/forums/{forum-id}/nodes",
			"GET api/nodes/{node-id}",
			"GET api/changes?since={RFC3339 timestamp}",
			"GET api/mentions/{user-id}?since={RFC3339 timestamp}",
			"POST api/nodes with JSON forum_id, parent_id, title, body, track, optional due_date (YYYY-MM-DD), priority (low, normal, high, or urgent), and tags",
			"POST api/nodes/{node-id}/status with JSON status: open, done, or none",
			"POST api/nodes/{node-id}/move with JSON forum_id, parent_id, and title",
			"POST api/nodes/{node-id}/attachments as multipart document files with optional replaces attachment id; combined limit 100 MB",
		},
	})
}

func (a *App) apiMutating(handler authedHandler) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !a.validMutationOrigin(c) {
			apiError(c, http.StatusForbidden, "cross-origin requests are not allowed")
			return
		}
		if strings.HasPrefix(c.ContentType(), "multipart/") {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxAttachmentBytes+(1<<20))
		}
		userID := strings.TrimSpace(c.GetHeader("authentigate-id"))
		if userID == "" {
			userID = defaultUserID
		}
		handler(c, userID)
	}
}

func (a *App) apiForums(c *gin.Context, userID string) {
	root, err := a.store.Load(userID)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "could not load forums")
		return
	}
	c.JSON(http.StatusOK, root.Forums)
}

func (a *App) apiUsers(c *gin.Context, userID string) {
	root, err := a.store.Load(userID)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "could not load users")
		return
	}
	c.JSON(http.StatusOK, root.Users)
}

func (a *App) apiForumNodes(c *gin.Context, userID string) {
	root, err := a.store.Load(userID)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "could not load nodes")
		return
	}
	forumID := cleanForumID(c.Param("id"))
	if findForum(root, forumID) == nil {
		apiError(c, http.StatusNotFound, fmt.Sprintf("forum %q was not found", forumID))
		return
	}
	nodes := make([]apiNode, 0)
	for _, node := range visibleChildren(root) {
		if node.ForumId == forumID {
			nodes = append(nodes, a.apiNode(node, rootPath, true))
		}
	}
	c.JSON(http.StatusOK, nodes)
}

func (a *App) apiGetNode(c *gin.Context, userID string) {
	root, err := a.store.Load(userID)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "could not load node")
		return
	}
	chain := visibleTaskChain(c.Param("id"), root)
	if len(chain) == 0 {
		apiError(c, http.StatusNotFound, fmt.Sprintf("node %q was not found", c.Param("id")))
		return
	}
	parentID := rootPath
	if len(chain) > 2 {
		parentID = chain[len(chain)-2].Id
	}
	c.JSON(http.StatusOK, a.apiNode(chain[len(chain)-1], parentID, true))
}

func (a *App) apiChanges(c *gin.Context, userID string) {
	since, err := parseSince(c.Query("since"))
	if err != nil {
		apiError(c, http.StatusBadRequest, err.Error())
		return
	}

	root, err := a.store.Load(userID)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "could not load changes")
		return
	}
	nodes := make([]apiNode, 0)
	collectChangedNodes(a, root, rootPath, since, &nodes)
	c.JSON(http.StatusOK, nodes)
}

func (a *App) apiMentions(c *gin.Context, userID string) {
	since, err := parseSince(c.Query("since"))
	if err != nil {
		apiError(c, http.StatusBadRequest, err.Error())
		return
	}
	root, err := a.store.Load(userID)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "could not load mentions")
		return
	}
	userIDToFind := cleanUserID(c.Param("id"))
	if findUser(root, userIDToFind) == nil {
		apiError(c, http.StatusNotFound, fmt.Sprintf("user %q was not found", userIDToFind))
		return
	}
	needle := "@" + strings.ToLower(userIDToFind)
	nodes := make([]apiNode, 0)
	collectMentionedNodes(a, root, rootPath, since, needle, &nodes)
	c.JSON(http.StatusOK, nodes)
}

func parseSince(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("since must be RFC3339, received %q", value)
	}
	return parsed, nil
}

func collectChangedNodes(app *App, parent *Task, parentID string, since time.Time, nodes *[]apiNode) {
	for _, node := range parent.SubTasks {
		if node == nil {
			continue
		}
		if !node.UpdatedAt.Before(since) {
			*nodes = append(*nodes, app.apiNode(node, parentID, false))
		}
		collectChangedNodes(app, node, node.Id, since, nodes)
	}
}

func collectMentionedNodes(app *App, parent *Task, parentID string, since time.Time, needle string, nodes *[]apiNode) {
	for _, node := range visibleChildren(parent) {
		content := strings.ToLower(node.Name + "\n" + node.Text)
		if !node.UpdatedAt.Before(since) && strings.Contains(content, needle) {
			*nodes = append(*nodes, app.apiNode(node, parentID, false))
		}
		collectMentionedNodes(app, node, node.Id, since, needle, nodes)
	}
}

func (a *App) apiCreateForum(c *gin.Context, userID string) {
	var request apiForumRequest
	if err := decodeJSON(c, &request); err != nil {
		apiError(c, http.StatusBadRequest, err.Error())
		return
	}
	name := strings.TrimSpace(request.Name)
	if name == "" {
		apiError(c, http.StatusBadRequest, "forum name is required")
		return
	}
	forum := &Forum{Name: name, Description: strings.TrimSpace(request.Description)}
	forum.Id = newForumID(forum.Name)
	err := a.store.Update(userID, func(root *Task) error {
		if findForum(root, forum.Id) != nil {
			forum.Id = forum.Id + "-" + newTaskID()[:8]
		}
		root.Forums = append(root.Forums, forum)
		return nil
	})
	if err != nil {
		apiError(c, http.StatusInternalServerError, "could not save forum")
		return
	}
	c.JSON(http.StatusCreated, forum)
}

func (a *App) apiCreateNode(c *gin.Context, userID string) {
	var request apiCreateNodeRequest
	if err := decodeJSON(c, &request); err != nil {
		apiError(c, http.StatusBadRequest, err.Error())
		return
	}
	actorID, actorName, actorIsAgent := requestActor(c)
	parentID := strings.TrimSpace(request.ParentID)
	forumID := cleanForumID(request.ForumID)
	if parentID == "" && strings.TrimSpace(request.Title) == "" {
		apiError(c, http.StatusBadRequest, "title is required for a forum post")
		return
	}
	track := request.Track || parentID == ""
	node := newTask(request.Title, request.Body, forumID, actorID, track)
	if parentID == "" {
		node.Name = cleanTitle(node.Name)
	}
	if err := setTaskMetadataTags(node, request.DueDate, request.Priority, request.Tags); err != nil {
		apiError(c, http.StatusBadRequest, err.Error())
		return
	}

	err := a.store.Update(userID, func(root *Task) error {
		parent := root
		if parentID != "" {
			chain := visibleTaskChain(parentID, root)
			if len(chain) == 0 {
				return errTaskNotFound
			}
			parent = chain[len(chain)-1]
			node.ForumId = parent.ForumId
		} else if findForum(root, node.ForumId) == nil {
			return errForumNotFound
		}
		ensureUser(root, actorID, actorName, actorIsAgent)
		parent.SubTasks = append(parent.SubTasks, node)
		return nil
	})
	if err != nil {
		a.apiMutationError(c, err)
		return
	}
	c.JSON(http.StatusCreated, a.apiNode(node, firstNonEmpty(parentID, rootPath), true))
}

func (a *App) apiSetStatus(c *gin.Context, userID string) {
	var request apiStatusRequest
	if err := decodeJSON(c, &request); err != nil {
		apiError(c, http.StatusBadRequest, err.Error())
		return
	}
	status := strings.ToLower(strings.TrimSpace(request.Status))
	if status != "open" && status != "done" && status != "none" {
		apiError(c, http.StatusBadRequest, fmt.Sprintf("status must be open, done, or none; received %q", request.Status))
		return
	}

	var response apiNode
	err := a.store.Update(userID, func(root *Task) error {
		chain := visibleTaskChain(c.Param("id"), root)
		if len(chain) == 0 {
			return errTaskNotFound
		}
		node := chain[len(chain)-1]
		node.Track = status != "none"
		node.Checked = status == "done"
		node.UpdatedAt = time.Now().UTC()
		parentID := rootPath
		if len(chain) > 2 {
			parentID = chain[len(chain)-2].Id
		}
		response = a.apiNode(node, parentID, true)
		return nil
	})
	if err != nil {
		a.apiMutationError(c, err)
		return
	}
	c.JSON(http.StatusOK, response)
}

func (a *App) apiMoveNode(c *gin.Context, userID string) {
	var request apiMoveRequest
	if err := decodeJSON(c, &request); err != nil {
		apiError(c, http.StatusBadRequest, err.Error())
		return
	}
	err := a.store.Update(userID, func(root *Task) error {
		return moveTask(root, c.Param("id"), request.ParentID, cleanForumID(request.ForumID), request.Title)
	})
	if err != nil {
		a.apiMutationError(c, err)
		return
	}
	root, err := a.store.Load(userID)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "node moved but could not be reloaded")
		return
	}
	chain := FindTaskChain(c.Param("id"), root)
	parentID := rootPath
	if len(chain) > 2 {
		parentID = chain[len(chain)-2].Id
	}
	c.JSON(http.StatusOK, a.apiNode(chain[len(chain)-1], parentID, true))
}

func (a *App) apiAttachDocument(c *gin.Context, userID string) {
	replaces := strings.TrimSpace(c.PostForm("replaces"))
	attachments, err := a.collectAttachments(c, replaces)
	if err != nil {
		status, message := attachmentError(err)
		apiError(c, status, message)
		return
	}
	if len(attachments) == 0 {
		apiError(c, http.StatusBadRequest, "at least one document is required")
		return
	}

	var response apiNode
	err = a.store.Update(userID, func(root *Task) error {
		chain := visibleTaskChain(c.Param("id"), root)
		if len(chain) == 0 {
			return errTaskNotFound
		}
		if replaces != "" {
			_, replaced := findAttachment(root, replaces)
			if replaced == nil {
				return errDocumentNotFound
			}
		}
		node := chain[len(chain)-1]
		node.Attachments = append(node.Attachments, attachments...)
		node.UpdatedAt = time.Now().UTC()
		parentID := rootPath
		if len(chain) > 2 {
			parentID = chain[len(chain)-2].Id
		}
		response = a.apiNode(node, parentID, true)
		return nil
	})
	if err != nil {
		a.apiMutationError(c, err)
		return
	}
	c.JSON(http.StatusCreated, response)
}

func (a *App) apiNode(node *Task, parentID string, includeReplies bool) apiNode {
	status := ""
	if node.Track && node.Checked {
		status = "done"
	} else if node.Track {
		status = "open"
	}
	response := apiNode{
		ID:        node.Id,
		ForumID:   node.ForumId,
		ParentID:  parentID,
		AuthorID:  node.AuthorId,
		Title:     node.Name,
		Body:      node.Text,
		Status:    status,
		DueDate:   node.DueDate,
		Priority:  normalizePriority(node.Priority),
		Tags:      append([]string(nil), node.Tags...),
		CreatedAt: node.TimeStamp,
		UpdatedAt: node.UpdatedAt,
		Deleted:   node.Deleted,
	}
	for _, attachment := range node.Attachments {
		if attachment == nil {
			continue
		}
		response.Attachments = append(response.Attachments, apiAttachment{
			ID:        attachment.Id,
			Name:      attachment.Name,
			Size:      attachment.Size,
			CreatedAt: attachment.TimeStamp,
			Replaces:  attachment.Replaces,
			URL:       a.prefix + "document?q=" + node.Id + "&doc=" + attachment.Id,
		})
	}
	if includeReplies {
		for _, reply := range visibleChildren(node) {
			response.Replies = append(response.Replies, a.apiNode(reply, node.Id, true))
		}
	}
	return response
}

func (a *App) apiMutationError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, errTaskNotFound):
		apiError(c, http.StatusNotFound, "node was not found")
	case errors.Is(err, errForumNotFound):
		apiError(c, http.StatusNotFound, "forum was not found")
	case errors.Is(err, errDocumentNotFound):
		apiError(c, http.StatusBadRequest, "the document being replaced was not found")
	case errors.Is(err, errInvalidMove):
		apiError(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, errTitleRequired):
		apiError(c, http.StatusBadRequest, err.Error())
	default:
		apiError(c, http.StatusInternalServerError, "could not save changes: "+err.Error())
	}
}

func decodeJSON[T apiCreateNodeRequest | apiStatusRequest | apiMoveRequest | apiForumRequest](c *gin.Context, destination *T) error {
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("request body is not valid JSON: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain exactly one JSON object")
	}
	return nil
}

func apiError(c *gin.Context, status int, message string) {
	c.JSON(status, apiErrorResponse{Error: message})
}
