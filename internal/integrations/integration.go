package integrations

import "context"

type Message struct {
	Source  string
	Sender  string
	Content string
}

type Adapter interface {
	Name() string
	Start(context.Context) error
	Stop(context.Context) error
}

type MessageHandler interface {
	HandleMessage(context.Context, Message) error
}
