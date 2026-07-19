package browser

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/wandxy/morph/internal/permissions"
)

const testPageURL = "https://93.184.216.34/page"

type approverFunc func(context.Context, permissions.EvaluationInput) error

func (f approverFunc) Authorize(ctx context.Context, input permissions.EvaluationInput) error {
	return f(ctx, input)
}

type interactiveBackend struct {
	session *interactiveBackendSession
}

func (b *interactiveBackend) Start(context.Context, LaunchOptions) (BackendSession, error) {
	return b.session, nil
}

type interactiveBackendSession struct {
	mu             sync.Mutex
	tabs           map[string]BackendTab
	active         string
	nextID         int
	closed         bool
	clicks         []int64
	typed          []string
	pressed        []string
	scrolled       [][2]int64
	selected       []string
	waits          []WaitCondition
	blockWait      bool
	snapshots      map[string]BackendSnapshot
	authorize      NetworkRequestAuthorizer
	authorizedTabs []string
	requestTarget  string
	failAction     Action
}

func (s *interactiveBackendSession) getFailure(action Action) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failAction == action {
		return errors.New("backend action failed")
	}
	return nil
}

func newInteractiveBackendSession() *interactiveBackendSession {
	return &interactiveBackendSession{
		tabs: map[string]BackendTab{
			"tab-1": {ID: "tab-1", Title: "Page", URL: testPageURL, Active: true},
		},
		active: "tab-1",
		snapshots: map[string]BackendSnapshot{
			"tab-1": {
				URL: testPageURL, Title: "Page",
				Nodes: []BackendSnapshotNode{
					{BackendNodeID: 41, Role: "textbox", Name: "Name"},
					{BackendNodeID: 42, Role: "button", Name: "Submit"},
					{Role: "paragraph", Name: "Read only"},
					{
						BackendNodeID: 43, Role: "textbox", Name: "Password", Value: "secret", Sensitive: true,
						Properties: map[string]string{"value": "secret"},
					},
				},
			},
		},
	}
}

func (s *interactiveBackendSession) Health(context.Context) error { return nil }

func (s *interactiveBackendSession) Close(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

func (s *interactiveBackendSession) ListTabs(context.Context) ([]BackendTab, error) {
	if err := s.getFailure(ActionTabs); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]BackendTab, 0, len(s.tabs))
	for _, tab := range s.tabs {
		tab.Active = tab.ID == s.active
		result = append(result, tab)
	}
	return result, nil
}

