package e2e

import (
	"context"
	"errors"
	"strings"

	rpcclient "github.com/wandxy/hand/internal/rpc/client"
)

type rpcClientAPI interface {
	Respond(context.Context, string, rpcclient.RespondOptions) (string, error)
	CurrentSession(context.Context) (string, error)
	Close() error
}

var rpcclientNewClient = func(ctx context.Context, opts rpcclient.Options) (rpcClientAPI, error) {
	return rpcclient.NewClient(ctx, opts)
}

type RPCAdapter struct {
	harness *RPCHarness
}

func NewRPCAdapter(harness *RPCHarness) *RPCAdapter {
	return &RPCAdapter{harness: harness}
}

func (a *RPCAdapter) Send(ctx context.Context, req RootChatRequest) (RootChatResult, error) {
	if a == nil || a.harness == nil {
		return RootChatResult{}, errors.New("e2e rpc adapter is required")
	}
	if err := req.Validate(); err != nil {
		return RootChatResult{}, err
	}

	client, err := rpcclientNewClient(normalizeHarnessContext(ctx), rpcclient.Options{
		Address: a.harness.address,
		Port:    a.harness.port,
	})
	if err != nil {
		return RootChatResult{}, err
	}
	defer func() {
		_ = client.Close()
	}()

	var events []Event
	reply, err := client.Respond(normalizeHarnessContext(ctx), req.Message, rpcclient.RespondOptions{
		Instruct:  req.Instruct,
		SessionID: req.SessionID,
		Stream:    req.Stream,
		OnEvent: func(event rpcclient.Event) {
			events = append(events, Event{
				Channel: strings.TrimSpace(event.Channel),
				Text:    event.Text,
			})
		},
	})
	if err != nil {
		return RootChatResult{}, err
	}

	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		sessionID, err = client.CurrentSession(normalizeHarnessContext(ctx))
		if err != nil {
			return RootChatResult{}, err
		}
	}

	return RootChatResult{
		Reply:     reply,
		SessionID: sessionID,
		Events:    events,
	}, nil
}
