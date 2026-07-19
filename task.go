package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path"
	"strings"
	"time"
)

const (
	defaultUserID  = "personalusermode"
	defaultForumID = "general"
	currentSchema  = 1
	rootPath       = "ca15fd43dfaeb80eb8c125735e0479b0"
)

type Forum struct {
	Id          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type User struct {
	Id      string `json:"id"`
	Name    string `json:"name"`
	IsAgent bool   `json:"is_agent"`
}

// Node is the common record used for a forum post, reply, or task. Its
// position and optional task status determine how it is presented.
type Node struct {
	Id          string
	Name        string
	Text        string
	TimeStamp   time.Time
	UpdatedAt   time.Time
	ForumId     string
	AuthorId    string
	Track       bool
	Checked     bool
	Deleted     bool
	Attachments []*Attachment
	SubTasks    []*Node
	Schema      int      `json:",omitempty"`
	Forums      []*Forum `json:",omitempty"`
	Users       []*User  `json:",omitempty"`
}

type Task = Node

// Attachment is an immutable record of a file attached to a node. Replaces
// explicitly links a revision to the attachment it supersedes.
type Attachment struct {
	Id        string
	Name      string
	Blob      string
	Size      int64
	TimeStamp time.Time
	Replaces  string
}

func newAttachment(name, blob string, size int64, replaces string) *Attachment {
	return &Attachment{
		Id:        newTaskID(),
		Name:      cleanFileName(name),
		Blob:      blob,
		Size:      size,
		TimeStamp: time.Now().UTC(),
		Replaces:  strings.TrimSpace(replaces),
	}
}

func defaultRoot() *Task {
	now := time.Now().UTC()
	return &Task{
		Id:        rootPath,
		Name:      "Quester",
		Text:      "Quest style task tracking",
		TimeStamp: now,
		UpdatedAt: now,
		Schema:    currentSchema,
		Forums:    []*Forum{defaultForum()},
		Users:     []*User{defaultUser()},
	}
}

func defaultForum() *Forum {
	return &Forum{Id: defaultForumID, Name: "General", Description: "General posts, conversations, and tasks."}
}

func defaultUser() *User {
	return &User{Id: defaultUserID, Name: "Personal user"}
}

func newTask(name, text, forumID, authorID string, track bool) *Task {
	now := time.Now().UTC()
	return &Task{
		Id:        newTaskID(),
		Name:      cleanOptionalTitle(name),
		Text:      strings.TrimSpace(text),
		TimeStamp: now,
		UpdatedAt: now,
		ForumId:   cleanForumID(forumID),
		AuthorId:  cleanUserID(authorID),
		Track:     track,
	}
}

func newTaskID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err == nil {
		return hex.EncodeToString(b[:])
	}
	return fmt.Sprintf("%d", time.Now().UTC().UnixNano())
}

func cleanTitle(title string) string {
	title = cleanOptionalTitle(title)
	if title == "" {
		return "Untitled task"
	}
	return title
}

func cleanOptionalTitle(title string) string {
	return strings.TrimSpace(title)
}

func cleanForumID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return defaultForumID
	}
	return id
}

func cleanUserID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return defaultUserID
	}
	return id
}

func cleanForumName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "Untitled forum"
	}
	return name
}

func newForumID(name string) string {
	var id strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			id.WriteRune(r)
		case id.Len() > 0 && !strings.HasSuffix(id.String(), "-"):
			id.WriteByte('-')
		}
	}
	cleaned := strings.Trim(id.String(), "-")
	if cleaned == "" {
		return "forum-" + newTaskID()[:8]
	}
	return cleaned
}

// cleanFileName reduces an uploaded name to a bare file name.
func cleanFileName(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	name = strings.TrimSpace(path.Base(name))
	if name == "" || name == "." || name == ".." || name == "/" {
		return "unnamed-file"
	}
	return name
}

func normalizeTree(root *Task) *Task {
	if root == nil {
		return defaultRoot()
	}
	legacy := root.Schema == 0
	if strings.TrimSpace(root.Id) == "" {
		root.Id = rootPath
	}
	if strings.TrimSpace(root.Name) == "" {
		root.Name = "Quester"
	}
	root.Schema = currentSchema
	normalizeForums(root)
	normalizeUsers(root)
	normalizeAttachments(root)
	normalizeChildren(root, defaultForumID, legacy)
	return root
}