func (s *interactiveBackendSession) OpenTab(_ context.Context, rawURL string) (BackendTab, error) {
	if err := s.getFailure(ActionOpen); err != nil {
		return BackendTab{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	id := "opened-tab"
	tab := BackendTab{ID: id, URL: rawURL, Title: "Opened", Active: true}
	s.tabs[id] = tab
	s.active = id
	return tab, nil
}

func (s *interactiveBackendSession) FocusTab(_ context.Context, tabID string) error {
	if err := s.getFailure(ActionFocus); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.tabs[tabID]; !ok {
		return errors.New("tab not found")
	}
	s.active = tabID
	return nil
}

func (s *interactiveBackendSession) CloseTab(_ context.Context, tabID string) error {
	if err := s.getFailure(ActionClose); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tabs, tabID)
	if s.active == tabID {
		s.active = ""
	}
	return nil
}

func (s *interactiveBackendSession) Navigate(_ context.Context, tabID, rawURL string) (BackendTab, error) {
	return s.navigate(ActionNavigate, tabID, rawURL)
}

func (s *interactiveBackendSession) navigate(action Action, tabID, rawURL string) (BackendTab, error) {
	if err := s.getFailure(action); err != nil {
		return BackendTab{}, err
	}
	s.mu.Lock()
	authorize := s.authorize
	requestTarget := s.requestTarget
	s.mu.Unlock()
	if requestTarget == "" {
		requestTarget = rawURL
	}
	if authorize != nil {
		target, err := permissions.NetworkTargetFromURL(
			requestTarget, "GET", permissions.NetworkRequestNavigation,
		)
		if err != nil {
			return BackendTab{}, err
		}
		if err := authorize(context.Background(), target); err != nil {
			return BackendTab{}, err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tab := s.tabs[tabID]
	tab.URL = rawURL
	tab.Title = "Navigated"
	s.tabs[tabID] = tab
	return tab, nil
}

func (s *interactiveBackendSession) SetNetworkAuthorizer(tabID string, authorize NetworkRequestAuthorizer) func() {
	s.mu.Lock()
	previous := s.authorize
	s.authorize = authorize
	s.authorizedTabs = append(s.authorizedTabs, tabID)
	s.mu.Unlock()
	return func() {
		s.mu.Lock()
		s.authorize = previous
		s.mu.Unlock()
	}
}

func (s *interactiveBackendSession) Back(ctx context.Context, tabID string) (BackendTab, error) {
	return s.navigate(ActionBack, tabID, testPageURL+"/back")
}

func (s *interactiveBackendSession) Forward(ctx context.Context, tabID string) (BackendTab, error) {
	return s.navigate(ActionForward, tabID, testPageURL+"/forward")
}

func (s *interactiveBackendSession) Reload(ctx context.Context, tabID string) (BackendTab, error) {
	return s.navigate(ActionReload, tabID, testPageURL+"/reload")
}

func (s *interactiveBackendSession) Snapshot(_ context.Context, tabID string) (BackendSnapshot, error) {
	if err := s.getFailure(ActionSnapshot); err != nil {
		return BackendSnapshot{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshots[tabID], nil
}

func (s *interactiveBackendSession) Click(_ context.Context, _ string, nodeID int64) error {
	if err := s.getFailure(ActionClick); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clicks = append(s.clicks, nodeID)
	return nil
}

func (s *interactiveBackendSession) Type(_ context.Context, _ string, nodeID int64, text string, _ bool) error {
	if err := s.getFailure(ActionType); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clicks = append(s.clicks, nodeID)
	s.typed = append(s.typed, text)
	return nil
}

func (s *interactiveBackendSession) Press(_ context.Context, _ string, key string) error {
	if err := s.getFailure(ActionPress); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pressed = append(s.pressed, key)
	return nil
}

func (s *interactiveBackendSession) Scroll(_ context.Context, _ string, x, y int64) error {
	if err := s.getFailure(ActionScroll); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scrolled = append(s.scrolled, [2]int64{x, y})
	return nil
}

func (s *interactiveBackendSession) Select(_ context.Context, _ string, nodeID int64, value string) error {
	if err := s.getFailure(ActionSelect); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clicks = append(s.clicks, nodeID)
	s.selected = append(s.selected, value)
	return nil
}

func (s *interactiveBackendSession) Wait(
	ctx context.Context,
	_ string,
	condition WaitCondition,
	_ string,
	_ int64,
) error {
	if err := s.getFailure(ActionWait); err != nil {
		return err
	}
	s.mu.Lock()
	s.waits = append(s.waits, condition)
	block := s.blockWait
	s.mu.Unlock()
	if block {
		<-ctx.Done()
		return ctx.Err()
	}
	return nil
}

func TestService_ActionsMaintainOwnedTabsAndRejectStaleReferences(t *testing.T) {
	backendSession := newInteractiveBackendSession()
	service, err := NewService(
		context.Background(), testBrowserConfig(t), allowChecker(),
		&interactiveBackend{session: backendSession},
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close(context.Background())) })
	ctx := testBrowserContext("owner", "session")
	session, err := service.Start(ctx, StartRequest{})
	require.NoError(t, err)

	tabs, err := service.Tabs(ctx, session.ID)
	require.NoError(t, err)
	require.Equal(t, []Tab{{
		ID: "tab-1", SessionID: session.ID, Title: "Page", URL: testPageURL, Active: true, Generation: 1,
	}}, tabs)

	snapshot, err := service.Snapshot(ctx, ActionRequest{SessionID: session.ID, TabID: "tab-1"})
	require.NoError(t, err)
	require.Equal(t, uint64(2), snapshot.Generation)
	require.Regexp(t, `^r[0-9a-f]{24}g2e1$`, snapshot.Nodes[0].Ref)
	require.Regexp(t, `^r[0-9a-f]{24}g2e2$`, snapshot.Nodes[1].Ref)
	require.Empty(t, snapshot.Nodes[2].Ref)
	require.Regexp(t, `^r[0-9a-f]{24}g2e3$`, snapshot.Nodes[3].Ref)
	require.Empty(t, snapshot.Nodes[3].Value)
	require.Nil(t, snapshot.Nodes[3].Properties)
	typeOperations, err := service.ResolveOperations(ctx, ActionType, ActionRequest{
		SessionID: session.ID, TabID: "tab-1", Ref: snapshot.Nodes[3].Ref,
	})
	require.NoError(t, err)
	require.Contains(t, typeOperations[0].Effects, permissions.EffectCredentialBearing)

	clicked, err := service.Click(ctx, ActionRequest{
		SessionID: session.ID, TabID: "tab-1", Ref: snapshot.Nodes[1].Ref,
	})
	require.NoError(t, err)
	require.Equal(t, uint64(3), clicked.Generation)
	require.Equal(t, []int64{42}, backendSession.clicks)

	_, err = service.Click(ctx, ActionRequest{
		SessionID: session.ID, TabID: "tab-1", Ref: snapshot.Nodes[1].Ref,
	})
	browserErr, ok := GetError(err)
	require.True(t, ok)
	require.Equal(t, ErrorStaleReference, browserErr.Code)
	require.Equal(t, []int64{42}, backendSession.clicks)

	other := testBrowserContext("other", "session")
	_, err = service.Tabs(other, session.ID)
	require.EqualError(t, err, "browser session belongs to another owner")
}

func TestService_SnapshotRefsCannotCrossTabs(t *testing.T) {
	backendSession := newInteractiveBackendSession()
	backendSession.tabs["tab-2"] = BackendTab{ID: "tab-2", Title: "Other", URL: testPageURL + "/other"}
	backendSession.snapshots["tab-2"] = backendSession.snapshots["tab-1"]
	service, err := NewService(
		context.Background(), testBrowserConfig(t), allowChecker(),
		&interactiveBackend{session: backendSession},
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close(context.Background())) })
	ctx := testBrowserContext("owner", "session")
	session, err := service.Start(ctx, StartRequest{})
	require.NoError(t, err)
	_, err = service.Tabs(ctx, session.ID)
	require.NoError(t, err)
	first, err := service.Snapshot(ctx, ActionRequest{SessionID: session.ID, TabID: "tab-1"})
	require.NoError(t, err)
	second, err := service.Snapshot(ctx, ActionRequest{SessionID: session.ID, TabID: "tab-2"})
	require.NoError(t, err)
	require.NotEqual(t, first.Nodes[0].Ref, second.Nodes[0].Ref)

	_, err = service.Click(ctx, ActionRequest{
		SessionID: session.ID, TabID: "tab-2", Ref: first.Nodes[0].Ref,
	})
	browserErr, ok := GetError(err)
	require.True(t, ok)
	require.Equal(t, ErrorStaleReference, browserErr.Code)
}

func TestService_ResolveOperationsUsesAuthoritativeTabAndNavigationTargets(t *testing.T) {
	backendSession := newInteractiveBackendSession()
	service, err := NewService(
		context.Background(), testBrowserConfig(t), allowChecker(),
		&interactiveBackend{session: backendSession},
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close(context.Background())) })
	ctx := testBrowserContext("owner", "session")
	session, err := service.Start(ctx, StartRequest{})
	require.NoError(t, err)

	operations, err := service.ResolveOperations(ctx, ActionNavigate, ActionRequest{
		SessionID: session.ID, TabID: "tab-1", URL: "https://93.184.216.35/next",
	})
	require.NoError(t, err)
	require.Len(t, operations, 2)
	require.Equal(t, permissions.ResourceBrowser, operations[0].Resource)
	require.Contains(t, operations[0].Target, "93.184.216.34")
	require.Equal(t, permissions.ResourceNetwork, operations[1].Resource)
	require.Equal(t, "93.184.216.35", operations[1].Network.Host)

	operations, err = service.ResolveOperations(ctx, ActionBack, ActionRequest{
		SessionID: session.ID, TabID: "tab-1", URL: "https://93.184.216.99/untrusted",
	})
	require.NoError(t, err)
	require.Len(t, operations, 2)
	require.Equal(t, "93.184.216.34", operations[1].Network.Host)

	_, err = service.ResolveOperations(ctx, ActionNavigate, ActionRequest{
		SessionID: session.ID, TabID: "tab-1",
	})
	require.EqualError(t, err, "browser navigation URL is required")
}

