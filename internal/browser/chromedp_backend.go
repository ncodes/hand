package browser

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"sync"

	"github.com/chromedp/cdproto/browser"
	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/chromedp"
	"github.com/wandxy/morph/internal/config"
)

type ChromiumBackend struct{}

type chromiumSession struct {
	ctx             context.Context
	cancelContext   context.CancelFunc
	cancelAllocator context.CancelFunc
	process         *browserProcess
	once            sync.Once
	closeErr        error
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

	browserCtx, cancelContext := chromedp.NewContext(allocatorCtx)
	session := &chromiumSession{
		ctx: browserCtx, cancelContext: cancelContext, cancelAllocator: cancelAllocator, process: process,
	}
	actions := []chromedp.Action{chromedp.ActionFunc(func(actionCtx context.Context) error {
		_, _, _, _, _, err := browser.GetVersion().Do(actionCtx)
		return err
	})}
	if opts.ProxyUser != "" {
		chromedp.ListenTarget(browserCtx, getProxyAuthorizationListener(browserCtx, opts.ProxyUser, opts.ProxySecret))
		actions = append([]chromedp.Action{fetch.Enable().WithHandleAuthRequests(true)}, actions...)
	}
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
		return session, nil
	case <-startCtx.Done():
		_ = session.Close(context.Background())
		return nil, startCtx.Err()
	}
}

func getProxyAuthorizationListener(ctx context.Context, username, password string) func(any) {
	return func(event any) {
		switch value := event.(type) {
		case *fetch.EventRequestPaused:
			go func() {
				_ = chromedp.Run(ctx, fetch.ContinueRequest(value.RequestID))
			}()
		case *fetch.EventAuthRequired:
			response := &fetch.AuthChallengeResponse{Response: fetch.AuthChallengeResponseResponseCancelAuth}
			if value.AuthChallenge != nil && value.AuthChallenge.Source == fetch.AuthChallengeSourceProxy {
				response.Response = fetch.AuthChallengeResponseResponseProvideCredentials
				response.Username = username
				response.Password = password
			}
			go func() {
				_ = chromedp.Run(ctx, fetch.ContinueWithAuth(value.RequestID, response))
			}()
		}
	}
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
		if s.cancelContext != nil {
			s.cancelContext()
		}
		if s.cancelAllocator != nil {
			s.cancelAllocator()
		}
		if s.process != nil {
			s.closeErr = s.process.stop()
		}
	})

	return s.closeErr
}
