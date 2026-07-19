package browser

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/wandxy/morph/internal/config"
	"github.com/wandxy/morph/internal/permissions"
)

type checkerFunc func(context.Context, permissions.EvaluationInput) (permissions.Evaluation, error)

func (f checkerFunc) Check(ctx context.Context, input permissions.EvaluationInput) (permissions.Evaluation, error) {
	return f(ctx, input)
}

type fakeBackend struct {
	mu        sync.Mutex
	starts    int
	options   LaunchOptions
	startErr  error
	healthErr error
	session   *fakeBackendSession
}

type fakeBackendSession struct {
	mu          sync.Mutex
	closed      int
	healthErr   error
	closeCtxErr error
}

type blockingBackend struct {
	started chan struct{}
	release chan struct{}
	session *fakeBackendSession
}

func (b *blockingBackend) Start(context.Context, LaunchOptions) (BackendSession, error) {
	close(b.started)
	<-b.release
	return b.session, nil
}

func (b *fakeBackend) Start(_ context.Context, options LaunchOptions) (BackendSession, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.starts++
	b.options = options
	if b.startErr != nil {
		return nil, b.startErr
	}
	b.session = &fakeBackendSession{healthErr: b.healthErr}
	return b.session, nil
}

func (s *fakeBackendSession) Health(context.Context) error {
	return s.healthErr
}

func (s *fakeBackendSession) Close(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed++
	s.closeCtxErr = ctx.Err()
	return nil
}

func TestService_StartStopPreservesOwnershipAndCleansEphemeralProfile(t *testing.T) {
	cfg := testBrowserConfig(t)
	backend := &fakeBackend{}
	service, err := NewService(context.Background(), cfg, allowChecker(), backend)
	require.NoError(t, err)
	ctx := testBrowserContext("owner", "session")

	session, err := service.Start(ctx, StartRequest{})
	require.NoError(t, err)
	require.Equal(t, SessionReady, session.State)
	require.DirExists(t, backend.options.DataDir)
	info, err := os.Stat(backend.options.DataDir)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o700), info.Mode().Perm())
	require.NotEmpty(t, backend.options.ProxyURL)
	require.Equal(t, cfg.Executable, backend.options.Executable)

	err = service.Touch(testBrowserContext("other", "session"), session.ID)
	require.EqualError(t, err, "browser session belongs to another owner")
	_, err = service.Stop(testBrowserContext("owner", "other-session"), session.ID)
	require.EqualError(t, err, "browser session belongs to another owner")
	differentRun := permissions.WithContext(context.Background(), permissions.AuthorizationContext{
		Actor:   permissions.Actor{Kind: permissions.ActorLocalOwner, ID: "owner"},
		Surface: permissions.SurfaceTUI, Profile: "default", SessionID: "session", RunID: "other-run",
	})
	require.NoError(t, service.Touch(differentRun, session.ID))

	stopped, err := service.Stop(differentRun, session.ID)
	require.NoError(t, err)
	require.Equal(t, SessionStopped, stopped.State)
	require.NoDirExists(t, backend.options.DataDir)
	require.Equal(t, 1, backend.session.closed)
	require.NoError(t, service.Close(context.Background()))
}

func TestService_PermissionDenialPreventsBackendAndFilesystemSideEffects(t *testing.T) {
	cfg := testBrowserConfig(t)
	backend := &fakeBackend{}
	checker := checkerFunc(func(context.Context, permissions.EvaluationInput) (permissions.Evaluation, error) {
		return permissions.Evaluation{Decision: permissions.DecisionDeny}, &permissions.DecisionError{
			Code:       permissions.ErrorCodeDenied,
			Evaluation: permissions.Evaluation{Decision: permissions.DecisionDeny},
		}
	})
	service, err := NewService(context.Background(), cfg, checker, backend)
	require.NoError(t, err)

	_, err = service.Start(testBrowserContext("owner", "session"), StartRequest{})
	require.Error(t, err)
	require.Zero(t, backend.starts)
	entries, readErr := os.ReadDir(cfg.TemporaryRoot)
	if !errors.Is(readErr, os.ErrNotExist) {
		require.NoError(t, readErr)
		require.Empty(t, entries)
	}
	require.NoError(t, service.Close(context.Background()))
}

