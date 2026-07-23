package browser

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/browser"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
	"github.com/rs/zerolog/log"
	"github.com/wandxy/morph/internal/config"
	"github.com/wandxy/morph/internal/permissions"
)

type ChromiumBackend struct{}

type networkAuthorization struct {
	id        uint64
	ctx       context.Context
	cancel    context.CancelFunc
	authorize NetworkRequestAuthorizer
}

type networkActivity struct {
	pending int
	last    time.Time
	changed chan struct{}
}

type chromiumSession struct {
	ctx                context.Context
	cancelContext      context.CancelFunc
	cancelBootstrap    context.CancelFunc
	cancelAllocator    context.CancelFunc
	process            *browserProcess
	once               sync.Once
	mu                 sync.Mutex
	activeTabID        string
	openingTargets     int
	tabContexts        map[string]context.Context
	tabCancels         map[string]context.CancelFunc
	proxyUser          string
	proxySecret        string
	networkAuthorizers map[string]networkAuthorization
	nextAuthorization  uint64
	openingTabIDs      map[string]struct{}
	networkErrors      map[string]error
	networkActivity    map[string]*networkActivity
	consoleMessages    map[string][]ConsoleMessage
	dialogResponses    map[string]dialogResponse
	downloadEvents     chan any
	downloadArmed      bool
	downloadFrameIDs   map[cdp.FrameID]struct{}
	downloadGUID       string
	downloadMaxBytes   int64
	downloadLimitSent  bool
	downloadRoot       string
	attachmentScope    string
	browserContextID   string
	attachmentTargets  map[string]struct{}
	quarantinedTargets map[string]struct{}
	attached           bool
	closeErr           error
}

type dialogResponse struct {
	accept     bool
	promptText string
	result     chan error
}