func TestService_ResolveOperationsClassifiesEveryPhaseTwoAction(t *testing.T) {
	backendSession := newInteractiveBackendSession()
	service, err := NewService(
		context.Background(), testBrowserConfig(t), allowChecker(),
		&interactiveBackend{session: backendSession},
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close(context.Background())) })
	ctx := testBrowserContext("owner", "session")
	session, err := service.Start(ctx, StartRequest{})
	require.NoError(t, err)
	snapshot, err := service.Snapshot(ctx, ActionRequest{SessionID: session.ID, TabID: "tab-1"})
	require.NoError(t, err)

	for _, action := range []Action{
		ActionStatus, ActionProfiles, ActionStart, ActionStop, ActionTabs, ActionOpen, ActionFocus, ActionClose,
		ActionNavigate, ActionReload, ActionSnapshot, ActionClick, ActionType, ActionPress, ActionScroll,
		ActionSelect, ActionWait, ActionBack, ActionForward,
	} {
		t.Run(string(action), func(t *testing.T) {
			request := ActionRequest{
				SessionID: session.ID, TabID: "tab-1",
				Ref: snapshot.Nodes[0].Ref, Condition: WaitLoad,
			}
			if action == ActionOpen || action == ActionNavigate {
				request.URL = testPageURL + "/target"
			}
			if action == ActionStatus || action == ActionProfiles || action == ActionStart {
				request.SessionID = ""
				request.TabID = ""
			}
			operations, operationErr := service.ResolveOperations(ctx, action, request)
			require.NoError(t, operationErr)
			require.NotEmpty(t, operations)
			for _, operation := range operations {
				require.Equal(t, "browser", operation.Tool)
				require.NotEqual(t, permissions.ActionUnknown, operation.Action)
			}
		})
	}
}