func normalizeForums(root *Task) {
	forums := root.Forums[:0]
	seen := map[string]bool{}
	for _, forum := range root.Forums {
		if forum == nil {
			continue
		}
		forum.Id = cleanForumID(forum.Id)
		forum.Name = cleanForumName(forum.Name)
		forum.Description = strings.TrimSpace(forum.Description)
		if seen[forum.Id] {
			continue
		}
		seen[forum.Id] = true
		forums = append(forums, forum)
	}
	if !seen[defaultForumID] {
		forums = append([]*Forum{defaultForum()}, forums...)
	}
	root.Forums = forums
}

func normalizeUsers(root *Task) {
	users := root.Users[:0]
	seen := map[string]bool{}
	for _, user := range root.Users {
		if user == nil {
			continue
		}
		user.Id = cleanUserID(user.Id)
		user.Name = cleanTitle(user.Name)
		if seen[user.Id] {
			continue
		}
		seen[user.Id] = true
		users = append(users, user)
	}
	if !seen[defaultUserID] {
		users = append([]*User{defaultUser()}, users...)
	}
	root.Users = users
}

func normalizeChildren(task *Task, inheritedForum string, legacy bool) {
	for _, child := range task.SubTasks {
		if child == nil {
			continue
		}
		if strings.TrimSpace(child.Id) == "" {
			child.Id = newTaskID()
		}
		if legacy {
			child.Name = cleanTitle(child.Name)
		} else {
			child.Name = cleanOptionalTitle(child.Name)
		}
		child.ForumId = cleanForumID(firstNonEmpty(child.ForumId, inheritedForum))
		child.AuthorId = cleanUserID(child.AuthorId)
		if child.UpdatedAt.IsZero() {
			child.UpdatedAt = child.TimeStamp
		}
		if legacy {
			child.Track = true
		}
		normalizeAttachments(child)
		normalizeChildren(child, child.ForumId, legacy)
	}
}

