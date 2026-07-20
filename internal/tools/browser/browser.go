package browser

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"

	browserdomain "github.com/wandxy/morph/internal/browser"
	envtypes "github.com/wandxy/morph/internal/environment/types"
	"github.com/wandxy/morph/internal/permissions"
	"github.com/wandxy/morph/internal/tools"
)

const toolName = "browser"

func Definition(runtime envtypes.Runtime) tools.Definition {
	return tools.Definition{
		Name:         toolName,
		Description:  "Operate an isolated, permission-aware browser using typed lifecycle, navigation, interaction, artifact, file-transfer, and dialog actions.",
		InputSchema:  inputSchema(),
		Groups:       []string{"core"},
		Requires:     tools.Capabilities{Browser: true},
		ParallelSafe: false,
		ResolvePermission: func(ctx context.Context, call tools.Call) ([]permissions.EvaluationInput, error) {
			request, err := decodeRequest(call.Input)
			if err != nil {
				return nil, err
			}
			request, err = prepareRequest(runtime, request)
			if err != nil {
				return nil, err
			}
			service, err := getService(ctx, runtime)
			if err != nil {
				return nil, err
			}
			return service.ResolvePermissionInputs(ctx, request.Action, actionRequestFromRequest(request))
		},
		Handler: tools.HandlerFunc(func(ctx context.Context, call tools.Call) (tools.Result, error) {
			request, err := decodeRequest(call.Input)
			if err != nil {
				return tools.Result{}, err
			}
			request, err = prepareRequest(runtime, request)
			if err != nil {
				return tools.Result{}, err
			}
			service, err := getService(ctx, runtime)
			if err != nil {
				return tools.Result{}, err
			}
			result, err := dispatch(ctx, service, request)
			if err != nil {
				return tools.Result{Error: getToolError(err).String()}, nil
			}
			raw, err := json.Marshal(result)
			if err != nil {
				return tools.Result{}, err
			}
			return tools.Result{Output: string(raw)}, nil
		}),
	}
}

func getService(ctx context.Context, runtime envtypes.Runtime) (envtypes.BrowserService, error) {
	if runtime == nil {
		return nil, tools.NewPermissionResolutionError("browser_unavailable", "browser runtime is unavailable")
	}
	service, ok, err := runtime.BrowserService(ctx)
	if err != nil {
		return nil, err
	}
	if !ok || service == nil {
		return nil, tools.NewPermissionResolutionError("browser_unavailable", "browser service is unavailable")
	}
	return service, nil
}

func getToolError(err error) tools.Error {
	if decisionErr, ok := permissions.GetDecisionError(err); ok {
		return tools.Error{Code: decisionErr.Code, Message: decisionErr.Error()}
	}
	var browserErr *browserdomain.Error
	if errors.As(err, &browserErr) {
		return tools.Error{
			Code: string(browserErr.Code), Message: getSafeBrowserErrorMessage(browserErr.Code),
			Retryable: browserErr.Retryable,
		}
	}
	return tools.Error{Code: "browser_failed", Message: "browser operation failed"}
}

func getSafeBrowserErrorMessage(code browserdomain.ErrorCode) string {
	switch code {
	case browserdomain.ErrorInvalidRequest:
		return "browser request is invalid"
	case browserdomain.ErrorUnavailable:
		return "browser is unavailable"
	case browserdomain.ErrorStartFailed:
		return "browser failed to start"
	case browserdomain.ErrorHealthFailed:
		return "browser health check failed"
	case browserdomain.ErrorNotFound:
		return "browser resource was not found"
	case browserdomain.ErrorOwnership:
		return "browser resource belongs to another owner"
	case browserdomain.ErrorClosed:
		return "browser session is closed"
	case browserdomain.ErrorNotReady:
		return "browser session is not ready"
	case browserdomain.ErrorStaleReference:
		return "browser element reference is stale"
	case browserdomain.ErrorTimeout:
		return "browser operation timed out"
	default:
		return "browser operation failed"
	}
}

func dispatch(ctx context.Context, service envtypes.BrowserService, request request) (any, error) {
	dispatcher, ok := actionDispatchers[request.Action]
	if !ok {
		return nil, errors.New("browser action is not supported")
	}
	return dispatcher(ctx, service, request)
}

type actionDispatcher func(context.Context, envtypes.BrowserService, request) (any, error)

