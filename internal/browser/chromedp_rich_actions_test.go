package browser

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cdpbrowser "github.com/chromedp/cdproto/browser"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/page"
	cdpruntime "github.com/chromedp/cdproto/runtime"
	"github.com/stretchr/testify/require"
)

func TestConsoleListener_BoundsSanitizesAndClassifiesMessages(t *testing.T) {
	session := &chromiumSession{consoleMessages: make(map[string][]ConsoleMessage)}
	listener := session.getConsoleListener("tab")
	secret, err := json.Marshal("authorization=Bearer-secret\u001b")
	require.NoError(t, err)
	for range maxConsoleMessages + 5 {
		listener(&cdpruntime.EventConsoleAPICalled{
			Type: cdpruntime.APITypeWarning,
			Args: []*cdpruntime.RemoteObject{{Value: secret}},
		})
	}
	listener(&cdpruntime.EventExceptionThrown{ExceptionDetails: &cdpruntime.ExceptionDetails{
		Text: "password=hunter2", Exception: &cdpruntime.RemoteObject{Description: "boom"},
	}})
	session.mu.Lock()
	messages := append([]ConsoleMessage(nil), session.consoleMessages["tab"]...)
	session.mu.Unlock()
	require.Len(t, messages, maxConsoleMessages)
	require.Equal(t, ConsoleError, messages[len(messages)-1].Level)
	require.Equal(t, "password=[redacted] boom", messages[len(messages)-1].Text)
	require.NotContains(t, messages[0].Text, "Bearer-secret")
	require.NotContains(t, messages[0].Text, "\u001b")
	require.Equal(
		t,
		`{"token":[redacted]} Bearer [redacted] red`,
		sanitizeConsoleText(`{"token":"value"} Bearer credential `+"\x1b[31mred\x1b[0m"),
	)

	listener(&cdpruntime.EventExceptionThrown{})
	listener(struct{}{})
	require.Equal(t, ConsoleDebug, getConsoleLevel(cdpruntime.APITypeDebug))
	require.Equal(t, ConsoleError, getConsoleLevel(cdpruntime.APITypeAssert))
	require.Equal(t, ConsoleInfo, getConsoleLevel(cdpruntime.APITypeLog))
	require.Equal(t, "NaN", getRemoteObjectText(&cdpruntime.RemoteObject{UnserializableValue: "NaN"}))
	require.Empty(t, getRemoteObjectText(nil))
	require.Len(t, sanitizeConsoleText(strings.Repeat("x", maxConsoleText+5)), maxConsoleText)
}

func TestGetDialogConsoleMessage_ReportsSanitizedTypeMessageAndResolution(t *testing.T) {
	event := &page.EventJavascriptDialogOpening{
		Type: page.DialogTypePrompt, Message: "Token=secret", DefaultPrompt: "https://user:pass@example.com/?key=value",
	}
	automatic := getDialogConsoleMessage(event, false, false)
	require.Equal(t, ConsoleWarn, automatic.Level)
	require.Contains(t, automatic.Text, "automatically dismissed prompt dialog")
	require.NotContains(t, automatic.Text, "secret")
	require.NotContains(t, automatic.Text, "user:pass")
	require.NotContains(t, automatic.Text, "key=value")
	require.Contains(t, getDialogConsoleMessage(event, true, true).Text, "accepted prompt dialog")
	require.Contains(t, getDialogConsoleMessage(event, true, false).Text, "dismissed prompt dialog")
}

