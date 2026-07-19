package browser

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	cdpbrowser "github.com/chromedp/cdproto/browser"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/cdproto/page"
	cdpruntime "github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

func (s *chromiumSession) Screenshot(ctx context.Context, tabID string, fullPage bool) (BackendArtifact, error) {
	var data []byte
	params := page.CaptureScreenshot().WithFormat(page.CaptureScreenshotFormatPng).WithFromSurface(true)
	if fullPage {
		params = params.WithCaptureBeyondViewport(true)
	}
	if err := s.runInTab(ctx, tabID, chromedp.ActionFunc(func(actionCtx context.Context) error {
		var err error
		data, err = params.Do(actionCtx)
		return err
	})); err != nil {
		return BackendArtifact{}, err
	}

	return BackendArtifact{
		Kind: ArtifactScreenshot, Name: "screenshot.png", MIMEType: "image/png", Data: data,
	}, nil
}

func (s *chromiumSession) PDF(ctx context.Context, tabID string) (BackendArtifact, error) {
	var data []byte
	if err := s.runInTab(ctx, tabID, chromedp.ActionFunc(func(actionCtx context.Context) error {
		var err error
		data, _, err = page.PrintToPDF().WithPrintBackground(true).Do(actionCtx)
		return err
	})); err != nil {
		return BackendArtifact{}, err
	}

	return BackendArtifact{Kind: ArtifactPDF, Name: "page.pdf", MIMEType: "application/pdf", Data: data}, nil
}

func (s *chromiumSession) Console(ctx context.Context, tabID string, limit int) ([]ConsoleMessage, error) {
	if limit == 0 {
		limit = defaultConsoleLimit
	}
	if limit < 1 || limit > maxConsoleMessages {
		return nil, errors.New("browser console limit must be between 1 and 200")
	}
	if err := s.runInTab(ctx, tabID); err != nil {
		return nil, err
	}
	s.mu.Lock()
	messages := append([]ConsoleMessage(nil), s.consoleMessages[tabID]...)
	s.mu.Unlock()
	if len(messages) > limit {
		messages = messages[len(messages)-limit:]
	}

	return messages, nil
}

func (s *chromiumSession) Upload(ctx context.Context, tabID string, backendNodeID int64, stagedPath string) error {
	if !filepath.IsAbs(stagedPath) {
		return errors.New("browser staged upload path must be absolute")
	}
	return s.runOnNode(ctx, tabID, backendNodeID, func(nodeIDs []cdp.NodeID) chromedp.Action {
		return chromedp.ActionFunc(func(actionCtx context.Context) error {
			return dom.SetFileInputFiles([]string{stagedPath}).WithNodeID(nodeIDs[0]).Do(actionCtx)
		})
	})
}

func (s *chromiumSession) RespondToDialog(
	ctx context.Context,
	tabID string,
	backendNodeID int64,
	accept bool,
	promptText string,
) error {
	actionCtx, done := s.newActionContext(ctx)
	defer done()
	response := dialogResponse{accept: accept, promptText: promptText, result: make(chan error, 1)}
	s.mu.Lock()
	if _, exists := s.dialogResponses[tabID]; exists {
		s.mu.Unlock()
		return errors.New("browser dialog response is already armed")
	}
	s.dialogResponses[tabID] = response
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.dialogResponses, tabID)
		s.mu.Unlock()
	}()
	if err := s.Click(actionCtx, tabID, backendNodeID); err != nil {
		return err
	}
	select {
	case err := <-response.result:
		return err
	case <-actionCtx.Done():
		return actionCtx.Err()
	}
}

