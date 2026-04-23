package environment

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/environment/planstore"
	"github.com/wandxy/hand/internal/environment/process"
	"github.com/wandxy/hand/internal/environment/sessionmessages"
	"github.com/wandxy/hand/internal/environment/sessionsearch"
	"github.com/wandxy/hand/internal/guardrails"
	"github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/session"
	"github.com/wandxy/hand/internal/storage"
	"github.com/wandxy/hand/internal/storage/memory"
	"github.com/wandxy/hand/pkg/nanoid"
)

var runtimeSearchSessionID = nanoid.MustFromSeed(storage.SessionIDPrefix, "runtime-search", "EnvironmentRuntimeTestSeed")

func TestNewRuntime_DefaultsRootToCWDAndNormalizesPolicy(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	runtime := NewRuntime(nil, guardrails.CommandPolicy{
		Ask:  []string{" git push "},
		Deny: []string{"git push", "git push"},
	}, nil)

	require.Equal(t, []string{dir}, runtime.FilePolicy().Roots)
	require.Equal(t, []string{"git push"}, runtime.CommandPolicy().Ask)
	require.Equal(t, []string{"git push"}, runtime.CommandPolicy().Deny)
	require.IsType(t, &process.DefaultManager{}, runtime.processMgr)
	require.IsType(t, &planstore.MemoryPlanStore{}, runtime.plans)
}

func TestNewRuntime_FallsBackWhenGetwdFails(t *testing.T) {
	originalGetwd := getwd

	t.Cleanup(func() {
		getwd = originalGetwd
	})

	getwd = func() (string, error) {
		return "", errors.New("getwd failed")
	}

	runtime := NewRuntime(nil, guardrails.CommandPolicy{}, nil)

	expectedRoot, err := filepath.Abs(".")
	require.NoError(t, err)
	require.Equal(t, []string{filepath.Clean(expectedRoot)}, runtime.FilePolicy().Roots)
}

func TestNewRuntime_NormalizesConfiguredRoots(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "workspace")

	runtime := NewRuntime([]string{root, filepath.Join(root, ".")}, guardrails.CommandPolicy{}, nil)

	require.Equal(t, []string{root}, runtime.FilePolicy().Roots)
}

func TestRuntime_FilePolicyHandlesNilReceiver(t *testing.T) {
	var runtime *Runtime

	require.Equal(t, guardrails.NormalizeRoots(nil), runtime.FilePolicy().Roots)
}

func TestRuntime_CommandPolicyHandlesNilReceiver(t *testing.T) {
	var runtime *Runtime

	require.Equal(t, guardrails.CommandPolicy{}.Normalize(), runtime.CommandPolicy())
}

func TestRuntime_PlanMethodsDelegateToStore(t *testing.T) {
	runtime := NewRuntime([]string{t.TempDir()}, guardrails.CommandPolicy{}, nil)

	replaced, err := runtime.ReplacePlan("session-1", planstore.Plan{
		Steps: []planstore.PlanStep{{ID: "step-1", Content: "First", Status: planstore.PlanStatusInProgress}},
	})
	require.NoError(t, err)

	merged, err := runtime.MergePlan("session-1", []planstore.PartialPlanStep{{
		ID:      "step-2",
		Content: ptrTo("Second"),
		Status:  ptrTo(planstore.PlanStatusPending),
	}}, "updated", false)

	cleared := runtime.ClearPlan("session-1")

	require.Len(t, replaced.Steps, 1)
	require.Len(t, merged.Steps, 2)
	require.Equal(t, "updated", merged.Explanation)
	require.Equal(t, planstore.Plan{}, cleared)
	require.Equal(t, planstore.Plan{}, runtime.GetPlan("session-1"))
}