func TestService_BackendFailureCleansResourcesAndReportsFailedState(t *testing.T) {
	cfg := testBrowserConfig(t)
	backend := &fakeBackend{startErr: errors.New("launch failed")}
	service, err := NewService(context.Background(), cfg, allowChecker(), backend)
	require.NoError(t, err)

	session, err := service.Start(testBrowserContext("owner", "session"), StartRequest{})
	require.EqualError(t, err, "launch failed")
	browserErr, ok := GetError(err)
	require.True(t, ok)
	require.Equal(t, ErrorStartFailed, browserErr.Code)
	require.True(t, browserErr.Retryable)
	require.Equal(t, SessionFailed, session.State)
	require.Equal(t, "launch failed", session.Error)
	require.Len(t, service.Status().Sessions, 1)
	entries, readErr := os.ReadDir(cfg.TemporaryRoot)
	require.NoError(t, readErr)
	require.Empty(t, entries)
	require.NoError(t, service.Close(context.Background()))
}

func TestService_BackendHealthFailureClosesStartedBackend(t *testing.T) {
	cfg := testBrowserConfig(t)
	backend := &fakeBackend{healthErr: errors.New("not healthy")}
	service, err := NewService(context.Background(), cfg, allowChecker(), backend)
	require.NoError(t, err)

	session, err := service.Start(testBrowserContext("owner", "session"), StartRequest{})
	require.EqualError(t, err, "not healthy")
	require.Equal(t, SessionFailed, session.State)
	require.Equal(t, 1, backend.session.closed)
	require.NoDirExists(t, backend.options.DataDir)
	require.NoError(t, service.Close(context.Background()))
}

func TestService_StrictRemotePolicyBlocksBeforeBackendStart(t *testing.T) {
	cfg := testBrowserConfig(t)
	cfg.Profiles = []config.BrowserProfileConfig{{
		Name: "remote", Mode: config.BrowserProfileRemoteCDP, CDPEndpoint: "http://127.0.0.1:9222",
	}}
	cfg.DefaultProfile = "remote"
	backend := &fakeBackend{}
	policy := permissions.Policy{
		Default: permissions.DecisionAllow,
		SurfaceKindDefaults: map[permissions.SurfaceKind]permissions.Decision{
			permissions.SurfaceKindLocal: permissions.DecisionAllow,
		},
	}
	service, err := NewService(context.Background(), cfg, permissions.NewEngine(policy), backend)
	require.NoError(t, err)

	_, err = service.Start(testBrowserContext("owner", "session"), StartRequest{})
	decision, ok := permissions.GetDecisionError(err)
	require.True(t, ok)
	require.Equal(t, permissions.ReasonHardDeny, decision.Evaluation.ReasonCode)
	require.Zero(t, backend.starts)
	require.NoError(t, service.Close(context.Background()))
}

func TestService_FullAccessCanUseExplicitRemoteEndpoint(t *testing.T) {
	cfg := testBrowserConfig(t)
	cfg.Profiles = []config.BrowserProfileConfig{{
		Name: "remote", Mode: config.BrowserProfileRemoteCDP, CDPEndpoint: "http://127.0.0.1:9222",
	}}
	cfg.DefaultProfile = "remote"
	backend := &fakeBackend{}
	service, err := NewService(
		context.Background(), cfg, permissions.NewEngine(permissions.Policy{Preset: permissions.PresetFullAccess}), backend,
	)
	require.NoError(t, err)
	ctx := permissions.WithPreset(testBrowserContext("owner", "session"), permissions.PresetFullAccess)

	session, err := service.Start(ctx, StartRequest{})
	require.NoError(t, err)
	require.Equal(t, SessionReady, session.State)
	require.Equal(t, []Profile{{
		Name: "remote", Mode: config.BrowserProfileRemoteCDP, Default: true, Available: true,
	}}, service.Status().Profiles)
	require.NotEqual(t, cfg.Profiles[0].CDPEndpoint, backend.options.CDPEndpoint)
	require.Contains(t, backend.options.CDPEndpoint, "127.0.0.1:")
	require.False(t, service.sessions[session.ID].remoteRelay.getPolicy().Strict)
	normal := permissions.WithContext(context.Background(), permissions.AuthorizationContext{
		Actor:   permissions.Actor{Kind: permissions.ActorLocalOwner, ID: "owner"},
		Surface: permissions.SurfaceTUI, Profile: "default", SessionID: "session", RunID: "next-run",
	})
	require.NoError(t, service.Touch(normal, session.ID))
	require.True(t, service.sessions[session.ID].remoteRelay.getPolicy().Strict)
	require.NoError(t, service.Close(context.Background()))
}