var actionDispatchers = map[browserdomain.Action]actionDispatcher{
	browserdomain.ActionStatus: func(_ context.Context, service envtypes.BrowserService, _ request) (any, error) {
		return getSafeStatus(service.Status()), nil
	},
	browserdomain.ActionProfiles: func(_ context.Context, service envtypes.BrowserService, _ request) (any, error) {
		return service.Status().Profiles, nil
	},
	browserdomain.ActionStart: func(ctx context.Context, service envtypes.BrowserService, value request) (any, error) {
		session, err := service.Start(ctx, browserdomain.StartRequest{Profile: value.Profile})
		return getSafeSession(session), err
	},
	browserdomain.ActionStop: func(ctx context.Context, service envtypes.BrowserService, value request) (any, error) {
		session, err := service.Stop(ctx, value.SessionID)
		return getSafeSession(session), err
	},
	browserdomain.ActionTabs: func(ctx context.Context, service envtypes.BrowserService, value request) (any, error) {
		tabs, err := service.Tabs(ctx, value.SessionID)
		return getSafeTabs(tabs), err
	},
	browserdomain.ActionOpen:     getActionRequestDispatcher(envtypes.BrowserService.Open),
	browserdomain.ActionFocus:    getActionRequestDispatcher(envtypes.BrowserService.Focus),
	browserdomain.ActionClose:    getActionRequestDispatcher(envtypes.BrowserService.CloseTab),
	browserdomain.ActionNavigate: getActionRequestDispatcher(envtypes.BrowserService.Navigate),
	browserdomain.ActionReload:   getActionRequestDispatcher(envtypes.BrowserService.Reload),
	browserdomain.ActionSnapshot: func(ctx context.Context, service envtypes.BrowserService, value request) (any, error) {
		snapshot, err := service.Snapshot(ctx, actionRequestFromRequest(value))
		return getSafeSnapshot(snapshot), err
	},
	browserdomain.ActionScreenshot: func(ctx context.Context, service envtypes.BrowserService, value request) (any, error) {
		return service.Screenshot(ctx, actionRequestFromRequest(value))
	},
	browserdomain.ActionPDF: func(ctx context.Context, service envtypes.BrowserService, value request) (any, error) {
		return service.PDF(ctx, actionRequestFromRequest(value))
	},
	browserdomain.ActionConsole: func(ctx context.Context, service envtypes.BrowserService, value request) (any, error) {
		return service.Console(ctx, actionRequestFromRequest(value))
	},
	browserdomain.ActionClick:  getActionRequestDispatcher(envtypes.BrowserService.Click),
	browserdomain.ActionType:   getActionRequestDispatcher(envtypes.BrowserService.Type),
	browserdomain.ActionPress:  getActionRequestDispatcher(envtypes.BrowserService.Press),
	browserdomain.ActionScroll: getActionRequestDispatcher(envtypes.BrowserService.Scroll),
	browserdomain.ActionSelect: getActionRequestDispatcher(envtypes.BrowserService.Select),
	browserdomain.ActionUpload: getActionRequestDispatcher(envtypes.BrowserService.Upload),
	browserdomain.ActionDownload: func(ctx context.Context, service envtypes.BrowserService, value request) (any, error) {
		return service.Download(ctx, actionRequestFromRequest(value))
	},
	browserdomain.ActionAcceptDialog:  getActionRequestDispatcher(envtypes.BrowserService.AcceptDialog),
	browserdomain.ActionDismissDialog: getActionRequestDispatcher(envtypes.BrowserService.DismissDialog),
	browserdomain.ActionWait:          getActionRequestDispatcher(envtypes.BrowserService.Wait),
	browserdomain.ActionBack:          getActionRequestDispatcher(envtypes.BrowserService.Back),
	browserdomain.ActionForward:       getActionRequestDispatcher(envtypes.BrowserService.Forward),
}

func getActionRequestDispatcher(
	run func(envtypes.BrowserService, context.Context, browserdomain.ActionRequest) (browserdomain.Tab, error),
) actionDispatcher {
	return func(ctx context.Context, service envtypes.BrowserService, value request) (any, error) {
		tab, err := run(service, ctx, actionRequestFromRequest(value))
		return getSafeTab(tab), err
	}
}

func getSafeStatus(status browserdomain.Status) browserdomain.Status {
	status.Sessions = append([]browserdomain.Session(nil), status.Sessions...)
	for index := range status.Sessions {
		status.Sessions[index] = getSafeSession(status.Sessions[index])
	}
	return status
}

func getSafeSession(session browserdomain.Session) browserdomain.Session {
	session.Error = ""
	return session
}

func getSafeTabs(tabs []browserdomain.Tab) []browserdomain.Tab {
	for index := range tabs {
		tabs[index] = getSafeTab(tabs[index])
	}
	return tabs
}

func getSafeTab(tab browserdomain.Tab) browserdomain.Tab {
	tab.URL = getSafeURL(tab.URL)
	return tab
}

func getSafeSnapshot(snapshot browserdomain.Snapshot) browserdomain.Snapshot {
	snapshot.URL = getSafeURL(snapshot.URL)
	return snapshot
}

func getSafeURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if parsed.Hostname() == "" {
		if parsed.Scheme == "about" {
			return parsed.Scheme + ":" + parsed.Opaque
		}
		return ""
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}
