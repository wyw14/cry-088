package project

import (
	"fmt"
	"strings"

	"github.com/wyw14/cry-088/internal/domain/shared"
)

type TaskStatus string

const (
	TaskTodo       TaskStatus = "todo"
	TaskInProgress TaskStatus = "in_progress"
	TaskBlocked    TaskStatus = "blocked"
	TaskDone       TaskStatus = "done"
)

type Task struct {
	ID               string
	ProjectID        string
	ParentID         string
	Name             string
	OwnerID          string
	EstimatedMinutes int
	ActualMinutes    int
	Status           TaskStatus
	DependencyIDs    []string
	Version          int64
}

func NewTask(id, projectID, name, ownerID string, estimate int) (*Task, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(projectID) == "" || strings.TrimSpace(name) == "" {
		return nil, shared.New(shared.CodeInvalid, "task identity is incomplete")
	}
	if estimate <= 0 {
		return nil, shared.Field(shared.CodeInvalid, "task estimate must be positive", "estimated_minutes", "must be positive")
	}
	return &Task{ID: id, ProjectID: projectID, Name: strings.TrimSpace(name), OwnerID: ownerID, EstimatedMinutes: estimate, Status: TaskTodo, Version: 1}, nil
}

func (t *Task) AddDependency(dependencyID string) error {
	if dependencyID == "" || dependencyID == t.ID {
		return shared.New(shared.CodeInvalid, "task dependency is invalid")
	}
	for _, existing := range t.DependencyIDs {
		if existing == dependencyID {
			return shared.New(shared.CodeConflict, "task dependency already exists")
		}
	}
	t.DependencyIDs = append(t.DependencyIDs, dependencyID)
	t.Version++
	return nil
}

func (t *Task) Start(dependencies map[string]TaskStatus, expectedVersion int64) error {
	if t.Version != expectedVersion {
		return shared.New(shared.CodeConflict, "task was changed")
	}
	if t.Status != TaskTodo && t.Status != TaskBlocked {
		return shared.New(shared.CodeIllegalState, "task cannot be started")
	}
	for _, id := range t.DependencyIDs {
		if dependencies[id] != TaskDone {
			t.Status = TaskBlocked
			t.Version++
			return shared.New(shared.CodeIllegalState, fmt.Sprintf("dependency %s is not completed", id))
		}
	}
	t.Status = TaskInProgress
	t.Version++
	return nil
}

func ValidateDependencyGraph(tasks []Task) error {
	graph := make(map[string][]string, len(tasks))
	for _, task := range tasks {
		graph[task.ID] = append([]string(nil), task.DependencyIDs...)
	}
	visiting := map[string]bool{}
	visited := map[string]bool{}
	var visit func(string) error
	visit = func(id string) error {
		if visiting[id] {
			return shared.New(shared.CodeInvalid, "task dependency cycle detected")
		}
		if visited[id] {
			return nil
		}
		if _, exists := graph[id]; !exists {
			return shared.New(shared.CodeNotFound, "task dependency does not exist")
		}
		visiting[id] = true
		for _, dependency := range graph[id] {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		visiting[id] = false
		visited[id] = true
		return nil
	}
	for id := range graph {
		if err := visit(id); err != nil {
			return err
		}
	}
	return nil
}