func TestService_CloseUsesLiveCleanupContextAndRejectsFurtherStarts(t *testing.T) {
	cfg := testBrowserConfig(t)
	backend := &fakeBackend{}
	checks := 0
	checker := checkerFunc(func(context.Context, permissions.EvaluationInput) (permissions.Evaluation, error) {
		checks++
		return permissions.Evaluation{Decision: permissions.DecisionAllow}, nil
	})
	service, err := NewService(context.Background(), cfg, checker, backend)
	require.NoError(t, err)
	ctx := testBrowserContext("owner", "session")
	_, err = service.Start(ctx, StartRequest{})
	require.NoError(t, err)

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	require.NoError(t, service.Close(cancelled))
	require.NoError(t, backend.session.closeCtxErr)
	checksBeforeStart := checks
	_, err = service.Start(ctx, StartRequest{})
	require.Error(t, err)
	browserErr, ok := GetError(err)
	require.True(t, ok)
	require.Equal(t, ErrorClosed, browserErr.Code)
	require.Equal(t, checksBeforeStart, checks)
}

func TestService_StoppedSessionCannotBeTouchedAndStopIsIdempotent(t *testing.T) {
	cfg := testBrowserConfig(t)
	backend := &fakeBackend{}
	checks := 0
	checker := checkerFunc(func(context.Context, permissions.EvaluationInput) (permissions.Evaluation, error) {
		checks++
		return permissions.Evaluation{Decision: permissions.DecisionAllow}, nil
	})
	service, err := NewService(context.Background(), cfg, checker, backend)
	require.NoError(t, err)
	ctx := testBrowserContext("owner", "session")
	session, err := service.Start(ctx, StartRequest{})
	require.NoError(t, err)
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	_, err = service.Stop(cancelled, session.ID)
	require.NoError(t, err)
	require.NoError(t, backend.session.closeCtxErr)
	checksAfterStop := checks

	_, err = service.Stop(ctx, session.ID)
	require.NoError(t, err)
	require.Equal(t, checksAfterStop, checks)
	require.EqualError(t, service.Touch(ctx, session.ID), "browser session is not ready")
	require.NoError(t, service.Close(context.Background()))
}

func TestService_CloseDuringStartupLeavesSessionStoppedAndClosesLateBackend(t *testing.T) {
	cfg := testBrowserConfig(t)
	backend := &blockingBackend{
		started: make(chan struct{}), release: make(chan struct{}), session: &fakeBackendSession{},
	}
	service, err := NewService(context.Background(), cfg, allowChecker(), backend)
	require.NoError(t, err)
	result := make(chan Session, 1)
	errors := make(chan error, 1)
	go func() {
		session, startErr := service.Start(testBrowserContext("owner", "session"), StartRequest{})
		result <- session
		errors <- startErr
	}()
	<-backend.started
	require.NoError(t, service.Close(context.Background()))
	close(backend.release)

	session := <-result
	startErr := <-errors
	require.Error(t, startErr)
	browserErr, ok := GetError(startErr)
	require.True(t, ok)
	require.Equal(t, ErrorClosed, browserErr.Code)
	require.Equal(t, SessionStopped, session.State)
	require.Equal(t, 1, backend.session.closed)
	require.NoError(t, backend.session.closeCtxErr)
}

func TestService_ExistingSessionProfileIsUnavailable(t *testing.T) {
	cfg := testBrowserConfig(t)
	cfg.Profiles = []config.BrowserProfileConfig{{
		Name: "personal", Mode: config.BrowserProfileExistingSession, CDPEndpoint: "http://127.0.0.1:9222",
	}}
	cfg.DefaultProfile = "personal"
	backend := &fakeBackend{}
	service, err := NewService(context.Background(), cfg, allowChecker(), backend)
	require.NoError(t, err)
	require.Equal(t, []Profile{{
		Name: "personal", Mode: config.BrowserProfileExistingSession, Default: true, Available: false,
	}}, service.Status().Profiles)
	_, err = service.Start(testBrowserContext("owner", "session"), StartRequest{})
	require.EqualError(t, err, "existing browser session profiles require explicit attachment support")
	require.Zero(t, backend.starts)
	require.NoError(t, service.Close(context.Background()))
}