func (ChromiumBackend) Start(ctx context.Context, opts LaunchOptions) (BackendSession, error) {
	if opts.Timeout <= 0 {
		return nil, errors.New("browser startup timeout must be greater than zero")
	}
	if opts.ProxyURL != "" && (opts.ProxyUser == "" || opts.ProxySecret == "") {
		return nil, errors.New("browser proxy credentials are required")
	}
	if opts.ProxyURL == "" && (opts.ProxyUser != "" || opts.ProxySecret != "") {
		return nil, errors.New("browser proxy URL is required for proxy credentials")
	}
	startCtx, cancelStart := context.WithTimeout(ctx, opts.Timeout)
	defer cancelStart()

	var allocatorCtx context.Context
	var cancelAllocator context.CancelFunc
	var process *browserProcess
	switch opts.Mode {
	case config.BrowserProfileManagedEphemeral, config.BrowserProfileManagedPersistent:
		if strings.TrimSpace(opts.Executable) == "" {
			return nil, errors.New("browser executable is required")
		}
		if strings.TrimSpace(opts.DataDir) == "" {
			return nil, errors.New("browser data directory is required")
		}
		process = newBrowserProcess()
		allocatorOptions := append(append([]chromedp.ExecAllocatorOption(nil), chromedp.DefaultExecAllocatorOptions[:]...),
			chromedp.ExecPath(opts.Executable),
			chromedp.UserDataDir(opts.DataDir),
			chromedp.Flag("no-first-run", true),
			chromedp.Flag("no-default-browser-check", true),
			chromedp.Flag("disable-background-networking", true),
			chromedp.Flag("disable-component-update", true),
			chromedp.Flag("disable-sync", true),
			chromedp.Flag("disable-quic", true),
			chromedp.Flag("deny-permission-prompts", true),
			chromedp.Flag("force-webrtc-ip-handling-policy", "disable_non_proxied_udp"),
			chromedp.Flag("remote-debugging-address", "127.0.0.1"),
			chromedp.ModifyCmdFunc(func(command *exec.Cmd) {
				process.configure(command)
			}),
			chromedp.WSURLReadTimeout(opts.Timeout),
		)
		if opts.ProxyURL != "" {
			allocatorOptions = append(allocatorOptions,
				chromedp.ProxyServer(opts.ProxyURL),
				chromedp.Flag("proxy-bypass-list", "<-loopback>"),
			)
		}
		allocatorCtx, cancelAllocator = chromedp.NewExecAllocator(context.Background(), allocatorOptions...)
	case config.BrowserProfileRemoteCDP, config.BrowserProfileExistingSession:
		if strings.TrimSpace(opts.CDPEndpoint) == "" {
			return nil, errors.New("browser CDP endpoint is required")
		}
		allocatorCtx, cancelAllocator = chromedp.NewRemoteAllocator(context.Background(), opts.CDPEndpoint)
	default:
		return nil, errors.New("browser profile mode is invalid")
	}

	browserCtx, cancelContext, cancelBootstrap, err := prepareInitialBrowserContext(startCtx, allocatorCtx, opts)
	if err != nil {
		cancelAllocator()
		return nil, err
	}
	if isAttachedProfile(opts.Mode) {
		cancelContext = getAttachedContextCancel(browserCtx, cancelContext)
	}
	session := &chromiumSession{
		ctx: browserCtx, cancelContext: cancelContext, cancelBootstrap: cancelBootstrap,
		cancelAllocator: cancelAllocator, process: process,
		tabContexts: make(map[string]context.Context), tabCancels: make(map[string]context.CancelFunc),
		networkAuthorizers: make(map[string]networkAuthorization), openingTabIDs: make(map[string]struct{}),
		networkErrors: make(map[string]error), networkActivity: make(map[string]*networkActivity),
		consoleMessages: make(map[string][]ConsoleMessage),
		dialogResponses: make(map[string]dialogResponse), downloadEvents: make(chan any, 4),
		proxyUser: opts.ProxyUser, proxySecret: opts.ProxySecret,
		downloadRoot:    opts.DownloadRoot,
		attachmentScope: opts.AttachmentScope, browserContextID: opts.BrowserContextID,
		attachmentTargets:  make(map[string]struct{}, len(opts.TargetIDs)),
		quarantinedTargets: make(map[string]struct{}), attached: isAttachedProfile(opts.Mode),
	}
	for _, id := range opts.TargetIDs {
		session.attachmentTargets[id] = struct{}{}
	}
	actions := []chromedp.Action{
		network.Enable(),
		network.SetBypassServiceWorker(true),
		network.SetBlockedURLs().WithURLPatterns(getBlockedWebSocketPatterns()),
		page.Enable(),
		installBrowserNetworkGuardAction(),
		browser.SetDownloadBehavior(browser.SetDownloadBehaviorBehaviorDeny),
		chromedp.ActionFunc(func(actionCtx context.Context) error {
			_, _, _, _, _, err := browser.GetVersion().Do(actionCtx)
			return err
		}),
	}
	chromedp.ListenTarget(browserCtx, session.getRequestListener(browserCtx, ""))
	actions = append([]chromedp.Action{fetch.Enable().WithHandleAuthRequests(opts.ProxyUser != "")}, actions...)
	ready := make(chan error, 1)
	go func() {
		ready <- chromedp.Run(browserCtx, actions...)
	}()
	select {
	case err := <-ready:
		if err != nil {
			_ = session.Close(context.Background())
			if opts.Mode == config.BrowserProfileRemoteCDP || opts.Mode == config.BrowserProfileExistingSession {
				return nil, errors.New("browser CDP connection failed")
			}
			return nil, err
		}
		if process != nil {
			if err := process.attach(); err != nil {
				_ = session.Close(context.Background())
				return nil, err
			}
		}
		chromedp.ListenBrowser(browserCtx, session.getUnexpectedTargetListener(browserCtx))
		chromedp.ListenBrowser(browserCtx, session.getDownloadListener())
		return session, nil
	case <-startCtx.Done():
		_ = session.Close(context.Background())
		return nil, startCtx.Err()
	}
}

func getBlockedWebSocketPatterns() []*network.BlockPattern {
	return []*network.BlockPattern{
		{URLPattern: "ws://*:*/*", Block: true},
		{URLPattern: "wss://*:*/*", Block: true},
	}
}

