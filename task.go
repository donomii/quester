package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

const (
	defaultUserID = "personalusermode"
	rootPath      = "ca15fd43dfaeb80eb8c125735e0479b0"
)

type Task struct {
	Id        string
	Name      string
	Text      string
	TimeStamp time.Time
	Checked   bool
	Deleted   bool
	SubTasks  []*Task
}

func defaultRoot() *Task {
	return &Task{
		Id:        rootPath,
		Name:      "Quester",
		Text:      "Quest style task tracking",
		TimeStamp: time.Now().UTC(),
	}
}

func newTask(name, text string) *Task {
	return &Task{
		Id:        newTaskID(),
		Name:      cleanTitle(name),
		Text:      strings.TrimSpace(text),
		TimeStamp: time.Now().UTC(),
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
	title = strings.TrimSpace(title)
	if title == "" {
		return "Untitled task"
	}
	return title
}

func normalizeTree(root *Task) *Task {
	if root == nil {
		return defaultRoot()
	}
	if strings.TrimSpace(root.Id) == "" {
		root.Id = rootPath
	}
	if strings.TrimSpace(root.Name) == "" {
		root.Name = "Quester"
	}
	normalizeChildren(root)
	return root
}

func normalizeChildren(task *Task) {
	for _, child := range task.SubTasks {
		if child == nil {
			continue
		}
		if strings.TrimSpace(child.Id) == "" {
			child.Id = newTaskID()
		}
		if strings.TrimSpace(child.Name) == "" {
			child.Name = "Untitled task"
		}
		normalizeChildren(child)
	}
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
	if task == nil {
		return nil
	}
	path = normalizedPath(path)
	if path == rootPath {
		return task
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
	}
	return current
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