func TestService_AuthorizeAppliesNetworkHardDenyUnlessFullAccess(t *testing.T) {
	cfg := testBrowserConfig(t)
	policy := permissions.Policy{
		Default: permissions.DecisionAllow,
		SurfaceKindDefaults: map[permissions.SurfaceKind]permissions.Decision{
			permissions.SurfaceKindLocal: permissions.DecisionAllow,
		},
	}
	service, err := NewService(context.Background(), cfg, permissions.NewEngine(policy), &fakeBackend{})
	require.NoError(t, err)
	target, err := permissions.NetworkTargetFromURL(
		"http://127.0.0.1/private", "GET", permissions.NetworkRequestNavigation,
	)
	require.NoError(t, err)
	request := permissions.BrowserRequest{Profile: "default", Action: "navigate", Network: &target}

	err = service.Authorize(testBrowserContext("owner", "session"), request)
	decision, ok := permissions.GetDecisionError(err)
	require.True(t, ok)
	require.Equal(t, permissions.ReasonHardDeny, decision.Evaluation.ReasonCode)
	fullAccess := permissions.WithFullAccess(testBrowserContext("owner", "session"))
	fullAccess = permissions.WithPreset(fullAccess, permissions.PresetFullAccess)
	require.NoError(t, service.Authorize(fullAccess, request))
	require.NoError(t, service.Close(context.Background()))
}

func TestService_AuthorizeAttachesHardDenyOnlyToNetworkOperation(t *testing.T) {
	cfg := testBrowserConfig(t)
	inputs := make([]permissions.EvaluationInput, 0, 2)
	checker := checkerFunc(func(_ context.Context, input permissions.EvaluationInput) (permissions.Evaluation, error) {
		inputs = append(inputs, input)
		return permissions.Evaluation{Decision: permissions.DecisionAllow}, nil
	})
	service, err := NewService(context.Background(), cfg, checker, &fakeBackend{})
	require.NoError(t, err)
	target, err := permissions.NetworkTargetFromURL(
		"http://127.0.0.1/private", http.MethodGet, permissions.NetworkRequestNavigation,
	)
	require.NoError(t, err)

	require.NoError(t, service.Authorize(testBrowserContext("owner", "session"), permissions.BrowserRequest{
		Profile: "default", Action: "navigate", Network: &target,
	}))
	require.Len(t, inputs, 2)
	require.Equal(t, permissions.ResourceBrowser, inputs[0].Operation.Resource)
	require.Empty(t, inputs[0].HardDenyReason)
	require.Equal(t, permissions.ResourceNetwork, inputs[1].Operation.Resource)
	require.NotEmpty(t, inputs[1].HardDenyReason)
	require.NoError(t, service.Close(context.Background()))
}

func TestService_PublicMethodsRejectMissingServiceOwnerAndSession(t *testing.T) {
	var service *Service
	_, err := service.Start(context.Background(), StartRequest{})
	require.EqualError(t, err, "browser service is required")
	_, err = service.Stop(context.Background(), "missing")
	require.EqualError(t, err, "browser service is required")
	require.NoError(t, service.Close(context.Background()))

	cfg := testBrowserConfig(t)
	service, err = NewService(context.Background(), cfg, allowChecker(), &fakeBackend{})
	require.NoError(t, err)
	require.EqualError(t, service.Touch(context.Background(), "missing"), "browser authorization owner is required")
	require.EqualError(
		t, service.Touch(testBrowserContext("owner", "session"), "missing"), "browser session not found",
	)
	_, err = service.Stop(context.Background(), "missing")
	require.EqualError(t, err, "browser authorization owner is required")
	require.NoError(t, service.Close(context.Background()))
}