func prepareInitialBrowserContext(
	ctx context.Context,
	allocatorCtx context.Context,
	opts LaunchOptions,
) (context.Context, context.CancelFunc, context.CancelFunc, error) {
	switch opts.AttachmentScope {
	case config.BrowserAttachmentTargets:
		if len(opts.TargetIDs) == 0 {
			return nil, nil, nil, errors.New("target-scoped browser attachment requires a target ID")
		}
		browserCtx, cancel := chromedp.NewContext(
			allocatorCtx, chromedp.WithTargetID(target.ID(opts.TargetIDs[0])),
		)
		return browserCtx, cancel, nil, nil
	case config.BrowserAttachmentContext, config.BrowserAttachmentBrowser:
		return prepareScopedBrowserContext(ctx, allocatorCtx, opts)
	default:
		browserCtx, cancel := chromedp.NewContext(allocatorCtx)
		return browserCtx, cancel, nil, nil
	}
}

func prepareScopedBrowserContext(
	ctx context.Context,
	allocatorCtx context.Context,
	opts LaunchOptions,
) (context.Context, context.CancelFunc, context.CancelFunc, error) {
	bootstrapCtx, cancelBootstrap := chromedp.NewContext(allocatorCtx)
	stop := context.AfterFunc(ctx, cancelBootstrap)
	defer stop()
	chromiumCtx := chromedp.FromContext(bootstrapCtx)
	if chromiumCtx == nil || chromiumCtx.Allocator == nil {
		cancelBootstrap()
		return nil, nil, nil, errors.New("browser connection is unavailable")
	}
	if chromiumCtx.Browser == nil {
		browser, err := chromiumCtx.Allocator.Allocate(bootstrapCtx)
		if err != nil {
			cancelBootstrap()
			return nil, nil, nil, err
		}
		chromiumCtx.Browser = browser
	}
	infos, err := target.GetTargets().Do(cdp.WithExecutor(bootstrapCtx, chromiumCtx.Browser))
	if err != nil {
		cancelBootstrap()
		return nil, nil, nil, err
	}
	selected := getAttachmentTarget(infos, opts.AttachmentScope, opts.BrowserContextID)
	if selected == "" {
		cancelBootstrap()
		return nil, nil, nil, errors.New("configured browser attachment has no page target")
	}
	browserCtx, cancel := chromedp.NewContext(bootstrapCtx, chromedp.WithTargetID(selected))
	return browserCtx, cancel, cancelBootstrap, nil
}

func getAttachmentTarget(infos []*target.Info, scope string, browserContextID string) target.ID {
	var selected *target.Info
	for _, info := range infos {
		if info == nil || info.Type != "page" || info.Subtype != "" {
			continue
		}
		if scope != config.BrowserAttachmentBrowser &&
			(scope != config.BrowserAttachmentContext || string(info.BrowserContextID) != browserContextID) {
			continue
		}
		if selected == nil || isPreferredAttachmentTarget(info, selected) {
			selected = info
		}
	}
	if selected == nil {
		return ""
	}
	return selected.TargetID
}

func isPreferredAttachmentTarget(candidate, selected *target.Info) bool {
	candidateBlank := candidate.URL == "about:blank"
	selectedBlank := selected.URL == "about:blank"
	if candidateBlank != selectedBlank {
		return candidateBlank
	}
	return string(candidate.TargetID) < string(selected.TargetID)
}

func getAttachedContextCancel(ctx context.Context, cancel context.CancelFunc) context.CancelFunc {
	return func() {
		// chromedp cancellation closes a populated Target. Reverify this detach behavior before upgrading chromedp.
		if chromiumCtx := chromedp.FromContext(ctx); chromiumCtx != nil {
			chromiumCtx.Target = nil
		}
		cancel()
	}
}

func installBrowserNetworkGuardAction() chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		_, err := page.AddScriptToEvaluateOnNewDocument(browserNetworkGuardScript).Do(ctx)
		return err
	})
}