func TestService_NavigateAndWaitUpdateGenerationAndActivity(t *testing.T) {
	backendSession := newInteractiveBackendSession()
	service, err := NewService(
		context.Background(), testBrowserConfig(t), allowChecker(),
		&interactiveBackend{session: backendSession},
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close(context.Background())) })
	ctx := testBrowserContext("owner", "session")
	session, err := service.Start(ctx, StartRequest{})
	require.NoError(t, err)
	_, err = service.Tabs(ctx, session.ID)
	require.NoError(t, err)

	tab, err := service.Navigate(ctx, ActionRequest{
		SessionID: session.ID, TabID: "tab-1", URL: "https://93.184.216.35/next",
	})
	require.NoError(t, err)
	require.Equal(t, uint64(2), tab.Generation)
	require.Equal(t, "https://93.184.216.35/next", tab.URL)

	tab, err = service.Wait(ctx, ActionRequest{
		SessionID: session.ID, TabID: "tab-1", Condition: WaitURL, Value: "next",
	})
	require.NoError(t, err)
	require.Equal(t, uint64(2), tab.Generation)
	require.Equal(t, []WaitCondition{WaitURL}, backendSession.waits)
}

func TestService_NavigateDeniesServerObservedRedirectBeforeBackendMutation(t *testing.T) {
	backendSession := newInteractiveBackendSession()
	backendSession.requestTarget = "https://93.184.216.36/redirect"
	checker := checkerFunc(func(_ context.Context, input permissions.EvaluationInput) (permissions.Evaluation, error) {
		if input.Operation.Network != nil && input.Operation.Network.Host == "93.184.216.36" {
			evaluation := permissions.Evaluation{Decision: permissions.DecisionDeny, Reason: "redirect denied"}
			return evaluation, &permissions.DecisionError{
				Code: permissions.ErrorCodeDenied, Evaluation: evaluation,
			}
		}
		return permissions.Evaluation{Decision: permissions.DecisionAllow}, nil
	})
	service, err := NewService(
		context.Background(), testBrowserConfig(t), checker,
		&interactiveBackend{session: backendSession},
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close(context.Background())) })
	ctx := testBrowserContext("owner", "session")
	session, err := service.Start(ctx, StartRequest{})
	require.NoError(t, err)
	_, err = service.Tabs(ctx, session.ID)
	require.NoError(t, err)

	_, err = service.Navigate(ctx, ActionRequest{
		SessionID: session.ID, TabID: "tab-1", URL: "https://93.184.216.35/next",
	})
	decisionErr, ok := permissions.GetDecisionError(err)
	require.True(t, ok)
	require.Equal(t, permissions.ErrorCodeDenied, decisionErr.Code)
	backendSession.mu.Lock()
	require.Equal(t, testPageURL, backendSession.tabs["tab-1"].URL)
	backendSession.mu.Unlock()
}

