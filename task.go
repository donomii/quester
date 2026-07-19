package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"
)

const (
	defaultUserID   = "personalusermode"
	defaultForumID  = "general"
	currentSchema   = 2
	rootPath        = "ca15fd43dfaeb80eb8c125735e0479b0"
	defaultPriority = "normal"
	maxTags         = 20
	maxTagLength    = 40
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
	DueDate     string   `json:",omitempty"`
	Priority    string   `json:",omitempty"`
	Tags        []string `json:",omitempty"`
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
		Priority:  defaultPriority,
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
		child.DueDate = strings.TrimSpace(child.DueDate)
		child.Priority = normalizePriority(child.Priority)
		child.Tags = normalizeTags(child.Tags)
		normalizeAttachments(child)
		normalizeChildren(child, child.ForumId, legacy)
	}
}

func normalizePriority(priority string) string {
	priority = strings.ToLower(strings.TrimSpace(priority))
	if priority == "" {
		return defaultPriority
	}
	return priority
}

func validPriority(priority string) bool {
	switch priority {
	case "low", defaultPriority, "high", "urgent":
		return true
	default:
		return false
	}
}

func normalizeTags(tags []string) []string {
	seen := map[string]bool{}
	normalized := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		key := strings.ToLower(tag)
		if tag == "" || seen[key] {
			continue
		}
		seen[key] = true
		normalized = append(normalized, tag)
	}
	return normalized
}

func parseTaskMetadata(dueDate, priority, tagList string) (string, string, []string, error) {
	return parseTaskMetadataTags(dueDate, priority, strings.Split(tagList, ","))
}

func parseTaskMetadataTags(dueDate, priority string, inputTags []string) (string, string, []string, error) {
	dueDate = strings.TrimSpace(dueDate)
	if dueDate != "" {
		parsed, err := time.Parse("2006-01-02", dueDate)
		if err != nil || parsed.Format("2006-01-02") != dueDate {
			return "", "", nil, fmt.Errorf("due date must use YYYY-MM-DD, received %q", dueDate)
		}
	}
	priority = normalizePriority(priority)
	if !validPriority(priority) {
		return "", "", nil, fmt.Errorf("priority must be low, normal, high, or urgent, received %q", priority)
	}
	for _, tag := range inputTags {
		if strings.Contains(tag, ",") {
			return "", "", nil, fmt.Errorf("tag %q contains a comma", tag)
		}
	}
	tags := normalizeTags(inputTags)
	if len(tags) > maxTags {
		return "", "", nil, fmt.Errorf("tags contain %d entries; the maximum is %d", len(tags), maxTags)
	}
	for _, tag := range tags {
		if len([]rune(tag)) > maxTagLength {
			return "", "", nil, fmt.Errorf("tag %q is longer than %d characters", tag, maxTagLength)
		}
	}
	return dueDate, priority, tags, nil
}

func setTaskMetadata(task *Task, dueDate, priority, tagList string) error {
	dueDate, priority, tags, err := parseTaskMetadata(dueDate, priority, tagList)
	if err != nil {
		return err
	}
	task.DueDate = dueDate
	task.Priority = priority
	task.Tags = tags
	return nil
}

func setTaskMetadataTags(task *Task, dueDate, priority string, inputTags []string) error {
	dueDate, priority, tags, err := parseTaskMetadataTags(dueDate, priority, inputTags)
	if err != nil {
		return err
	}
	task.DueDate = dueDate
	task.Priority = priority
	task.Tags = tags
	return nil
}