func TestService_StopPermissionDenialLeavesSessionReady(t *testing.T) {
	cfg := testBrowserConfig(t)
	denyStop := false
	checker := checkerFunc(func(_ context.Context, input permissions.EvaluationInput) (permissions.Evaluation, error) {
		if denyStop && input.Operation.Action == permissions.ActionStop {
			evaluation := permissions.Evaluation{Decision: permissions.DecisionDeny}
			return evaluation, &permissions.DecisionError{Code: permissions.ErrorCodeDenied, Evaluation: evaluation}
		}
		return permissions.Evaluation{Decision: permissions.DecisionAllow}, nil
	})
	backend := &fakeBackend{}
	service, err := NewService(context.Background(), cfg, checker, backend)
	require.NoError(t, err)
	ctx := testBrowserContext("owner", "session")
	session, err := service.Start(ctx, StartRequest{})
	require.NoError(t, err)
	denyStop = true
	_, err = service.Stop(ctx, session.ID)
	require.Error(t, err)
	require.Equal(t, SessionReady, service.Status().Sessions[0].State)
	denyStop = false
	require.NoError(t, service.Close(context.Background()))
}

func TestService_InactivityStopsReadySession(t *testing.T) {
	cfg := testBrowserConfig(t)
	cfg.InactivityTimeout = time.Second
	cfg.CleanupInterval = 5 * time.Millisecond
	backend := &fakeBackend{}
	var mu sync.Mutex
	now := time.Now().UTC()
	clock := func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return now
	}
	service, err := NewService(context.Background(), cfg, allowChecker(), backend, WithNow(clock))
	require.NoError(t, err)
	session, err := service.Start(testBrowserContext("owner", "session"), StartRequest{})
	require.NoError(t, err)
	mu.Lock()
	now = now.Add(2 * time.Second)
	mu.Unlock()

	require.Eventually(t, func() bool {
		for _, value := range service.Status().Sessions {
			if value.ID == session.ID {
				return value.State == SessionStopped
			}
		}
		return false
	}, time.Second, 10*time.Millisecond)
	require.Equal(t, 1, backend.session.closed)
	require.NoError(t, service.Close(context.Background()))
}

func TestService_CleanupPrunesTerminalSessionsAfterRetention(t *testing.T) {
	cfg := testBrowserConfig(t)
	cfg.CleanupInterval = 5 * time.Millisecond
	cfg.TerminalRetention = time.Second
	backend := &fakeBackend{}
	var mu sync.Mutex
	now := time.Now().UTC()
	clock := func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return now
	}
	service, err := NewService(context.Background(), cfg, allowChecker(), backend, WithNow(clock))
	require.NoError(t, err)
	ctx := testBrowserContext("owner", "session")
	session, err := service.Start(ctx, StartRequest{})
	require.NoError(t, err)
	_, err = service.Stop(ctx, session.ID)
	require.NoError(t, err)
	require.Len(t, service.Status().Sessions, 1)
	mu.Lock()
	now = now.Add(2 * time.Second)
	mu.Unlock()
	require.Eventually(t, func() bool {
		return len(service.Status().Sessions) == 0
	}, time.Second, 10*time.Millisecond)
	require.NoError(t, service.Close(context.Background()))
}

func TestService_CleanupPrunesFailedSessionsAfterRetention(t *testing.T) {
	cfg := testBrowserConfig(t)
	cfg.CleanupInterval = 5 * time.Millisecond
	cfg.TerminalRetention = time.Second
	var mu sync.Mutex
	now := time.Now().UTC()
	clock := func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return now
	}
	service, err := NewService(
		context.Background(), cfg, allowChecker(), &fakeBackend{startErr: errors.New("failed")}, WithNow(clock),
	)
	require.NoError(t, err)
	_, err = service.Start(testBrowserContext("owner", "session"), StartRequest{})
	require.Error(t, err)
	require.Len(t, service.Status().Sessions, 1)
	mu.Lock()
	now = now.Add(2 * time.Second)
	mu.Unlock()
	require.Eventually(t, func() bool {
		return len(service.Status().Sessions) == 0
	}, time.Second, 10*time.Millisecond)
	require.NoError(t, service.Close(context.Background()))
}