func TestService_NavigateApprovesServerObservedRedirect(t *testing.T) {
	backendSession := newInteractiveBackendSession()
	backendSession.requestTarget = "https://93.184.216.36/redirect"
	checker := checkerFunc(func(_ context.Context, input permissions.EvaluationInput) (permissions.Evaluation, error) {
		if input.Operation.Network == nil || input.Operation.Network.Host != "93.184.216.36" {
			return permissions.Evaluation{Decision: permissions.DecisionAllow}, nil
		}
		evaluation := permissions.Evaluation{Decision: permissions.DecisionAsk, Reason: "approve redirect"}
		return evaluation, &permissions.DecisionError{
			Code: permissions.ErrorCodeApprovalRequired, Evaluation: evaluation,
		}
	})
	service, err := NewService(
		context.Background(), testBrowserConfig(t), checker,
		&interactiveBackend{session: backendSession},
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close(context.Background())) })
	var approved permissions.EvaluationInput
	service.SetApprover(approverFunc(func(_ context.Context, input permissions.EvaluationInput) error {
		approved = input
		return nil
	}))
	ctx := testBrowserContext("owner", "session")
	session, err := service.Start(ctx, StartRequest{})
	require.NoError(t, err)
	_, err = service.Tabs(ctx, session.ID)
	require.NoError(t, err)

	tab, err := service.Navigate(ctx, ActionRequest{
		SessionID: session.ID, TabID: "tab-1", URL: "https://93.184.216.35/next",
	})
	require.NoError(t, err)
	require.Equal(t, "approve redirect", approved.ApprovalReason)
	require.Equal(t, "93.184.216.36", approved.Operation.Network.Host)
	require.Equal(t, "https://93.184.216.35/next", tab.URL)
}

func TestService_NavigateDoesNotMutateWhenRedirectApprovalFails(t *testing.T) {
	backendSession := newInteractiveBackendSession()
	backendSession.requestTarget = "https://93.184.216.36/redirect"
	checker := checkerFunc(func(_ context.Context, input permissions.EvaluationInput) (permissions.Evaluation, error) {
		if input.Operation.Network == nil || input.Operation.Network.Host != "93.184.216.36" {
			return permissions.Evaluation{Decision: permissions.DecisionAllow}, nil
		}
		evaluation := permissions.Evaluation{Decision: permissions.DecisionAsk, Reason: "approve redirect"}
		return evaluation, &permissions.DecisionError{
			Code: permissions.ErrorCodeApprovalRequired, Evaluation: evaluation,
		}
	})
	service, err := NewService(
		context.Background(), testBrowserConfig(t), checker,
		&interactiveBackend{session: backendSession},
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close(context.Background())) })
	expected := errors.New("approval cancelled")
	service.SetApprover(approverFunc(func(context.Context, permissions.EvaluationInput) error {
		return expected
	}))
	ctx := testBrowserContext("owner", "session")
	session, err := service.Start(ctx, StartRequest{})
	require.NoError(t, err)
	_, err = service.Tabs(ctx, session.ID)
	require.NoError(t, err)

	_, err = service.Navigate(ctx, ActionRequest{
		SessionID: session.ID, TabID: "tab-1", URL: "https://93.184.216.35/next",
	})
	require.ErrorIs(t, err, expected)
	backendSession.mu.Lock()
	require.Equal(t, testPageURL, backendSession.tabs["tab-1"].URL)
	backendSession.mu.Unlock()
}