func validateTaskTree(task *Task) error {
	if task == nil {
		return errors.New("task tree is null")
	}
	if task.Id != rootPath {
		if !validPriority(task.Priority) {
			return fmt.Errorf("task %q has invalid priority %q", task.Id, task.Priority)
		}
		if task.DueDate != "" {
			parsed, err := time.Parse("2006-01-02", task.DueDate)
			if err != nil || parsed.Format("2006-01-02") != task.DueDate {
				return fmt.Errorf("task %q has invalid due date %q", task.Id, task.DueDate)
			}
		}
		if len(task.Tags) > maxTags {
			return fmt.Errorf("task %q has %d tags; the maximum is %d", task.Id, len(task.Tags), maxTags)
		}
		for _, tag := range task.Tags {
			if strings.TrimSpace(tag) == "" || len([]rune(tag)) > maxTagLength {
				return fmt.Errorf("task %q has invalid tag %q", task.Id, tag)
			}
		}
	}
	for _, attachment := range task.Attachments {
		if attachment == nil || !isBlobRef(attachment.Blob) || attachment.Size < 0 {
			return fmt.Errorf("task %q has invalid attachment content metadata", task.Id)
		}
	}
	for _, child := range task.SubTasks {
		if err := validateTaskTree(child); err != nil {
			return err
		}
	}
	return nil
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

// collectBlobRefs records every blob referenced by any attachment in the
// tree, including attachments on soft-deleted nodes: those records survive in
// the JSON, so their content must survive with them.
func collectBlobRefs(task *Task, refs map[string]bool) {
	if task == nil {
		return
	}
	for _, attachment := range task.Attachments {
		if attachment != nil && isBlobRef(attachment.Blob) {
			refs[attachment.Blob] = true
		}
	}
	for _, child := range task.SubTasks {
		collectBlobRefs(child, refs)
	}
}

// taskMatches reports whether the lower-cased needle occurs in the task's
// title, notes, or tags.
func taskMatches(task *Task, loweredNeedle string) bool {
	return strings.Contains(strings.ToLower(task.Name+"\n"+task.Text+"\n"+strings.Join(task.Tags, "\n")), loweredNeedle)
}

func applyBulkTaskAction(root *Task, ids []string, action string) error {
	if len(ids) == 0 {
		return errNoTasksSelected
	}
	tasks := make([]*Task, 0, len(ids))
	seen := map[string]bool{}
	for _, id := range ids {
		chain := visibleTaskChain(strings.TrimSpace(id), root)
		if len(chain) == 0 || chain[len(chain)-1] == root {
			return errTaskNotFound
		}
		task := chain[len(chain)-1]
		if !seen[task.Id] {
			seen[task.Id] = true
			tasks = append(tasks, task)
		}
	}
	now := time.Now().UTC()
	for _, task := range tasks {
		switch action {
		case "check":
			task.Track = true
			task.Checked = true
		case "uncheck":
			task.Track = true
			task.Checked = false
		case "delete":
			task.Deleted = true
		default:
			return errInvalidBulkAction
		}
		task.UpdatedAt = now
	}
	return nil
}

func selectedTaskExport(root *Task, ids []string) (*Task, error) {
	if len(ids) == 0 {
		return nil, errNoTasksSelected
	}
	selected := map[string]bool{}
	chains := make([][]*Task, 0, len(ids))
	for _, id := range ids {
		chain := visibleTaskChain(strings.TrimSpace(id), root)
		if len(chain) == 0 || chain[len(chain)-1] == root {
			return nil, errTaskNotFound
		}
		selected[chain[len(chain)-1].Id] = true
		chains = append(chains, chain)
	}
	exported := defaultRoot()
	exported.Name = "Quester selected tasks"
	exported.Text = "Selected task export"
	exported.Forums = append([]*Forum(nil), root.Forums...)
	exported.Users = append([]*User(nil), root.Users...)
	added := map[string]bool{}
	for _, chain := range chains {
		task := chain[len(chain)-1]
		if added[task.Id] || selectedAncestor(chain, selected) {
			continue
		}
		exported.SubTasks = append(exported.SubTasks, cloneTaskTree(task))
		added[task.Id] = true
	}
	return exported, nil
}

func selectedAncestor(chain []*Task, selected map[string]bool) bool {
	for _, ancestor := range chain[1 : len(chain)-1] {
		if selected[ancestor.Id] {
			return true
		}
	}
	return false
}

func cloneTaskTree(task *Task) *Task {
	cloned := *task
	cloned.Attachments = append([]*Attachment(nil), task.Attachments...)
	cloned.SubTasks = make([]*Node, 0, len(task.SubTasks))
	for _, child := range task.SubTasks {
		if child != nil {
			cloned.SubTasks = append(cloned.SubTasks, cloneTaskTree(child))
		}
	}
	return &cloned
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

func normalizeTaskSort(value string) string {
	switch value {
	case "completion", "title":
		return value
	default:
		return "created"
	}
}

func sortTasks(tasks []*Task, sortBy string) {
	sort.SliceStable(tasks, func(i, j int) bool {
		left, right := tasks[i], tasks[j]
		switch sortBy {
		case "completion":
			if left.Checked != right.Checked {
				return !left.Checked
			}
		case "title":
			leftTitle := strings.ToLower(left.Name)
			rightTitle := strings.ToLower(right.Name)
			if leftTitle != rightTitle {
				return leftTitle < rightTitle
			}
		}
		return left.TimeStamp.After(right.TimeStamp)
	})
}