func firstNonEmpty(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func normalizeAttachments(task *Task) {
	attachments := task.Attachments[:0]
	for _, attachment := range task.Attachments {
		if attachment == nil {
			continue
		}
		if strings.TrimSpace(attachment.Id) == "" {
			attachment.Id = newTaskID()
		}
		attachment.Name = cleanFileName(attachment.Name)
		attachments = append(attachments, attachment)
	}
	task.Attachments = attachments
}

func normalizedPath(path string) string {
	path = strings.Trim(path, "/")
	if path == "" {
		return rootPath
	}
	return path
}

func isRootPath(path string) bool {
	return normalizedPath(path) == rootPath
}

func joinTaskPath(parentPath, taskID string) string {
	parentPath = normalizedPath(parentPath)
	if parentPath == rootPath {
		return rootPath + "/" + taskID
	}
	return parentPath + "/" + taskID
}

func parentPath(path string) string {
	path = normalizedPath(path)
	if path == rootPath {
		return rootPath
	}
	index := strings.LastIndex(path, "/")
	if index < 0 {
		return rootPath
	}
	parent := strings.Trim(path[:index], "/")
	if parent == "" {
		return rootPath
	}
	return parent
}

func FindTask(path string, task *Task) *Task {
	chain := FindTaskChain(path, task)
	if len(chain) == 0 {
		return nil
	}
	return chain[len(chain)-1]
}

// FindTaskChain returns every task from the root to the task at path,
// inclusive, or nil if the path does not resolve.
func FindTaskChain(path string, task *Task) []*Task {
	if task == nil {
		return nil
	}
	path = normalizedPath(path)
	chain := []*Task{task}
	if path == rootPath {
		return chain
	}
	if !strings.Contains(path, "/") {
		return findTaskChainByID(path, task, chain)
	}

	parts := strings.Split(path, "/")
	if len(parts) > 0 && parts[0] == rootPath {
		parts = parts[1:]
	}

	current := task
	for _, part := range parts {
		if part == "" {
			continue
		}
		var next *Task
		for _, child := range current.SubTasks {
			if child != nil && child.Id == part {
				next = child
				break
			}
		}
		if next == nil {
			return nil
		}
		current = next
		chain = append(chain, current)
	}
	return chain
}

func findTaskChainByID(id string, task *Task, chain []*Task) []*Task {
	for _, child := range task.SubTasks {
		if child == nil {
			continue
		}
		childChain := append(slicesClone(chain), child)
		if child.Id == id {
			return childChain
		}
		if found := findTaskChainByID(id, child, childChain); len(found) > 0 {
			return found
		}
	}
	return nil
}

func slicesClone(tasks []*Task) []*Task {
	return append([]*Task(nil), tasks...)
}

func findParent(root, target *Task) *Task {
	if root == nil || target == nil || root == target {
		return nil
	}
	for _, child := range root.SubTasks {
		if child == target {
			return root
		}
		if parent := findParent(child, target); parent != nil {
			return parent
		}
	}
	return nil
}

func containsTask(root *Task, id string) bool {
	return FindTask(id, root) != nil
}

func visibleTaskChain(path string, root *Task) []*Task {
	chain := FindTaskChain(path, root)
	for _, task := range chain {
		if task == nil || task.Deleted {
			return nil
		}
	}
	return chain
}

func removeChild(parent, target *Task) bool {
	if parent == nil || target == nil {
		return false
	}
	for i, child := range parent.SubTasks {
		if child == target {
			parent.SubTasks = append(parent.SubTasks[:i], parent.SubTasks[i+1:]...)
			return true
		}
	}
	return false
}

func findAttachment(root *Task, id string) (*Task, *Attachment) {
	if root == nil || strings.TrimSpace(id) == "" {
		return nil, nil
	}
	for _, attachment := range root.Attachments {
		if attachment != nil && attachment.Id == id {
			return root, attachment
		}
	}
	for _, child := range root.SubTasks {
		if task, attachment := findAttachment(child, id); attachment != nil {
			return task, attachment
		}
	}
	return nil, nil
}

func ensureUser(root *Task, id, name string, isAgent bool) *User {
	id = cleanUserID(id)
	if user := findUser(root, id); user != nil {
		if strings.TrimSpace(name) != "" {
			user.Name = strings.TrimSpace(name)
		}
		if isAgent {
			user.IsAgent = true
		}
		return user
	}
	if strings.TrimSpace(name) == "" {
		name = id
	}
	user := &User{Id: id, Name: strings.TrimSpace(name), IsAgent: isAgent}
	root.Users = append(root.Users, user)
	return user
}

func moveTask(root *Task, id, newParentID, forumID, title string) error {
	node := FindTask(id, root)
	if node == nil || node == root || node.Deleted {
		return errTaskNotFound
	}
	oldParent := findParent(root, node)
	if oldParent == nil {
		return errTaskNotFound
	}

	newParent := root
	if strings.TrimSpace(newParentID) != "" && !isRootPath(newParentID) {
		chain := visibleTaskChain(newParentID, root)
		if len(chain) == 0 {
			return errTaskNotFound
		}
		newParent = chain[len(chain)-1]
		if node == newParent || containsTask(node, newParent.Id) {
			return errInvalidMove
		}
		forumID = newParent.ForumId
	} else if findForum(root, forumID) == nil {
		return errForumNotFound
	} else if strings.TrimSpace(title) == "" {
		return errTitleRequired
	} else {
		node.Name = strings.TrimSpace(title)
		node.Track = true
	}

	if !removeChild(oldParent, node) {
		return errTaskNotFound
	}
	setForum(node, forumID)
	node.UpdatedAt = time.Now().UTC()
	newParent.SubTasks = append(newParent.SubTasks, node)
	return nil
}

func setForum(task *Task, forumID string) {
	if task == nil {
		return
	}
	task.ForumId = cleanForumID(forumID)
	for _, child := range task.SubTasks {
		setForum(child, task.ForumId)
	}
}

func findForum(root *Task, id string) *Forum {
	id = cleanForumID(id)
	for _, forum := range root.Forums {
		if forum != nil && forum.Id == id {
			return forum
		}
	}
	return nil
}

func findUser(root *Task, id string) *User {
	id = cleanUserID(id)
	for _, user := range root.Users {
		if user != nil && user.Id == id {
			return user
		}
	}
	return nil
}

func visibleChildren(task *Task) []*Task {
	if task == nil {
		return nil
	}
	children := make([]*Task, 0, len(task.SubTasks))
	for _, child := range task.SubTasks {
		if child == nil || child.Deleted {
			continue
		}
		children = append(children, child)
	}
	return children
}
