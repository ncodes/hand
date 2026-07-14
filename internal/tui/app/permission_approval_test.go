package tui

import (
	"context"
	"errors"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"github.com/wandxy/morph/internal/permissions"
	rpcclient "github.com/wandxy/morph/internal/rpc/client"
	"github.com/wandxy/morph/internal/trace"
)

func TestModel_PermissionApprovalPromptResolvesFromKeyboard(t *testing.T) {
	client := &permissionAPIStub{}
	runModel := newModel()
	runModel.permissionClient = client
	message := permissionApprovalMsg{
		RequestID: "approval_1", Status: string(permissions.ApprovalPending),
		Summary: "run_command · execute process", Reason: "command policy requires approval",
		Effects: []string{"execution"}, ExpiresAt: time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC),
	}

	updated, _ := runModel.handleAppEvent(applyTUIMessageEvent{Message: message})
	require.Equal(t, "approval_1", updated.pendingApprovalID)
	require.Len(t, updated.messages, 1)
	require.Contains(t, updated.messages[0].PlainText(), "[y] allow once")
	require.NotContains(t, updated.messages[0].PlainText(), "printf secret")

	modelValue, cmd, handled := updated.handleKeyPressMsg(tea.KeyPressMsg{Code: 'y', Text: "y"})
	require.True(t, handled)
	require.NotNil(t, cmd)
	next := modelValue.(model)
	result := cmd().(permissionResolutionCompletedMsg)
	require.NoError(t, result.Err)
	require.Equal(t, permissions.GrantOnce, client.scope)

	modelValue, _, handled = next.handleAsyncMsg(result)
	require.True(t, handled)
	next = modelValue.(model)
	require.Empty(t, next.pendingApprovalID)
	require.Contains(t, next.messages[0].PlainText(), "approved")
}

func TestModel_PermissionApprovalSupportsSessionAlwaysDenyAndFailure(t *testing.T) {
	for _, test := range []struct {
		name     string
		key      string
		approved bool
		scope    permissions.GrantScope
	}{
		{name: "session", key: "s", approved: true, scope: permissions.GrantSession},
		{name: "always", key: "a", approved: true, scope: permissions.GrantAlways},
		{name: "deny", key: "n", approved: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := &permissionAPIStub{}
			runModel := newModel()
			runModel.permissionClient = client
			runModel.applyTUIMessage(permissionApprovalMsg{RequestID: "approval", Status: "pending", Summary: "operation"})
			_, cmd, handled := runModel.handleKeyPressMsg(tea.KeyPressMsg{Code: rune(test.key[0]), Text: test.key})
			require.True(t, handled)
			result := cmd().(permissionResolutionCompletedMsg)
			require.NoError(t, result.Err)
			require.Equal(t, test.approved, client.approved)
			require.Equal(t, test.scope, client.scope)
		})
	}

	runModel := newModel()
	runModel.permissionClient = &permissionAPIStub{err: errors.New("store unavailable")}
	runModel.applyTUIMessage(permissionApprovalMsg{
		RequestID: "approval", Status: string(permissions.ApprovalPending), Summary: "run command",
	})
	_, cmd, handled := runModel.handleKeyPressMsg(tea.KeyPressMsg{Code: 'y', Text: "y"})
	require.True(t, handled)
	result := cmd().(permissionResolutionCompletedMsg)
	modelValue, _, handled := runModel.handleAsyncMsg(result)
	require.True(t, handled)
	failedModel := modelValue.(model)
	require.Contains(t, failedModel.status.Text(), "approval failed")
	require.Empty(t, failedModel.pendingApprovalID)
	require.Contains(t, failedModel.messages[0].PlainText(), "Permission failed")
	require.Contains(t, failedModel.messages[0].PlainText(), "run command")
}