func TestService_OwnerCanChangeSessionEgressPolicyAcrossRuns(t *testing.T) {
	cfg := testBrowserConfig(t)
	service, err := NewService(context.Background(), cfg, allowChecker(), &fakeBackend{})
	require.NoError(t, err)
	fullAccess := permissions.WithPreset(testBrowserContext("owner", "session"), permissions.PresetFullAccess)
	fullAccess = permissions.WithFullAccess(fullAccess)
	session, err := service.Start(fullAccess, StartRequest{})
	require.NoError(t, err)
	runtime := service.sessions[session.ID]
	require.False(t, runtime.proxy.getPolicy().Strict)

	normal := permissions.WithContext(context.Background(), permissions.AuthorizationContext{
		Actor:   permissions.Actor{Kind: permissions.ActorLocalOwner, ID: "owner"},
		Surface: permissions.SurfaceTUI, Profile: "default", SessionID: "session", RunID: "next-run",
	})
	require.NoError(t, service.Touch(normal, session.ID))
	require.True(t, runtime.proxy.getPolicy().Strict)
	require.NoError(t, service.Touch(fullAccess, session.ID))
	require.False(t, runtime.proxy.getPolicy().Strict)
	require.NoError(t, service.Close(context.Background()))
}

func TestService_ParentCancellationStopsActiveSessions(t *testing.T) {
	cfg := testBrowserConfig(t)
	backend := &fakeBackend{}
	parent, cancel := context.WithCancel(context.Background())
	service, err := NewService(parent, cfg, allowChecker(), backend)
	require.NoError(t, err)
	session, err := service.Start(testBrowserContext("owner", "session"), StartRequest{})
	require.NoError(t, err)
	cancel()

	require.Eventually(t, func() bool {
		for _, value := range service.Status().Sessions {
			if value.ID == session.ID {
				return value.State == SessionStopped
			}
		}
		return false
	}, time.Second, 10*time.Millisecond)
	require.Equal(t, 1, backend.session.closed)
	require.NoError(t, service.Close(context.Background()))
}

func TestService_PersistentProfileKeepsDataAndReleasesLease(t *testing.T) {
	cfg := testBrowserConfig(t)
	directory := filepath.Join(cfg.ProfileRoot, "persistent")
	require.NoError(t, os.MkdirAll(directory, 0o755))
	require.NoError(t, os.Chmod(directory, 0o755))
	cfg.Profiles = []config.BrowserProfileConfig{{
		Name: "persistent", Mode: config.BrowserProfileManagedPersistent, Directory: directory,
	}}
	cfg.DefaultProfile = "persistent"
	backend := &fakeBackend{}
	service, err := NewService(context.Background(), cfg, allowChecker(), backend)
	require.NoError(t, err)
	ctx := testBrowserContext("owner", "session")

	session, err := service.Start(ctx, StartRequest{})
	require.NoError(t, err)
	require.FileExists(t, getProfileLockPath(directory))
	info, err := os.Stat(directory)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o700), info.Mode().Perm())
	_, err = service.Stop(ctx, session.ID)
	require.NoError(t, err)
	require.DirExists(t, directory)
	require.FileExists(t, getProfileLockPath(directory))
	lease, err := acquireProfileLease(directory)
	require.NoError(t, err)
	require.NoError(t, lease.Close())
	require.NoError(t, service.Close(context.Background()))
}

