package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/wandxy/morph/internal/permissions"
	rpcclient "github.com/wandxy/morph/internal/rpc/client"
	"github.com/wandxy/morph/internal/rpc/rpcmeta"
	"github.com/wandxy/morph/internal/trace"
	"github.com/wandxy/morph/pkg/str"
)

type rootChatPermissionAPIProvider interface {
	PermissionAPI() rpcclient.PermissionAPI
}

func getRootChatPermissionAPI(client rpcclient.ChatClient) rpcclient.PermissionAPI {
	provider, ok := client.(rootChatPermissionAPIProvider)
	if !ok {
		return nil
	}

	return provider.PermissionAPI()
}

type rootChatApprovalHandler struct {
	input       *bufio.Reader
	output      io.Writer
	permissions rpcclient.PermissionAPI
	interactive bool
}

func newRootChatApprovalHandler(
	input io.Reader,
	output io.Writer,
	permissionAPI rpcclient.PermissionAPI,
	interactive bool,
) *rootChatApprovalHandler {
	var reader *bufio.Reader
	if input != nil {
		reader = bufio.NewReader(input)
	}

	return &rootChatApprovalHandler{
		input:       reader,
		output:      output,
		permissions: permissionAPI,
		interactive: interactive,
	}
}

func (h *rootChatApprovalHandler) Handle(ctx context.Context, event rpcclient.Event) (bool, error) {
	traceEvent, ok := event.TraceEvent.(*trace.Event)
	if !ok || traceEvent == nil || traceEvent.Type != trace.EvtPermissionApprovalChanged {
		return false, nil
	}

	decoded, ok := trace.DecodePayload(traceEvent.Type, traceEvent.Payload)
	if !ok {
		return true, errors.New("invalid permission approval event")
	}
	payload, ok := decoded.(trace.PermissionApprovalPayload)
	if !ok || str.String(payload.RequestID).Trim() == "" {
		return true, errors.New("invalid permission approval event")
	}
	if payload.Status != string(permissions.ApprovalPending) {
		return true, nil
	}
	if !h.interactive {
		return true, fmt.Errorf(
			"approval required for %s; root chat input and output must be an interactive terminal (%s)",
			payload.Summary,
			payload.RequestID,
		)
	}
	if h.input == nil || h.permissions == nil {
		return true, errors.New("interactive permission approval is unavailable")
	}

	approved, scope, err := h.prompt(ctx, payload)
	if err != nil {
		return true, err
	}
	resolveCtx := rpcmeta.WithOutgoingPermissionSurface(ctx, permissions.SurfaceCLI)
	request, err := h.permissions.ResolveApprovalRequest(resolveCtx, payload.RequestID, approved, scope)
	if err != nil {
		return true, fmt.Errorf("resolve permission approval: %w", err)
	}

	status := "denied"
	if approved {
		status = "approved (" + string(scope) + ")"
	}
	summary := request.Summary
	if str.String(summary).Trim() == "" {
		summary = payload.Summary
	}
	_, err = fmt.Fprintf(h.output, "\nPermission %s — %s\n\n", status, summary)

	return true, err
}

func (h *rootChatApprovalHandler) prompt(
	ctx context.Context,
	payload trace.PermissionApprovalPayload,
) (bool, permissions.GrantScope, error) {
	if _, err := fmt.Fprintf(h.output, "\nPermission approval required\n%s\n", payload.Summary); err != nil {
		return false, "", err
	}

	if len(payload.Effects) > 0 {
		if _, err := fmt.Fprintf(h.output, "Effects: %s\n", strings.Join(payload.Effects, ", ")); err != nil {
			return false, "", err
		}
	}
	if str.String(payload.Reason).Trim() != "" {
		if _, err := fmt.Fprintf(h.output, "Reason: %s\n", payload.Reason); err != nil {
			return false, "", err
		}
	}
	if !payload.ExpiresAt.IsZero() {
		if _, err := fmt.Fprintf(h.output, "Expires: %s\n", payload.ExpiresAt.In(time.Local).Format("15:04:05 MST")); err != nil {
			return false, "", err
		}
	}

	alwaysAvailable := isRootChatAlwaysApprovalAvailable(payload.Effects)
	choices := "[y] allow once  [s] session  [n] deny"
	if alwaysAvailable {
		choices = "[y] allow once  [s] session  [a] always  [n] deny"
	}
	if _, err := fmt.Fprintf(h.output, "%s\n> ", choices); err != nil {
		return false, "", err
	}

	result := make(chan rootChatApprovalChoice, 1)
	go func() {
		approved, scope, err := h.readChoice(alwaysAvailable)
		result <- rootChatApprovalChoice{approved: approved, scope: scope, err: err}
	}()

	var expiry <-chan time.Time
	var timer *time.Timer
	if !payload.ExpiresAt.IsZero() {
		timer = time.NewTimer(max(time.Until(payload.ExpiresAt), 0))
		expiry = timer.C
		defer timer.Stop()
	}

	select {
	case choice := <-result:
		return choice.approved, choice.scope, choice.err
	case <-ctx.Done():
		return false, "", ctx.Err()
	case <-expiry:
		return false, "", fmt.Errorf("permission approval %s expired", payload.RequestID)
	}
}

type rootChatApprovalChoice struct {
	approved bool
	scope    permissions.GrantScope
	err      error
}

func (h *rootChatApprovalHandler) readChoice(
	alwaysAvailable bool,
) (bool, permissions.GrantScope, error) {
	for {
		value, readErr := h.input.ReadString('\n')
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "y", "yes":
			return true, permissions.GrantOnce, nil
		case "s", "session":
			return true, permissions.GrantSession, nil
		case "a", "always":
			if alwaysAvailable {
				return true, permissions.GrantAlways, nil
			}
		case "n", "no", "deny":
			return false, "", nil
		}
		if readErr != nil {
			return false, "", fmt.Errorf("read permission approval: %w", readErr)
		}
		if _, err := fmt.Fprint(h.output, "Choose y, s, n"); err != nil {
			return false, "", err
		}
		if alwaysAvailable {
			if _, err := fmt.Fprint(h.output, ", or a"); err != nil {
				return false, "", err
			}
		}
		if _, err := fmt.Fprint(h.output, ": "); err != nil {
			return false, "", err
		}
	}
}

func isRootChatAlwaysApprovalAvailable(effects []string) bool {
	for _, effect := range effects {
		switch permissions.Effect(effect) {
		case permissions.EffectDestructive, permissions.EffectCredentialBearing, permissions.EffectPrivilegeChanging,
			permissions.EffectExecution, permissions.EffectNetwork, permissions.EffectExternalSystem:
			return false
		}
	}

	return true
}