func TestModel_PermissionApprovalHidesAndRejectsUnsafeAlwaysChoice(t *testing.T) {
	runModel := newModel()
	runModel.permissionClient = &permissionAPIStub{}
	runModel.applyTUIMessage(permissionApprovalMsg{
		RequestID: "approval", Status: "pending", Summary: "credential update",
		Effects: []string{string(permissions.EffectCredentialBearing)},
	})
	require.NotContains(t, runModel.messages[0].PlainText(), "[a] always")
	updated, cmd, handled := runModel.handleKeyPressMsg(tea.KeyPressMsg{Code: 'a', Text: "a"})
	require.True(t, handled)
	require.NotNil(t, cmd)
	require.Contains(t, updated.(model).status.Text(), "always approval is unavailable")
}

func TestModel_PermissionApprovalQueuesIndependentRequests(t *testing.T) {
	runModel := newModel()
	runModel.applyTUIMessage(permissionApprovalMsg{
		RequestID: "approval_1", Status: string(permissions.ApprovalPending), Summary: "first",
	})
	runModel.applyTUIMessage(permissionApprovalMsg{
		RequestID: "approval_2", Status: string(permissions.ApprovalPending), Summary: "second",
	})
	require.Equal(t, "approval_1", runModel.pendingApprovalID)
	require.Len(t, runModel.messages, 2)

	runModel.applyTUIMessage(permissionApprovalMsg{
		RequestID: "approval_1", Status: string(permissions.ApprovalApproved), Summary: "first",
	})
	require.Equal(t, "approval_2", runModel.pendingApprovalID)

	runModel.applyTUIMessage(permissionApprovalMsg{
		RequestID: "approval_2", Status: string(permissions.ApprovalDenied), Summary: "second",
	})
	require.Empty(t, runModel.pendingApprovalID)
}

func TestTraceEventToTUIMessage_DecodesPermissionApprovalLifecycle(t *testing.T) {
	message, ok := traceEventToTUIMessage(trace.Event{
		Type: trace.EvtPermissionApprovalChanged,
		Payload: map[string]any{
			"request_id": "approval_1", "status": "pending", "operation_summary": "run_command · execute process",
			"effects": []string{"execution"},
		},
	})
	require.True(t, ok)
	require.Equal(t, "approval_1", message.(permissionApprovalMsg).RequestID)
}

type permissionAPIStub struct {
	approved bool
	scope    permissions.GrantScope
	err      error
}

func (s *permissionAPIStub) ResolveApprovalRequest(
	_ context.Context,
	id string,
	approved bool,
	scope permissions.GrantScope,
) (permissions.ApprovalRequest, error) {
	s.approved = approved
	s.scope = scope
	if s.err != nil {
		return permissions.ApprovalRequest{}, s.err
	}
	status := permissions.ApprovalDenied
	if approved {
		status = permissions.ApprovalApproved
	}
	return permissions.ApprovalRequest{ID: id, Status: status, Scope: scope}, nil
}

func (*permissionAPIStub) ListApprovalRequests(context.Context, permissions.ApprovalQuery) ([]permissions.ApprovalRequest, error) {
	return nil, nil
}
func (*permissionAPIStub) GetApprovalRequest(context.Context, string) (permissions.ApprovalRequest, bool, error) {
	return permissions.ApprovalRequest{}, false, nil
}
func (*permissionAPIStub) ListApprovalGrants(context.Context, permissions.GrantQuery) ([]permissions.ApprovalGrant, error) {
	return nil, nil
}
func (*permissionAPIStub) RevokeApprovalGrant(context.Context, string) (permissions.ApprovalGrant, error) {
	return permissions.ApprovalGrant{}, nil
}
func (*permissionAPIStub) DeleteApprovalRecord(context.Context, string) (permissions.ApprovalDeleteResult, error) {
	return permissions.ApprovalDeleteResult{}, nil
}
func (*permissionAPIStub) PruneApprovals(context.Context, bool) (permissions.ApprovalPruneResult, error) {
	return permissions.ApprovalPruneResult{}, nil
}

var _ rpcclient.PermissionAPI = (*permissionAPIStub)(nil)