func (s *chromiumSession) getDownloadListener() func(any) {
	return func(event any) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if !s.downloadArmed {
			return
		}
		switch value := event.(type) {
		case *browser.EventDownloadWillBegin:
			if _, owned := s.downloadFrameIDs[value.FrameID]; !owned || s.downloadGUID != "" {
				return
			}
			s.downloadGUID = value.GUID
		case *browser.EventDownloadProgress:
			if value.GUID != s.downloadGUID {
				return
			}
			if value.State == browser.DownloadProgressStateInProgress {
				overLimit := value.ReceivedBytes > float64(s.downloadMaxBytes) ||
					value.TotalBytes > float64(s.downloadMaxBytes)
				if !overLimit || s.downloadLimitSent {
					return
				}
				s.downloadLimitSent = true
			}
		default:
			return
		}
		select {
		case s.downloadEvents <- event:
		default:
		}
	}
}

func (s *chromiumSession) getUnexpectedTargetListener(ctx context.Context) func(any) {
	return func(event any) {
		if destroyed, ok := event.(*target.EventTargetDestroyed); ok {
			s.mu.Lock()
			delete(s.quarantinedTargets, string(destroyed.TargetID))
			s.mu.Unlock()
			return
		}
		created, ok := event.(*target.EventTargetCreated)
		if !ok || created.TargetInfo == nil || created.TargetInfo.Type != "page" {
			return
		}
		s.mu.Lock()
		if s.attached {
			if _, opening := s.openingTabIDs[string(created.TargetInfo.TargetID)]; opening {
				s.mu.Unlock()
				return
			}
			s.quarantinedTargets[string(created.TargetInfo.TargetID)] = struct{}{}
			s.mu.Unlock()
			return
		}
		if s.openingTargets > 0 {
			s.openingTargets--
			s.mu.Unlock()
			return
		}
		s.mu.Unlock()
		go func() {
			chromiumCtx := chromedp.FromContext(s.ctx)
			if chromiumCtx != nil && chromiumCtx.Browser != nil {
				_ = target.CloseTarget(created.TargetInfo.TargetID).Do(cdp.WithExecutor(ctx, chromiumCtx.Browser))
			}
		}()
	}
}

func (s *chromiumSession) getRequestListener(ctx context.Context, tabID string) func(any) {
	return func(event any) {
		switch value := event.(type) {
		case *fetch.EventRequestPaused:
			s.markNetworkRequestStarted(tabID)
			go func() {
				defer s.markNetworkRequestFinished(tabID)
				requestClass := permissions.NetworkRequestSubresource
				if value.RedirectedRequestID != "" {
					requestClass = permissions.NetworkRequestRedirect
				} else if value.ResourceType == network.ResourceTypeDocument {
					requestClass = permissions.NetworkRequestNavigation
				}
				target, err := permissions.NetworkTargetFromURL(value.Request.URL, value.Request.Method, requestClass)
				if err != nil {
					log.Warn().
						Str("browser_tab_id", tabID).
						Str("network_method", value.Request.Method).
						Str("network_resource_type", string(value.ResourceType)).
						Str("error", getSafeBrowserNetworkError(err)).
						Msg("Browser intercepted an invalid network request")
					_ = chromedp.Run(ctx, fetch.FailRequest(value.RequestID, network.ErrorReasonBlockedByClient))
					return
				}
				addBrowserNetworkLogFields(log.Debug(), target).
					Str("browser_tab_id", tabID).
					Str("network_resource_type", string(value.ResourceType)).
					Msg("Browser network request intercepted")
				authorization, ok := s.getNetworkAuthorization(tabID)
				if !ok {
					addBrowserNetworkLogFields(log.Warn(), target).
						Str("browser_tab_id", tabID).
						Str("network_resource_type", string(value.ResourceType)).
						Msg("Browser network request had no active authorizer")
					_ = chromedp.Run(ctx, fetch.FailRequest(value.RequestID, network.ErrorReasonBlockedByClient))
					return
				}
				err = authorization.authorize(authorization.ctx, target)
				if err == nil {
					err = authorization.ctx.Err()
				}
				if err != nil {
					level := log.Warn()
					message := "Browser network request authorization failed"
					if errors.Is(err, context.Canceled) {
						level = log.Debug()
						message = "Browser network request authorization was cancelled"
					}
					addBrowserNetworkLogFields(level, target).
						Str("browser_tab_id", tabID).
						Uint64("browser_network_authorization_id", authorization.id).
						Str("network_resource_type", string(value.ResourceType)).
						Str("error", getSafeBrowserNetworkError(err)).
						Msg(message)
					s.recordNetworkError(tabID, authorization.id, err)
					_ = chromedp.Run(ctx, fetch.FailRequest(value.RequestID, network.ErrorReasonBlockedByClient))
					return
				}
				if err := s.continueAuthorizedRequest(ctx, tabID, authorization.id, value.RequestID); err != nil {
					addBrowserNetworkLogFields(log.Warn(), target).
						Str("browser_tab_id", tabID).
						Uint64("browser_network_authorization_id", authorization.id).
						Str("network_resource_type", string(value.ResourceType)).
						Str("error", getSafeBrowserNetworkError(err)).
						Msg("Browser failed to continue an authorized network request")
					_ = chromedp.Run(ctx, fetch.FailRequest(value.RequestID, network.ErrorReasonBlockedByClient))
					return
				}
				addBrowserNetworkLogFields(log.Debug(), target).
					Str("browser_tab_id", tabID).
					Uint64("browser_network_authorization_id", authorization.id).
					Str("network_resource_type", string(value.ResourceType)).
					Msg("Browser continued an authorized network request")
			}()
		case *fetch.EventAuthRequired:
			response := &fetch.AuthChallengeResponse{Response: fetch.AuthChallengeResponseResponseCancelAuth}
			if value.AuthChallenge != nil && value.AuthChallenge.Source == fetch.AuthChallengeSourceProxy {
				response.Response = fetch.AuthChallengeResponseResponseProvideCredentials
				response.Username = s.proxyUser
				response.Password = s.proxySecret
			}
			go func() {
				_ = chromedp.Run(ctx, fetch.ContinueWithAuth(value.RequestID, response))
			}()
		}
	}
}