func TestRuntime_PlanMethodsHandleNilReceiver(t *testing.T) {
	var runtime *Runtime

	require.Equal(t, planstore.Plan{}, runtime.GetPlan("session-1"))

	replaced, err := runtime.ReplacePlan("session-1", planstore.Plan{})
	require.Equal(t, planstore.Plan{}, replaced)
	require.EqualError(t, err, "plan store is required")

	require.Equal(t, planstore.Plan{}, runtime.ClearPlan("session-1"))

	_, err = runtime.MergePlan("session-1", nil, "", false)
	require.EqualError(t, err, "plan store is required")

	runtime.HydratePlan("session-1", planstore.Plan{
		Steps: []planstore.PlanStep{{ID: "step-1", Content: "First", Status: planstore.PlanStatusInProgress}},
	})

	require.Equal(t, planstore.Plan{}, runtime.GetPlan("session-1"))
}

func TestRuntime_HydratePlanDelegatesToStore(t *testing.T) {
	runtime := NewRuntime([]string{t.TempDir()}, guardrails.CommandPolicy{}, nil)

	runtime.HydratePlan("session-1", planstore.Plan{
		Steps:       []planstore.PlanStep{{ID: "step-1", Content: "First", Status: planstore.PlanStatusInProgress}},
		Explanation: "restored",
	})

	require.Equal(t, planstore.Plan{
		Steps:       []planstore.PlanStep{{ID: "step-1", Content: "First", Status: planstore.PlanStatusInProgress}},
		Explanation: "restored",
	}, runtime.GetPlan("session-1"))
}

func TestRuntime_ProcessMethodsDelegateToStore(t *testing.T) {
	runtime := NewRuntime([]string{t.TempDir()}, guardrails.CommandPolicy{}, nil)

	info, err := runtime.StartProcess(context.Background(), "session-1", process.StartRequest{
		Command:           "printf",
		Args:              []string{"hello"},
		OutputBufferBytes: 32,
	})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		current, err := runtime.GetProcess("session-1", info.ID)
		require.NoError(t, err)
		return current.Status == process.StatusExited
	}, 5*time.Second, 20*time.Millisecond)

	output, err := runtime.ReadProcess("session-1", process.ReadRequest{ProcessID: info.ID})
	require.NoError(t, err)
	require.Equal(t, "hello", output.Stdout)

	stopped, err := runtime.StopProcess(context.Background(), "session-1", info.ID)
	require.NoError(t, err)
	require.Equal(t, info.ID, stopped.ID)

	list := runtime.ListProcesses("session-1")
	require.Len(t, list, 1)
	require.Equal(t, info.ID, list[0].ID)
}

func TestRuntime_ProcessMethodsHandleNilReceiver(t *testing.T) {
	var runtime *Runtime

	_, err := runtime.StartProcess(context.Background(), "session-1", process.StartRequest{})
	require.EqualError(t, err, "process manager is required")

	_, err = runtime.GetProcess("session-1", "proc_1")
	require.EqualError(t, err, "process manager is required")

	_, err = runtime.ReadProcess("session-1", process.ReadRequest{ProcessID: "proc_1"})
	require.EqualError(t, err, "process manager is required")

	_, err = runtime.StopProcess(context.Background(), "session-1", "proc_1")
	require.EqualError(t, err, "process manager is required")

	require.Nil(t, runtime.ListProcesses("session-1"))
}

