package types

import "github.com/wandxy/hand/internal/guardrails"

type Runtime interface {
	FilePolicy() guardrails.FilesystemPolicy
	CommandPolicy() guardrails.CommandPolicy
	ListTodos() []TodoItem
	ReplaceTodos([]TodoItem) []TodoItem
	ClearTodos() []TodoItem
}

type TodoItem struct {
	Text string `json:"text"`
	Done bool   `json:"done"`
}