func TestService_RejectsInvalidConstructionAndStartRequests(t *testing.T) {
	cfg := testBrowserConfig(t)
	_, err := NewService(context.Background(), cfg, nil, &fakeBackend{})
	require.EqualError(t, err, "browser permission checker is required")
	_, err = NewService(context.Background(), cfg, allowChecker(), nil)
	require.EqualError(t, err, "browser backend is required")
	invalidDuration := cfg
	invalidDuration.StartTimeout = 0
	_, err = NewService(context.Background(), invalidDuration, allowChecker(), &fakeBackend{})
	require.EqualError(t, err, "browser lifecycle durations must be greater than zero")
	noProfiles := cfg
	noProfiles.Profiles = nil
	_, err = NewService(context.Background(), noProfiles, allowChecker(), &fakeBackend{})
	require.EqualError(t, err, "browser profiles are required")
	badDefault := cfg
	badDefault.DefaultProfile = "missing"
	_, err = NewService(context.Background(), badDefault, allowChecker(), &fakeBackend{})
	require.EqualError(t, err, "browser default profile is not configured")
	invalidNetwork := cfg
	invalidNetwork.Network.DevelopmentAllowedHosts = []string{"bad/host"}
	_, err = NewService(context.Background(), invalidNetwork, allowChecker(), &fakeBackend{})
	require.EqualError(t, err, "browser development allowed host is invalid")
	_, err = NewService(context.Background(), cfg, allowChecker(), &fakeBackend{}, WithNow(nil))
	require.EqualError(t, err, "browser clock is required")

	service, err := NewService(context.Background(), cfg, allowChecker(), &fakeBackend{})
	require.NoError(t, err)
	_, err = service.Start(context.Background(), StartRequest{})
	require.EqualError(t, err, "browser authorization owner is required")
	_, err = service.Start(testBrowserContext("owner", "session"), StartRequest{Profile: "missing"})
	require.EqualError(t, err, "browser profile is not configured")
	cfg.Enabled = false
	disabled, err := NewService(context.Background(), cfg, allowChecker(), &fakeBackend{})
	require.NoError(t, err)
	_, err = disabled.Start(testBrowserContext("owner", "session"), StartRequest{})
	require.EqualError(t, err, "browser service is disabled")
	require.NoError(t, disabled.Close(context.Background()))
	require.NoError(t, service.Close(context.Background()))
	require.Empty(t, (*Service)(nil).Status().Sessions)
}

func TestGetCleanupContext_PreservesEarlierLiveDeadlineAndReplacesExpiredDeadline(t *testing.T) {
	earlier, cancelEarlier := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancelEarlier()
	cleanup, cancelCleanup := getCleanupContext(earlier, time.Second)
	defer cancelCleanup()
	wantDeadline, ok := earlier.Deadline()
	require.True(t, ok)
	gotDeadline, ok := cleanup.Deadline()
	require.True(t, ok)
	require.Equal(t, wantDeadline, gotDeadline)

	expired, cancelExpired := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancelExpired()
	cleanup, cancelCleanup = getCleanupContext(expired, time.Second)
	defer cancelCleanup()
	require.NoError(t, cleanup.Err())
}

func TestProfileLease_RejectsLiveOwnerAndIgnoresUnlockedFile(t *testing.T) {
	directory := t.TempDir()
	lease, err := acquireProfileLease(directory)
	require.NoError(t, err)
	_, err = acquireProfileLease(directory)
	require.EqualError(t, err, "browser profile is already in use")
	require.NoError(t, lease.Close())

	require.NoError(t, os.WriteFile(getProfileLockPath(directory), []byte("99999999\n"), 0o600))
	lease, err = acquireProfileLease(directory)
	require.NoError(t, err)
	require.NoError(t, lease.Close())
	require.NoError(t, lease.Close())
}

func testBrowserConfig(t *testing.T) config.BrowserConfig {
	t.Helper()
	root := t.TempDir()
	executable := filepath.Join(root, "chromium")
	require.NoError(t, os.WriteFile(executable, []byte("test"), 0o700))
	strict := true
	return config.BrowserConfig{
		Enabled: true, Executable: executable, DefaultProfile: "default",
		ProfileRoot: filepath.Join(root, "profiles"), TemporaryRoot: filepath.Join(root, "tmp"),
		StartTimeout: time.Second, InactivityTimeout: time.Minute, CleanupInterval: time.Minute,
		TerminalRetention: time.Minute,
		Profiles:          []config.BrowserProfileConfig{{Name: "default", Mode: config.BrowserProfileManagedEphemeral}},
		Network:           config.BrowserNetworkConfig{Strict: &strict},
	}
}

func testBrowserContext(actorID, sessionID string) context.Context {
	return permissions.WithContext(context.Background(), permissions.AuthorizationContext{
		Actor:   permissions.Actor{Kind: permissions.ActorLocalOwner, ID: actorID},
		Surface: permissions.SurfaceTUI, Profile: "default", SessionID: sessionID, RunID: "run",
	})
}

func allowChecker() permissions.Checker {
	return checkerFunc(func(context.Context, permissions.EvaluationInput) (permissions.Evaluation, error) {
		return permissions.Evaluation{Decision: permissions.DecisionAllow}, nil
	})
}
