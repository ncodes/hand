package environment

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	processenv "github.com/wandxy/hand/internal/environment/process"
	envtypes "github.com/wandxy/hand/internal/environment/types"
	"github.com/wandxy/hand/internal/guardrails"
)

func TestNewRuntime_DefaultsRootToCWDAndNormalizesPolicy(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	runtime := NewRuntime(nil, guardrails.CommandPolicy{
		Ask:  []string{" git push "},
		Deny: []string{"git push", "git push"},
	})

	require.Equal(t, []string{dir}, runtime.FilePolicy().Roots)
	require.Equal(t, []string{"git push"}, runtime.CommandPolicy().Ask)
	require.Equal(t, []string{"git push"}, runtime.CommandPolicy().Deny)
	require.IsType(t, &processenv.DefaultManager{}, runtime.processMgr)
	require.IsType(t, &MemoryPlanStore{}, runtime.plans)
}

func TestNewRuntime_FallsBackWhenGetwdFails(t *testing.T) {
	originalGetwd := getwd

	t.Cleanup(func() {
		getwd = originalGetwd
	})

	getwd = func() (string, error) {
		return "", errors.New("getwd failed")
	}

	runtime := NewRuntime(nil, guardrails.CommandPolicy{})

	expectedRoot, err := filepath.Abs(".")
	require.NoError(t, err)
	require.Equal(t, []string{filepath.Clean(expectedRoot)}, runtime.FilePolicy().Roots)
}

func TestNewRuntime_NormalizesConfiguredRoots(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "workspace")

	runtime := NewRuntime([]string{root, filepath.Join(root, ".")}, guardrails.CommandPolicy{})

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
	runtime := NewRuntime([]string{t.TempDir()}, guardrails.CommandPolicy{})

	replaced, err := runtime.ReplacePlan("session-1", envtypes.Plan{
		Steps: []envtypes.PlanStep{{ID: "step-1", Content: "First", Status: envtypes.PlanStatusInProgress}},
	})
	require.NoError(t, err)

	merged, err := runtime.MergePlan("session-1", []envtypes.PartialPlanStep{{
		ID:      "step-2",
		Content: new("Second"),
		Status:  new(envtypes.PlanStatusPending),
	}}, "updated", false)

	cleared := runtime.ClearPlan("session-1")

	require.Len(t, replaced.Steps, 1)
	require.Len(t, merged.Steps, 2)
	require.Equal(t, "updated", merged.Explanation)
	require.Equal(t, envtypes.Plan{}, cleared)
	require.Equal(t, envtypes.Plan{}, runtime.GetPlan("session-1"))
}

func TestRuntime_PlanMethodsHandleNilReceiver(t *testing.T) {
	var runtime *Runtime

	require.Equal(t, envtypes.Plan{}, runtime.GetPlan("session-1"))

	replaced, err := runtime.ReplacePlan("session-1", envtypes.Plan{})
	require.Equal(t, envtypes.Plan{}, replaced)
	require.EqualError(t, err, "plan store is required")

	require.Equal(t, envtypes.Plan{}, runtime.ClearPlan("session-1"))

	_, err = runtime.MergePlan("session-1", nil, "", false)
	require.EqualError(t, err, "plan store is required")

	runtime.HydratePlan("session-1", envtypes.Plan{
		Steps: []envtypes.PlanStep{{ID: "step-1", Content: "First", Status: envtypes.PlanStatusInProgress}},
	})

	require.Equal(t, envtypes.Plan{}, runtime.GetPlan("session-1"))
}

func TestRuntime_HydratePlanDelegatesToStore(t *testing.T) {
	runtime := NewRuntime([]string{t.TempDir()}, guardrails.CommandPolicy{})

	runtime.HydratePlan("session-1", envtypes.Plan{
		Steps:       []envtypes.PlanStep{{ID: "step-1", Content: "First", Status: envtypes.PlanStatusInProgress}},
		Explanation: "restored",
	})

	require.Equal(t, envtypes.Plan{
		Steps:       []envtypes.PlanStep{{ID: "step-1", Content: "First", Status: envtypes.PlanStatusInProgress}},
		Explanation: "restored",
	}, runtime.GetPlan("session-1"))
}

func TestRuntime_ProcessMethodsDelegateToStore(t *testing.T) {
	runtime := NewRuntime([]string{t.TempDir()}, guardrails.CommandPolicy{})

	info, err := runtime.StartProcess(context.Background(), "session-1", processenv.StartRequest{
		Command:           "printf",
		Args:              []string{"hello"},
		OutputBufferBytes: 32,
	})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		current, err := runtime.GetProcess("session-1", info.ID)
		require.NoError(t, err)
		return current.Status == processenv.StatusExited
	}, 5*time.Second, 20*time.Millisecond)

	output, err := runtime.ReadProcess("session-1", processenv.ReadRequest{ProcessID: info.ID})
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

	_, err := runtime.StartProcess(context.Background(), "session-1", processenv.StartRequest{})
	require.EqualError(t, err, "process manager is required")

	_, err = runtime.GetProcess("session-1", "proc_1")
	require.EqualError(t, err, "process manager is required")

	_, err = runtime.ReadProcess("session-1", processenv.ReadRequest{ProcessID: "proc_1"})
	require.EqualError(t, err, "process manager is required")

	_, err = runtime.StopProcess(context.Background(), "session-1", "proc_1")
	require.EqualError(t, err, "process manager is required")

	require.Nil(t, runtime.ListProcesses("session-1"))
}
