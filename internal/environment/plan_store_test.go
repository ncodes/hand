package environment

import (
	"testing"

	"github.com/stretchr/testify/require"

	envtypes "github.com/wandxy/hand/internal/environment/types"
)

func TestMemoryPlanStore_ReturnsDefensiveCopies(t *testing.T) {
	store := &MemoryPlanStore{}
	plan, err := store.Replace("session-1", envtypes.Plan{
		Steps: []envtypes.PlanStep{{ID: "step-1", Content: "First", Status: envtypes.PlanStatusInProgress}},
	})
	require.NoError(t, err)
	plan.Steps[0].Content = "mutated"

	current := store.Get("session-1")
	require.Equal(t, "First", current.Steps[0].Content)
}

func TestMemoryPlanStore_MergePreservesOrderAndAppendsNewSteps(t *testing.T) {
	store := &MemoryPlanStore{}
	_, err := store.Replace("session-1", envtypes.Plan{
		Steps: []envtypes.PlanStep{
			{ID: "step-1", Content: "First", Status: envtypes.PlanStatusInProgress},
			{ID: "step-2", Content: "Second", Status: envtypes.PlanStatusPending},
		},
	})
	require.NoError(t, err)

	merged, err := store.Merge("session-1", []envtypes.PartialPlanStep{
		{ID: "step-2", Content: new("Second updated")},
		{ID: "step-3", Content: new("Third"), Status: new(envtypes.PlanStatusPending)},
	}, "updated", false)

	require.NoError(t, err)
	require.Equal(t, []envtypes.PlanStep{
		{ID: "step-1", Content: "First", Status: envtypes.PlanStatusInProgress},
		{ID: "step-2", Content: "Second updated", Status: envtypes.PlanStatusPending},
		{ID: "step-3", Content: "Third", Status: envtypes.PlanStatusPending},
	}, merged.Steps)
	require.Equal(t, "updated", merged.Explanation)
}

func TestMemoryPlanStore_ClearRemovesPlan(t *testing.T) {
	store := &MemoryPlanStore{}
	_, err := store.Replace("session-1", envtypes.Plan{
		Steps: []envtypes.PlanStep{{ID: "step-1", Content: "First", Status: envtypes.PlanStatusInProgress}},
	})
	require.NoError(t, err)

	cleared := store.Clear("session-1")

	require.Equal(t, envtypes.Plan{}, cleared)
	require.Equal(t, envtypes.Plan{}, store.Get("session-1"))
}

func TestMemoryPlanStore_ReplaceRejectsInvalidPlan(t *testing.T) {
	store := &MemoryPlanStore{}

	plan, err := store.Replace("session-1", envtypes.Plan{
		Steps: []envtypes.PlanStep{{ID: "step-1", Content: "", Status: envtypes.PlanStatusInProgress}},
	})

	require.Equal(t, envtypes.Plan{}, plan)
	require.EqualError(t, err, "step 0 content is required")
	require.Equal(t, envtypes.Plan{}, store.Get("session-1"))
}

func TestMemoryPlanStore_MergeUpdatesStatusAndClearsCompleted(t *testing.T) {
	store := &MemoryPlanStore{}
	_, err := store.Replace("", envtypes.Plan{
		Steps: []envtypes.PlanStep{
			{ID: "step-1", Content: "First", Status: envtypes.PlanStatusInProgress},
			{ID: "step-2", Content: "Second", Status: envtypes.PlanStatusPending},
			{ID: "step-3", Content: "Third", Status: envtypes.PlanStatusCompleted},
		},
	})
	require.NoError(t, err)

	merged, err := store.Merge("", []envtypes.PartialPlanStep{
		{ID: "step-1", Status: new(envtypes.PlanStatusCompleted)},
		{ID: "step-2", Status: new(envtypes.PlanStatusInProgress)},
	}, "trimmed", true)

	require.NoError(t, err)
	require.Equal(t, envtypes.Plan{
		Steps: []envtypes.PlanStep{
			{ID: "step-2", Content: "Second", Status: envtypes.PlanStatusInProgress},
		},
		Explanation: "trimmed",
	}, merged)
	require.Equal(t, merged, store.Get(""))
}

