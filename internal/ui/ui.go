package ui

import "context"

type Event struct {
	Type    string
	Message string
}

type Surface interface {
	Start(context.Context) error
	Render(context.Context, Event) error
}