func (s *chromiumSession) getNetworkAuthorization(tabID string) (networkAuthorization, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getNetworkAuthorizationLocked(tabID)
}

func (s *chromiumSession) getNetworkAuthorizationLocked(tabID string) (networkAuthorization, bool) {
	authorization, ok := s.networkAuthorizers[tabID]
	if !ok {
		if _, opening := s.openingTabIDs[tabID]; opening {
			authorization, ok = s.networkAuthorizers[""]
		}
	}
	if !ok {
		authorization, ok = s.networkAuthorizers["*"]
	}
	return authorization, ok
}

func (s *chromiumSession) continueAuthorizedRequest(
	ctx context.Context,
	tabID string,
	authorizationID uint64,
	requestID fetch.RequestID,
) error {
	s.mu.Lock()
	authorization, ok := s.getNetworkAuthorizationLocked(tabID)
	if !ok || authorization.id != authorizationID || authorization.ctx.Err() != nil {
		s.mu.Unlock()
		return context.Canceled
	}
	authorizationCtx := authorization.ctx
	s.mu.Unlock()

	requestCtx, done := newBoundedActionContext(ctx, authorizationCtx)
	defer done()
	return chromedp.Run(requestCtx, fetch.ContinueRequest(requestID))
}

func (s *chromiumSession) SetNetworkAuthorizer(tabID string, authorize NetworkRequestAuthorizer) func() {
	s.mu.Lock()
	if s.networkAuthorizers == nil {
		s.networkAuthorizers = make(map[string]networkAuthorization)
	}
	if s.networkErrors == nil {
		s.networkErrors = make(map[string]error)
	}
	previous := s.networkAuthorizers[tabID]
	parent := s.ctx
	if parent == nil {
		parent = context.Background()
	}
	authorizationCtx, cancel := context.WithCancel(parent)
	s.nextAuthorization++
	authorization := networkAuthorization{
		id: s.nextAuthorization, ctx: authorizationCtx, cancel: cancel, authorize: authorize,
	}
	s.networkAuthorizers[tabID] = authorization
	delete(s.networkErrors, tabID)
	s.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			authorization.cancel()
			s.mu.Lock()
			current, ok := s.networkAuthorizers[tabID]
			if ok && current.id == authorization.id {
				if previous.authorize == nil {
					delete(s.networkAuthorizers, tabID)
				} else {
					s.networkAuthorizers[tabID] = previous
				}
			}
			s.mu.Unlock()
		})
	}
}