func (s *chromiumSession) Download(
	ctx context.Context,
	tabID string,
	backendNodeID int64,
	maxBytes int64,
) (BackendArtifact, error) {
	if maxBytes <= 0 {
		return BackendArtifact{}, errors.New("browser download size limit must be greater than zero")
	}
	if !filepath.IsAbs(s.downloadRoot) {
		return BackendArtifact{}, errors.New("browser download root is unavailable")
	}
	frameIDs := make(map[cdp.FrameID]struct{})
	if err := s.runInTab(ctx, tabID, chromedp.ActionFunc(func(actionCtx context.Context) error {
		frameTree, err := page.GetFrameTree().Do(actionCtx)
		if err != nil {
			return err
		}
		if frameTree == nil || frameTree.Frame == nil {
			return errors.New("browser tab frame is unavailable")
		}
		collectFrameIDs(frameTree, frameIDs)
		return nil
	})); err != nil {
		return BackendArtifact{}, err
	}
	directory, err := os.MkdirTemp(s.downloadRoot, "download-")
	if err != nil {
		return BackendArtifact{}, err
	}
	defer os.RemoveAll(directory)
	actionCtx, done := s.newActionContext(ctx)
	defer done()
	browserCtx, err := s.getBrowserExecutorContext(actionCtx)
	if err != nil {
		return BackendArtifact{}, err
	}
	s.mu.Lock()
	if s.downloadArmed {
		s.mu.Unlock()
		return BackendArtifact{}, errors.New("browser download is already armed")
	}
	for len(s.downloadEvents) > 0 {
		<-s.downloadEvents
	}
	s.downloadArmed = true
	s.downloadFrameIDs = frameIDs
	s.downloadGUID = ""
	s.downloadMaxBytes = maxBytes
	s.downloadLimitSent = false
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.downloadArmed = false
		s.downloadFrameIDs = nil
		s.downloadGUID = ""
		s.downloadMaxBytes = 0
		s.downloadLimitSent = false
		s.mu.Unlock()
		_ = cdpbrowser.SetDownloadBehavior(cdpbrowser.SetDownloadBehaviorBehaviorDeny).Do(browserCtx)
	}()
	if err := cdpbrowser.SetDownloadBehavior(cdpbrowser.SetDownloadBehaviorBehaviorAllowAndName).
		WithDownloadPath(directory).
		WithEventsEnabled(true).
		Do(browserCtx); err != nil {
		return BackendArtifact{}, err
	}
	if err := s.Click(actionCtx, tabID, backendNodeID); err != nil {
		return BackendArtifact{}, err
	}

	var guid string
	var sourceURL string
	var name string
	for {
		select {
		case raw := <-s.downloadEvents:
			switch event := raw.(type) {
			case *cdpbrowser.EventDownloadWillBegin:
				if guid == "" {
					guid, sourceURL, name = event.GUID, event.URL, event.SuggestedFilename
				}
			case *cdpbrowser.EventDownloadProgress:
				if guid == "" || event.GUID != guid {
					continue
				}
				if event.ReceivedBytes > float64(maxBytes) || event.TotalBytes > float64(maxBytes) {
					_ = cdpbrowser.CancelDownload(guid).Do(browserCtx)
					return BackendArtifact{}, errors.New("browser download exceeds the size limit")
				}
				switch event.State {
				case cdpbrowser.DownloadProgressStateCanceled:
					return BackendArtifact{}, errors.New("browser download was cancelled")
				case cdpbrowser.DownloadProgressStateCompleted:
					return readDownloadedArtifact(directory, guid, name, sourceURL, maxBytes)
				}
			}
		case <-actionCtx.Done():
			if guid != "" {
				_ = cdpbrowser.CancelDownload(guid).Do(browserCtx)
			}
			return BackendArtifact{}, actionCtx.Err()
		}
	}
}

func collectFrameIDs(tree *page.FrameTree, result map[cdp.FrameID]struct{}) {
	if tree == nil || tree.Frame == nil {
		return
	}
	result[tree.Frame.ID] = struct{}{}
	for _, child := range tree.ChildFrames {
		collectFrameIDs(child, result)
	}
}

func readDownloadedArtifact(directory, guid, name, sourceURL string, maxBytes int64) (BackendArtifact, error) {
	path := filepath.Join(directory, guid)
	info, err := os.Lstat(path)
	if err != nil {
		return BackendArtifact{}, err
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxBytes {
		return BackendArtifact{}, errors.New("browser downloaded artifact is invalid")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return BackendArtifact{}, err
	}
	if int64(len(data)) != info.Size() {
		return BackendArtifact{}, errors.New("browser downloaded artifact changed before it was stored")
	}

	return BackendArtifact{
		Kind: ArtifactDownload, Name: name, MIMEType: http.DetectContentType(data), SourceURL: sourceURL, Data: data,
	}, nil
}

func (s *chromiumSession) getConsoleListener(tabID string) func(any) {
	return func(event any) {
		var message ConsoleMessage
		switch value := event.(type) {
		case *cdpruntime.EventConsoleAPICalled:
			parts := make([]string, 0, len(value.Args))
			for _, argument := range value.Args {
				parts = append(parts, getRemoteObjectText(argument))
			}
			message = ConsoleMessage{
				Level: getConsoleLevel(value.Type), Text: sanitizeConsoleText(strings.Join(parts, " ")), Timestamp: time.Now().UTC(),
			}
		case *cdpruntime.EventExceptionThrown:
			if value.ExceptionDetails == nil {
				return
			}
			text := value.ExceptionDetails.Text
			if value.ExceptionDetails.Exception != nil {
				text += " " + getRemoteObjectText(value.ExceptionDetails.Exception)
			}
			message = ConsoleMessage{Level: ConsoleError, Text: sanitizeConsoleText(text), Timestamp: time.Now().UTC()}
		default:
			return
		}
		if message.Text == "" {
			return
		}
		s.recordConsoleMessage(tabID, message)
	}
}

func (s *chromiumSession) recordConsoleMessage(tabID string, message ConsoleMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	messages := append(s.consoleMessages[tabID], message)
	if len(messages) > maxConsoleMessages {
		messages = append([]ConsoleMessage(nil), messages[len(messages)-maxConsoleMessages:]...)
	}
	s.consoleMessages[tabID] = messages
}

func getRemoteObjectText(value *cdpruntime.RemoteObject) string {
	if value == nil {
		return ""
	}
	if len(value.Value) > 0 {
		var decoded any
		if err := json.Unmarshal(value.Value, &decoded); err == nil {
			return fmt.Sprint(decoded)
		}
	}
	if value.UnserializableValue != "" {
		return string(value.UnserializableValue)
	}
	return value.Description
}

func getConsoleLevel(value cdpruntime.APIType) ConsoleLevel {
	switch value {
	case cdpruntime.APITypeDebug:
		return ConsoleDebug
	case cdpruntime.APITypeWarning:
		return ConsoleWarn
	case cdpruntime.APITypeError, cdpruntime.APITypeAssert:
		return ConsoleError
	default:
		return ConsoleInfo
	}
}
