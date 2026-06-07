package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"

	"github.com/wandxy/hand/internal/config"
	gatewaysession "github.com/wandxy/hand/internal/gateway/session"
	storage "github.com/wandxy/hand/internal/state/core"
	agentcore "github.com/wandxy/hand/pkg/agent"
	gatewaytelegram "github.com/wandxy/hand/pkg/gateway/telegram"
	gatewaytypes "github.com/wandxy/hand/pkg/gateway/types"
	"github.com/wandxy/hand/pkg/nanoid"
)

var genericCreatedSessionID = nanoid.MustFromSeed(
	storage.SessionIDPrefix,
	"telegram-created",
	"telegram-created-test",
)

type genericResponderStub struct {
	message        string
	options        agentcore.RespondOptions
	binding        storage.GatewayBinding
	savedBinding   storage.GatewayBinding
	createdSession storage.Session
	reply          string
	err            error
	contextErr     error
	getBindingErr  error
	saveBindingErr error
	createErr      error
	bindingFound   bool
	called         bool
	created        bool
}

func (s *genericResponderStub) Respond(
	ctx context.Context,
	message string,
	opts agentcore.RespondOptions,
) (string, error) {
	s.called = true
	s.message = message
	s.options = opts
	s.contextErr = ctx.Err()
	if opts.OnEvent != nil {
		opts.OnEvent(agentcore.Event{Kind: agentcore.EventKindTextDelta, Channel: "reasoning", Text: "ignored"})
		opts.OnEvent(agentcore.Event{Kind: agentcore.EventKindTextDelta, Channel: "assistant", Text: "stream "})
		opts.OnEvent(agentcore.Event{Kind: agentcore.EventKindTrace})
		opts.OnEvent(agentcore.Event{Kind: agentcore.EventKindTextDelta, Channel: "assistant", Text: "delta"})
	}
	return s.reply, s.err
}

func (s *genericResponderStub) CreateSession(context.Context, string) (storage.Session, error) {
	s.created = true
	if s.createErr != nil {
		return storage.Session{}, s.createErr
	}
	if s.createdSession.ID == "" {
		s.createdSession = storage.Session{ID: genericCreatedSessionID}
	}

	return s.createdSession, nil
}

func (s *genericResponderStub) SaveGatewayBinding(
	_ context.Context,
	binding storage.GatewayBinding,
) error {
	s.savedBinding = binding
	return s.saveBindingErr
}

func (s *genericResponderStub) GetGatewayBinding(
	context.Context,
	string,
) (storage.GatewayBinding, bool, error) {
	return s.binding, s.bindingFound, s.getBindingErr
}

type telegramAPICall struct {
	method    string
	target    gatewaytelegram.Target
	messageID int64
	draftID   int64
	text      string
	offset    int64
}

type fakeTelegramAPI struct {
	mu          sync.Mutex
	calls       []telegramAPICall
	updates     [][]gatewaytelegram.Update
	getErr      error
	getErrs     []error
	sendErr     error
	sendErrs    []error
	editErr     error
	editErrs    []error
	draftErr    error
	draftErrs   []error
	nextMessage int64
	onGet       func(int64)
	onCall      func(telegramAPICall)
}

func (a *fakeTelegramAPI) GetUpdates(
	_ context.Context,
	offset int64,
) ([]gatewaytelegram.Update, error) {
	a.mu.Lock()
	a.calls = append(a.calls, telegramAPICall{method: "getUpdates", offset: offset})
	index := len(a.callsOfMethodLocked("getUpdates")) - 1
	if a.onGet != nil {
		a.onGet(offset)
	}
	if err := a.nextErrorLocked("getUpdates"); err != nil {
		a.mu.Unlock()
		return nil, err
	}
	if index >= len(a.updates) {
		a.mu.Unlock()
		return nil, nil
	}
	updates := append([]gatewaytelegram.Update(nil), a.updates[index]...)
	a.mu.Unlock()
	return updates, nil
}

func (a *fakeTelegramAPI) SendMessage(
	_ context.Context,
	target gatewaytelegram.Target,
	text string,
) (int64, error) {
	return a.recordMessageCall("sendMessage", target, 0, 0, text, a.nextError("sendMessage"))
}

func (a *fakeTelegramAPI) EditMessageText(
	_ context.Context,
	target gatewaytelegram.Target,
	messageID int64,
	text string,
) error {
	_, err := a.recordMessageCall("editMessageText", target, messageID, 0, text, a.nextError("editMessageText"))
	return err
}

func (a *fakeTelegramAPI) SendMessageDraft(
	_ context.Context,
	target gatewaytelegram.Target,
	draftID int64,
	text string,
) error {
	_, err := a.recordMessageCall("sendMessageDraft", target, 0, draftID, text, a.nextError("sendMessageDraft"))
	return err
}

func (a *fakeTelegramAPI) nextError(method string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.nextErrorLocked(method)
}

func (a *fakeTelegramAPI) nextErrorLocked(method string) error {
	switch method {
	case "getUpdates":
		if len(a.getErrs) > 0 {
			err := a.getErrs[0]
			a.getErrs = a.getErrs[1:]
			return err
		}
		return a.getErr
	case "sendMessage":
		if len(a.sendErrs) > 0 {
			err := a.sendErrs[0]
			a.sendErrs = a.sendErrs[1:]
			return err
		}
		return a.sendErr
	case "editMessageText":
		if len(a.editErrs) > 0 {
			err := a.editErrs[0]
			a.editErrs = a.editErrs[1:]
			return err
		}
		return a.editErr
	case "sendMessageDraft":
		if len(a.draftErrs) > 0 {
			err := a.draftErrs[0]
			a.draftErrs = a.draftErrs[1:]
			return err
		}
		return a.draftErr
	default:
		return nil
	}
}

func (a *fakeTelegramAPI) recordMessageCall(
	method string,
	target gatewaytelegram.Target,
	messageID int64,
	draftID int64,
	text string,
	err error,
) (int64, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	call := telegramAPICall{method: method, target: target, messageID: messageID, draftID: draftID, text: text}
	a.calls = append(a.calls, call)
	if a.onCall != nil {
		a.onCall(call)
	}
	if err != nil {
		return 0, err
	}
	a.nextMessage++
	return a.nextMessage, nil
}

func (a *fakeTelegramAPI) callsOfMethod(method string) []telegramAPICall {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]telegramAPICall(nil), a.callsOfMethodLocked(method)...)
}

func (a *fakeTelegramAPI) callsOfMethodLocked(method string) []telegramAPICall {
	var calls []telegramAPICall
	for _, call := range a.calls {
		if call.method == method {
			calls = append(calls, call)
		}
	}

	return calls
}

func (a *fakeTelegramAPI) allCalls() []telegramAPICall {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]telegramAPICall(nil), a.calls...)
}

func newWebhookHandler(cfg config.GatewayConfig, service gatewaysession.Service) http.Handler {
	return newWebhookHandlerWithDispatchContext(context.Background(), cfg, service)
}

func newWebhookHandlerWithDispatchContext(
	dispatchCtx context.Context,
	cfg config.GatewayConfig,
	service gatewaysession.Service,
) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(WebhookPath, HandleWebhook(dispatchCtx, cfg.Telegram, service))
	return mux
}

func decodeGatewayResponse(
	t requireTestingT,
	recorder *httptest.ResponseRecorder,
) gatewaytypes.RespondResponse {
	var response gatewaytypes.RespondResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return response
}

type requireTestingT interface {
	Fatalf(format string, args ...any)
}

var errTelegramTest = errors.New("telegram test error")