func TestMemoryPlanStore_MergeRejectsInvalidPlan(t *testing.T) {
	store := &MemoryPlanStore{}
	_, err := store.Replace("session-1", envtypes.Plan{
		Steps: []envtypes.PlanStep{
			{ID: "step-1", Content: "First", Status: envtypes.PlanStatusInProgress},
			{ID: "step-2", Content: "Second", Status: envtypes.PlanStatusPending},
		},
	})
	require.NoError(t, err)

	merged, err := store.Merge("session-1", []envtypes.PartialPlanStep{
		{ID: "step-1", Status: new(envtypes.PlanStatusCompleted)},
	}, "invalid", false)

	require.Equal(t, envtypes.Plan{}, merged)
	require.EqualError(t, err, "exactly one step must be in_progress while active work remains")
	require.Equal(t, envtypes.Plan{
		Steps: []envtypes.PlanStep{
			{ID: "step-1", Content: "First", Status: envtypes.PlanStatusInProgress},
			{ID: "step-2", Content: "Second", Status: envtypes.PlanStatusPending},
		},
	}, store.Get("session-1"))
}

func TestMemoryPlanStore_MergeInitializesEmptyStore(t *testing.T) {
	store := &MemoryPlanStore{}

	merged, err := store.Merge("session-1", []envtypes.PartialPlanStep{
		{ID: "step-1", Content: new("First"), Status: new(envtypes.PlanStatusInProgress)},
	}, "created", false)

	require.NoError(t, err)
	require.Equal(t, envtypes.Plan{
		Steps:       []envtypes.PlanStep{{ID: "step-1", Content: "First", Status: envtypes.PlanStatusInProgress}},
		Explanation: "created",
	}, merged)
}

func TestMemoryPlanStore_ClearEmptyStoreReturnsEmptyPlan(t *testing.T) {
	store := &MemoryPlanStore{}

	require.Equal(t, envtypes.Plan{}, store.Clear("session-1"))
}

func TestMemoryPlanStore_HydrateStoresDefensiveCopy(t *testing.T) {
	store := &MemoryPlanStore{}
	plan := envtypes.Plan{
		Steps:       []envtypes.PlanStep{{ID: "step-1", Content: "First", Status: envtypes.PlanStatusInProgress}},
		Explanation: "loaded",
	}

	store.Hydrate("session-1", plan)
	plan.Steps[0].Content = "mutated"

	current := store.Get("session-1")
	require.Equal(t, envtypes.Plan{
		Steps:       []envtypes.PlanStep{{ID: "step-1", Content: "First", Status: envtypes.PlanStatusInProgress}},
		Explanation: "loaded",
	}, current)
}

func TestMemoryPlanStore_ValidatePlanRejectsMissingID(t *testing.T) {
	err := envtypes.ValidatePlan(envtypes.Plan{
		Steps: []envtypes.PlanStep{{ID: " ", Content: "First", Status: envtypes.PlanStatusInProgress}},
	})

	require.EqualError(t, err, "step 0 id is required")
}

func TestMemoryPlanStore_ValidatePlanRejectsInvalidStatus(t *testing.T) {
	err := envtypes.ValidatePlan(envtypes.Plan{
		Steps: []envtypes.PlanStep{{ID: "step-1", Content: "First", Status: "unknown"}},
	})

	require.EqualError(t, err, "step 0 status is invalid")
}

func TestMemoryPlanStore_ValidatePlanRejectsMultipleInProgressSteps(t *testing.T) {
	err := envtypes.ValidatePlan(envtypes.Plan{
		Steps: []envtypes.PlanStep{
			{ID: "step-1", Content: "First", Status: envtypes.PlanStatusInProgress},
			{ID: "step-2", Content: "Second", Status: envtypes.PlanStatusInProgress},
		},
	})

	require.EqualError(t, err, "only one step may be in_progress")
}

func TestMemoryPlanStore_ValidatePlanAllowsTerminalOnlyPlan(t *testing.T) {
	err := envtypes.ValidatePlan(envtypes.Plan{
		Steps: []envtypes.PlanStep{
			{ID: "step-1", Content: "First", Status: envtypes.PlanStatusCompleted},
			{ID: "step-2", Content: "Second", Status: envtypes.PlanStatusCancelled},
		},
	})

	require.NoError(t, err)
}