func TestReadDownloadedArtifact_ValidatesBoundedStableRegularFiles(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "guid"), []byte("data"), 0o600))
	artifact, err := readDownloadedArtifact(root, "guid", "../report.txt", "https://example.com/file", 4)
	require.NoError(t, err)
	require.Equal(t, ArtifactDownload, artifact.Kind)
	require.Equal(t, []byte("data"), artifact.Data)
	_, err = readDownloadedArtifact(root, "guid", "report.txt", "", 3)
	require.EqualError(t, err, "browser downloaded artifact is invalid")
	require.NoError(t, os.Mkdir(filepath.Join(root, "directory"), 0o700))
	_, err = readDownloadedArtifact(root, "directory", "report.txt", "", 10)
	require.EqualError(t, err, "browser downloaded artifact is invalid")
	_, err = readDownloadedArtifact(root, "missing", "report.txt", "", 10)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestDownloadListener_ForwardsOnlyArmedDownloadEvents(t *testing.T) {
	frameIDs := make(map[cdp.FrameID]struct{})
	collectFrameIDs(&page.FrameTree{
		Frame:       &cdp.Frame{ID: "expected"},
		ChildFrames: []*page.FrameTree{{Frame: &cdp.Frame{ID: "child"}}},
	}, frameIDs)
	session := &chromiumSession{
		downloadEvents: make(chan any, 1), downloadFrameIDs: frameIDs,
	}
	listener := session.getDownloadListener()
	listener(&cdpbrowser.EventDownloadWillBegin{GUID: "ignored"})
	require.Empty(t, session.downloadEvents)
	session.downloadArmed = true
	listener(struct{}{})
	require.Empty(t, session.downloadEvents)
	listener(&cdpbrowser.EventDownloadWillBegin{GUID: "wrong-frame", FrameID: "other"})
	require.Empty(t, session.downloadEvents)
	event := &cdpbrowser.EventDownloadWillBegin{GUID: "accepted", FrameID: "child"}
	listener(event)
	require.Same(t, event, <-session.downloadEvents)
	session.downloadEvents <- event
	listener(&cdpbrowser.EventDownloadProgress{GUID: "dropped"})
	require.Len(t, session.downloadEvents, 1)
	<-session.downloadEvents
	session.downloadMaxBytes = 10
	listener(&cdpbrowser.EventDownloadProgress{
		GUID: "accepted", State: cdpbrowser.DownloadProgressStateInProgress, ReceivedBytes: 1, TotalBytes: 2,
	})
	require.Empty(t, session.downloadEvents)
	overLimit := &cdpbrowser.EventDownloadProgress{
		GUID: "accepted", State: cdpbrowser.DownloadProgressStateInProgress, ReceivedBytes: 11, TotalBytes: 20,
	}
	listener(overLimit)
	listener(overLimit)
	require.Same(t, overLimit, <-session.downloadEvents)
	completed := &cdpbrowser.EventDownloadProgress{
		GUID: "accepted", State: cdpbrowser.DownloadProgressStateCompleted, ReceivedBytes: 2, TotalBytes: 2,
	}
	listener(completed)
	require.Same(t, completed, <-session.downloadEvents)
}

func TestRichBackendActions_RejectInvalidLocalInputsWithoutBrowserSideEffects(t *testing.T) {
	session := &chromiumSession{consoleMessages: make(map[string][]ConsoleMessage)}
	_, err := session.Console(context.Background(), "tab", -1)
	require.EqualError(t, err, "browser console limit must be between 1 and 200")
	err = session.Upload(context.Background(), "tab", 1, "relative")
	require.EqualError(t, err, "browser staged upload path must be absolute")
	_, err = session.Download(context.Background(), "tab", 1, 0)
	require.EqualError(t, err, "browser download size limit must be greater than zero")
	_, err = session.Download(context.Background(), "tab", 1, 1)
	require.EqualError(t, err, "browser download root is unavailable")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	session.dialogResponses = map[string]dialogResponse{
		"tab": {result: make(chan error, 1)},
	}
	err = session.RespondToDialog(ctx, "tab", 1, true, "")
	require.EqualError(t, err, "browser dialog response is already armed")

	session.getConsoleListener("tab")(&cdpruntime.EventConsoleAPICalled{
		Type: cdpruntime.APITypeLog,
		Args: []*cdpruntime.RemoteObject{{Value: []byte(`invalid`)}, {Description: "fallback"}},
	})
	require.Equal(t, "fallback", session.consoleMessages["tab"][0].Text)
}