func TestService_InteractiveActionsDispatchAndMaintainTabState(t *testing.T) {
	backendSession := newInteractiveBackendSession()
	service, err := NewService(
		context.Background(), testBrowserConfig(t), allowChecker(),
		&interactiveBackend{session: backendSession},
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close(context.Background())) })
	ctx := testBrowserContext("owner", "session")
	session, err := service.Start(ctx, StartRequest{})
	require.NoError(t, err)

	opened, err := service.Open(ctx, ActionRequest{SessionID: session.ID, URL: testPageURL + "/opened"})
	require.NoError(t, err)
	require.Equal(t, "opened-tab", opened.ID)
	require.True(t, opened.Active)

	focused, err := service.Focus(ctx, ActionRequest{SessionID: session.ID, TabID: "tab-1"})
	require.NoError(t, err)
	require.True(t, focused.Active)

	for action, run := range map[Action]func(ActionRequest) (Tab, error){
		ActionType:   func(request ActionRequest) (Tab, error) { return service.Type(ctx, request) },
		ActionSelect: func(request ActionRequest) (Tab, error) { return service.Select(ctx, request) },
	} {
		snapshot, snapshotErr := service.Snapshot(ctx, ActionRequest{SessionID: session.ID, TabID: "tab-1"})
		require.NoError(t, snapshotErr)
		request := ActionRequest{
			SessionID: session.ID, TabID: "tab-1", Ref: snapshot.Nodes[0].Ref,
			Text: "Ada", Value: "choice", Replace: true,
		}
		updated, runErr := run(request)
		require.NoError(t, runErr, action)
		require.Greater(t, updated.Generation, snapshot.Generation)
	}

	pressed, err := service.Press(ctx, ActionRequest{SessionID: session.ID, TabID: "tab-1", Key: "Enter"})
	require.NoError(t, err)
	require.NotZero(t, pressed.Generation)
	scrolled, err := service.Scroll(ctx, ActionRequest{SessionID: session.ID, TabID: "tab-1", X: 4, Y: 9})
	require.NoError(t, err)
	require.Equal(t, pressed.Generation, scrolled.Generation)

	for _, navigation := range []func(context.Context, ActionRequest) (Tab, error){service.Back, service.Forward, service.Reload} {
		tab, navigationErr := navigation(ctx, ActionRequest{SessionID: session.ID, TabID: "tab-1"})
		require.NoError(t, navigationErr)
		require.NotEmpty(t, tab.URL)
	}

	visibleSnapshot, err := service.Snapshot(ctx, ActionRequest{SessionID: session.ID, TabID: "tab-1"})
	require.NoError(t, err)
	for _, request := range []ActionRequest{
		{SessionID: session.ID, TabID: "tab-1", Condition: WaitLoad},
		{SessionID: session.ID, TabID: "tab-1", Condition: WaitText, Value: "Page"},
		{SessionID: session.ID, TabID: "tab-1", Condition: WaitURL, Value: "page"},
		{SessionID: session.ID, TabID: "tab-1", Condition: WaitVisible, Ref: visibleSnapshot.Nodes[0].Ref},
	} {
		_, err = service.Wait(ctx, request)
		require.NoError(t, err)
	}

	closed, err := service.CloseTab(ctx, ActionRequest{SessionID: session.ID, TabID: opened.ID})
	require.NoError(t, err)
	require.Equal(t, opened.ID, closed.ID)
	tabs, err := service.Tabs(ctx, session.ID)
	require.NoError(t, err)
	require.Len(t, tabs, 1)

	backendSession.mu.Lock()
	require.Equal(t, []string{"Ada"}, backendSession.typed)
	require.Equal(t, []string{"choice"}, backendSession.selected)
	require.Equal(t, []string{"Enter"}, backendSession.pressed)
	require.Equal(t, [][2]int64{{4, 9}}, backendSession.scrolled)
	require.Equal(t, []WaitCondition{WaitLoad, WaitText, WaitURL, WaitVisible}, backendSession.waits)
	require.Contains(t, backendSession.authorizedTabs, "")
	require.Contains(t, backendSession.authorizedTabs, "tab-1")
	backendSession.mu.Unlock()
}

