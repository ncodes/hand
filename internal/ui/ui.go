package ui

import "context"

// Event is a generic UI event.
type Event struct {
	Type    string
	Message string
}

// Surface renders UI events.
type Surface interface {
	Start(context.Context) error
	Render(context.Context, Event) error
}