func TestRuntime_SearchSessionDelegatesToSessionManager(t *testing.T) {
	store := memory.NewSessionStore()
	manager, err := session.NewManager(store, time.Minute, time.Hour)
	require.NoError(t, err)
	require.NoError(t, manager.Save(context.Background(), memory.Session{ID: runtimeSearchSessionID}))
	require.NoError(t, manager.AppendMessages(context.Background(), runtimeSearchSessionID, []messages.Message{
		{Role: messages.RoleUser, Content: "hello world", CreatedAt: time.Now().UTC()},
		{Role: messages.RoleTool, Name: "process", Content: `{"process":{"id":"proc_1","status":"running"}}`, ToolCallID: "call-1", CreatedAt: time.Now().UTC()},
	}))

	runtime := NewRuntime([]string{t.TempDir()}, guardrails.CommandPolicy{}, manager)

	results, err := runtime.SearchSession(context.Background(), sessionsearch.SessionSearchRequest{
		SessionID:  runtimeSearchSessionID,
		Query:      "running",
		Role:       "tool",
		ToolName:   "process",
		MaxResults: 5,
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, runtimeSearchSessionID, results[0].SessionID)
	require.Len(t, results[0].Messages, 1)
	require.Equal(t, "tool", results[0].Messages[0].Role)
	require.Equal(t, "process", results[0].Messages[0].ToolName)
	require.NotZero(t, results[0].Messages[0].MessageID)
}

func TestRuntime_SearchSessionSupportsCrossSessionScope(t *testing.T) {
	store := memory.NewSessionStore()
	manager, err := session.NewManager(store, time.Minute, time.Hour)
	require.NoError(t, err)

	otherSessionID := nanoid.MustFromSeed(storage.SessionIDPrefix, "runtime-search-other", "EnvironmentRuntimeTestSeed")

	require.NoError(t, manager.Save(context.Background(), memory.Session{ID: runtimeSearchSessionID}))
	require.NoError(t, manager.Save(context.Background(), memory.Session{ID: otherSessionID}))
	require.NoError(t, manager.AppendMessages(context.Background(), runtimeSearchSessionID, []messages.Message{
		{Role: messages.RoleUser, Content: "origin needle", CreatedAt: time.Now().UTC()},
	}))
	require.NoError(t, manager.AppendMessages(context.Background(), otherSessionID, []messages.Message{
		{Role: messages.RoleUser, Content: "other needle", CreatedAt: time.Now().UTC()},
	}))

	runtime := NewRuntime([]string{t.TempDir()}, guardrails.CommandPolicy{}, manager)

	results, err := runtime.SearchSession(context.Background(), sessionsearch.SessionSearchRequest{
		IgnoreSessionID: runtimeSearchSessionID,
		Query:           "needle",
		MaxResults:      5,
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, otherSessionID, results[0].SessionID)
	require.Equal(t, "other needle", results[0].Messages[0].Snippet)
}

func TestRuntime_SearchSessionHandlesNilReceiver(t *testing.T) {
	var runtime *Runtime

	_, err := runtime.SearchSession(context.Background(), sessionsearch.SessionSearchRequest{SessionID: runtimeSearchSessionID, Query: "hello"})
	require.EqualError(t, err, "session manager is required")
}

func ptrTo[T any](value T) *T {
	return &value
}

func TestRuntime_GetSessionMessagesDelegatesToSessionManager(t *testing.T) {
	store := memory.NewSessionStore()
	manager, err := session.NewManager(store, time.Minute, time.Hour)
	require.NoError(t, err)
	require.NoError(t, manager.Save(context.Background(), memory.Session{ID: runtimeSearchSessionID}))
	require.NoError(t, manager.AppendMessages(context.Background(), runtimeSearchSessionID, []messages.Message{
		{Role: messages.RoleUser, Content: "hello world", CreatedAt: time.Now().UTC()},
		{Role: messages.RoleTool, Name: "process", Content: `{"process":{"id":"proc_1","status":"running"}}`, ToolCallID: "call-1", CreatedAt: time.Now().UTC()},
	}))

	runtime := NewRuntime([]string{t.TempDir()}, guardrails.CommandPolicy{}, manager)
	offsetStart := 0
	offsetEnd := 2

	response, err := runtime.GetSessionMessages(context.Background(), sessionmessages.SessionMessagesRequest{
		SessionID:   runtimeSearchSessionID,
		OffsetStart: &offsetStart,
		OffsetEnd:   &offsetEnd,
	})
	require.NoError(t, err)
	require.Equal(t, runtimeSearchSessionID, response.SessionID)
	require.Len(t, response.Messages, 2)
	require.Equal(t, []int{0, 1}, []int{response.Messages[0].Offset, response.Messages[1].Offset})
}

func TestRuntime_GetSessionMessagesHandlesNilReceiver(t *testing.T) {
	var runtime *Runtime
	offsetStart := 0
	offsetEnd := 1

	_, err := runtime.GetSessionMessages(context.Background(), sessionmessages.SessionMessagesRequest{
		SessionID:   runtimeSearchSessionID,
		OffsetStart: &offsetStart,
		OffsetEnd:   &offsetEnd,
	})
	require.EqualError(t, err, "session manager is required")
}