func TestService_SnapshotTruncatesBeforeExceedingOutputLimit(t *testing.T) {
	backendSession := newInteractiveBackendSession()
	backendSession.snapshots["tab-1"] = BackendSnapshot{
		URL: testPageURL,
		Nodes: []BackendSnapshotNode{{
			BackendNodeID: 41,
			Role:          "textbox",
			Properties:    map[string]string{"value": strings.Repeat("x", maxSnapshotChars)},
		}},
	}
	service, err := NewService(
		context.Background(), testBrowserConfig(t), allowChecker(),
		&interactiveBackend{session: backendSession},
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close(context.Background())) })
	ctx := testBrowserContext("owner", "session")
	session, err := service.Start(ctx, StartRequest{})
	require.NoError(t, err)

	snapshot, err := service.Snapshot(ctx, ActionRequest{SessionID: session.ID, TabID: "tab-1"})
	require.NoError(t, err)
	require.True(t, snapshot.Truncated)
	require.Empty(t, snapshot.Nodes)
}

func TestService_ActionsReturnStableValidationAndCapabilityErrors(t *testing.T) {
	_, err := (*Service)(nil).ResolveOperations(context.Background(), ActionStatus, ActionRequest{})
	require.EqualError(t, err, "browser service is required")

	service, err := NewService(context.Background(), testBrowserConfig(t), allowChecker(), &fakeBackend{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close(context.Background())) })
	ctx := testBrowserContext("owner", "session")
	session, err := service.Start(ctx, StartRequest{})
	require.NoError(t, err)
	_, err = service.Tabs(ctx, session.ID)
	browserErr, ok := GetError(err)
	require.True(t, ok)
	require.Equal(t, ErrorUnavailable, browserErr.Code)

	_, err = getActionTimeout(-time.Second)
	require.EqualError(t, err, "browser action timeout must be between zero and two minutes")
	_, err = getActionTimeout(maxActionTimeout + time.Millisecond)
	require.EqualError(t, err, "browser action timeout must be between zero and two minutes")
	require.Equal(t, defaultActionTimeout, mustGetActionTimeout(t, 0))
	require.Equal(t, time.Second, mustGetActionTimeout(t, time.Second))
}

func TestService_WaitReturnsStableTimeoutError(t *testing.T) {
	backendSession := newInteractiveBackendSession()
	backendSession.blockWait = true
	service, err := NewService(
		context.Background(), testBrowserConfig(t), allowChecker(),
		&interactiveBackend{session: backendSession},
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close(context.Background())) })
	ctx := testBrowserContext("owner", "session")
	session, err := service.Start(ctx, StartRequest{})
	require.NoError(t, err)

	_, err = service.Wait(ctx, ActionRequest{
		SessionID: session.ID, TabID: "tab-1", Condition: WaitLoad, Timeout: time.Millisecond,
	})
	browserErr, ok := GetError(err)
	require.True(t, ok)
	require.Equal(t, ErrorTimeout, browserErr.Code)
	require.True(t, browserErr.Retryable)
}

func TestService_CheckOperationsRequiresExactAdmissionEvidence(t *testing.T) {
	operation := permissions.Operation{
		Tool: "browser", Resource: permissions.ResourceBrowser, Action: permissions.ActionUpdate,
		Effects: []permissions.Effect{permissions.EffectWrite}, Target: "profile=default",
	}
	checker := checkerFunc(func(context.Context, permissions.EvaluationInput) (permissions.Evaluation, error) {
		evaluation := permissions.Evaluation{Decision: permissions.DecisionDeny, Reason: "changed operation"}
		return evaluation, &permissions.DecisionError{
			Code: permissions.ErrorCodeDenied, Evaluation: evaluation,
		}
	})
	service, err := NewService(
		context.Background(), testBrowserConfig(t), checker,
		&interactiveBackend{session: newInteractiveBackendSession()},
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close(context.Background())) })
	changed := operation
	changed.Effects = []permissions.Effect{permissions.EffectWrite, permissions.EffectExternalSystem}
	ctx := permissions.WithAuthorizedOperations(context.Background(), []permissions.Operation{operation})

	err = service.checkOperations(ctx, []permissions.Operation{changed})
	decisionErr, ok := permissions.GetDecisionError(err)
	require.True(t, ok)
	require.Equal(t, permissions.ErrorCodeDenied, decisionErr.Code)
}