func (s *chromiumSession) consumeNetworkError(tabID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.networkErrors[tabID]
	delete(s.networkErrors, tabID)
	return err
}

func (s *chromiumSession) recordNetworkError(tabID string, authorizationID uint64, err error) {
	if err == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	authorization, ok := s.getNetworkAuthorizationLocked(tabID)
	if !ok || authorization.id != authorizationID || authorization.ctx.Err() != nil {
		return
	}
	if s.networkErrors == nil {
		s.networkErrors = make(map[string]error)
	}
	if s.networkErrors[tabID] == nil {
		s.networkErrors[tabID] = err
	}
}

func (s *chromiumSession) WaitForNetworkIdle(ctx context.Context, tabID string, quietPeriod time.Duration) error {
	if quietPeriod <= 0 {
		return nil
	}
	for {
		s.mu.Lock()
		activity := s.getNetworkActivityLocked(tabID)
		pending := activity.pending
		quietFor := time.Since(activity.last)
		changed := activity.changed
		s.mu.Unlock()
		if pending == 0 && quietFor >= quietPeriod {
			return nil
		}

		wait := quietPeriod - quietFor
		if wait <= 0 || pending > 0 {
			wait = quietPeriod
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return context.Cause(ctx)
		case <-changed:
			timer.Stop()
		case <-timer.C:
		}
	}
}

func (s *chromiumSession) markNetworkRequestStarted(tabID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	activity := s.getNetworkActivityLocked(tabID)
	activity.pending++
	s.signalNetworkActivityLocked(activity)
}

func (s *chromiumSession) markNetworkRequestFinished(tabID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	activity := s.getNetworkActivityLocked(tabID)
	if activity.pending > 0 {
		activity.pending--
	}
	s.signalNetworkActivityLocked(activity)
}

func (s *chromiumSession) getNetworkActivityLocked(tabID string) *networkActivity {
	if s.networkActivity == nil {
		s.networkActivity = make(map[string]*networkActivity)
	}
	activity := s.networkActivity[tabID]
	if activity == nil {
		activity = &networkActivity{last: time.Now(), changed: make(chan struct{})}
		s.networkActivity[tabID] = activity
	}
	return activity
}

func (s *chromiumSession) signalNetworkActivityLocked(activity *networkActivity) {
	activity.last = time.Now()
	close(activity.changed)
	activity.changed = make(chan struct{})
}

func (s *chromiumSession) Health(ctx context.Context) error {
	if s == nil || s.ctx == nil {
		return errors.New("browser session is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	healthCtx, cancel := context.WithCancel(s.ctx)
	stop := context.AfterFunc(ctx, cancel)
	defer stop()
	defer cancel()

	return chromedp.Run(healthCtx, chromedp.ActionFunc(func(actionCtx context.Context) error {
		_, _, _, _, _, err := browser.GetVersion().Do(actionCtx)
		return err
	}))
}

func (s *chromiumSession) Close(context.Context) error {
	if s == nil {
		return nil
	}
	s.once.Do(func() {
		if s.process != nil {
			s.closeErr = s.process.stop()
		}
		s.preserveAttachedTargets()
		if s.cancelContext != nil {
			s.cancelContext()
		}
		if s.cancelBootstrap != nil {
			s.cancelBootstrap()
		}
		if s.cancelAllocator != nil {
			s.cancelAllocator()
		}
	})

	return s.closeErr
}

func (s *chromiumSession) preserveAttachedTargets() {
	if !s.attached {
		return
	}
	s.mu.Lock()
	contexts := make([]context.Context, 0, len(s.tabContexts)+1)
	contexts = append(contexts, s.ctx)
	for _, tabCtx := range s.tabContexts {
		contexts = append(contexts, tabCtx)
	}
	s.mu.Unlock()
	for _, ctx := range contexts {
		// Keep attached tabs open when Morph releases its contexts. This depends on chromedp cancellation semantics.
		if chromiumCtx := chromedp.FromContext(ctx); chromiumCtx != nil {
			chromiumCtx.Target = nil
		}
	}
}