func TestService_InteractiveBackendFailuresReturnStableErrors(t *testing.T) {
	tests := []struct {
		action Action
		run    func(context.Context, *Service, string, string) error
	}{
		{action: ActionTabs, run: func(ctx context.Context, service *Service, sessionID, _ string) error {
			_, err := service.Tabs(ctx, sessionID)
			return err
		}},
		{action: ActionOpen, run: func(ctx context.Context, service *Service, sessionID, _ string) error {
			_, err := service.Open(ctx, ActionRequest{SessionID: sessionID, URL: testPageURL + "/open"})
			return err
		}},
		{action: ActionFocus, run: runFailedTabAction((*Service).Focus)},
		{action: ActionClose, run: runFailedTabAction((*Service).CloseTab)},
		{action: ActionNavigate, run: func(ctx context.Context, service *Service, sessionID, _ string) error {
			_, err := service.Navigate(ctx, ActionRequest{
				SessionID: sessionID, TabID: "tab-1", URL: testPageURL + "/next",
			})
			return err
		}},
		{action: ActionBack, run: runFailedTabAction((*Service).Back)},
		{action: ActionForward, run: runFailedTabAction((*Service).Forward)},
		{action: ActionReload, run: runFailedTabAction((*Service).Reload)},
		{action: ActionSnapshot, run: runFailedTabAction(func(
			service *Service, ctx context.Context, request ActionRequest,
		) (Tab, error) {
			_, err := service.Snapshot(ctx, request)
			return Tab{}, err
		})},
		{action: ActionClick, run: runFailedElementAction((*Service).Click)},
		{action: ActionType, run: runFailedElementAction((*Service).Type)},
		{action: ActionPress, run: runFailedTabAction((*Service).Press)},
		{action: ActionScroll, run: runFailedTabAction((*Service).Scroll)},
		{action: ActionSelect, run: runFailedElementAction((*Service).Select)},
		{action: ActionWait, run: runFailedTabAction((*Service).Wait)},
	}

	for _, test := range tests {
		t.Run(string(test.action), func(t *testing.T) {
			backendSession := newInteractiveBackendSession()
			service, err := NewService(
				context.Background(), testBrowserConfig(t), allowChecker(),
				&interactiveBackend{session: backendSession},
			)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, service.Close(context.Background())) })
			ctx := testBrowserContext("owner", "session")
			session, err := service.Start(ctx, StartRequest{})
			require.NoError(t, err)
			var ref string
			if test.action == ActionClick || test.action == ActionType || test.action == ActionSelect {
				snapshot, snapshotErr := service.Snapshot(ctx, ActionRequest{SessionID: session.ID, TabID: "tab-1"})
				require.NoError(t, snapshotErr)
				ref = snapshot.Nodes[0].Ref
			}
			backendSession.failAction = test.action

			err = test.run(ctx, service, session.ID, ref)
			browserErr, ok := GetError(err)
			require.True(t, ok)
			require.Equal(t, ErrorUnavailable, browserErr.Code)
			require.Equal(t, test.action, browserErr.Operation)
			require.True(t, browserErr.Retryable)
		})
	}
}

func runFailedTabAction(
	run func(*Service, context.Context, ActionRequest) (Tab, error),
) func(context.Context, *Service, string, string) error {
	return func(ctx context.Context, service *Service, sessionID, _ string) error {
		_, err := run(service, ctx, ActionRequest{
			SessionID: sessionID, TabID: "tab-1", Key: "Enter", X: 1, Y: 2,
			Condition: WaitLoad,
		})
		return err
	}
}

func runFailedElementAction(
	run func(*Service, context.Context, ActionRequest) (Tab, error),
) func(context.Context, *Service, string, string) error {
	return func(ctx context.Context, service *Service, sessionID, ref string) error {
		_, err := run(service, ctx, ActionRequest{
			SessionID: sessionID, TabID: "tab-1", Ref: ref, Text: "value", Value: "value",
		})
		return err
	}
}

func mustGetActionTimeout(t *testing.T, timeout time.Duration) time.Duration {
	t.Helper()
	value, err := getActionTimeout(timeout)
	require.NoError(t, err)
	return value
}

func TestIsActionableRole_ClassifiesSupportedRoles(t *testing.T) {
	for _, role := range []string{
		"button", "checkbox", "combobox", "link", "listbox", "menuitem", "option", "radio",
		"searchbox", "slider", "spinbutton", "switch", "tab", "textbox",
	} {
		require.True(t, isActionableRole(role), role)
	}
	require.False(t, isActionableRole("paragraph"))
}
