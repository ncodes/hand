package planstore

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMemoryPlanStore_ReturnsDefensiveCopies(t *testing.T) {
	store := &MemoryPlanStore{}
	plan, err := store.Replace("session-1", Plan{
		Steps: []PlanStep{{ID: "step-1", Content: "First", Status: PlanStatusInProgress}},
	})
	require.NoError(t, err)
	plan.Steps[0].Content = "mutated"

	current := store.Get("session-1")
	require.Equal(t, "First", current.Steps[0].Content)
}

func TestMemoryPlanStore_MergePreservesOrderAndAppendsNewSteps(t *testing.T) {
	store := &MemoryPlanStore{}
	_, err := store.Replace("session-1", Plan{
		Steps: []PlanStep{
			{ID: "step-1", Content: "First", Status: PlanStatusInProgress},
			{ID: "step-2", Content: "Second", Status: PlanStatusPending},
		},
	})
	require.NoError(t, err)

	merged, err := store.Merge("session-1", []PartialPlanStep{
		{ID: "step-2", Content: new("Second updated")},
		{ID: "step-3", Content: new("Third"), Status: new(PlanStatusPending)},
	}, "updated", false)

	require.NoError(t, err)
	require.Equal(t, []PlanStep{
		{ID: "step-1", Content: "First", Status: PlanStatusInProgress},
		{ID: "step-2", Content: "Second updated", Status: PlanStatusPending},
		{ID: "step-3", Content: "Third", Status: PlanStatusPending},
	}, merged.Steps)
	require.Equal(t, "updated", merged.Explanation)
}

func TestMemoryPlanStore_ClearRemovesPlan(t *testing.T) {
	store := &MemoryPlanStore{}
	_, err := store.Replace("session-1", Plan{
		Steps: []PlanStep{{ID: "step-1", Content: "First", Status: PlanStatusInProgress}},
	})
	require.NoError(t, err)

	cleared := store.Clear("session-1")

	require.Equal(t, Plan{}, cleared)
	require.Equal(t, Plan{}, store.Get("session-1"))
}

func TestMemoryPlanStore_ReplaceRejectsInvalidPlan(t *testing.T) {
	store := &MemoryPlanStore{}

	plan, err := store.Replace("session-1", Plan{
		Steps: []PlanStep{{ID: "step-1", Content: "", Status: PlanStatusInProgress}},
	})

	require.Equal(t, Plan{}, plan)
	require.EqualError(t, err, "step 0 content is required")
	require.Equal(t, Plan{}, store.Get("session-1"))
}

func TestMemoryPlanStore_MergeUpdatesStatusAndClearsCompleted(t *testing.T) {
	store := &MemoryPlanStore{}
	_, err := store.Replace("", Plan{
		Steps: []PlanStep{
			{ID: "step-1", Content: "First", Status: PlanStatusInProgress},
			{ID: "step-2", Content: "Second", Status: PlanStatusPending},
			{ID: "step-3", Content: "Third", Status: PlanStatusCompleted},
		},
	})
	require.NoError(t, err)

	merged, err := store.Merge("", []PartialPlanStep{
		{ID: "step-1", Status: new(PlanStatusCompleted)},
		{ID: "step-2", Status: new(PlanStatusInProgress)},
	}, "trimmed", true)

	require.NoError(t, err)
	require.Equal(t, Plan{
		Steps: []PlanStep{
			{ID: "step-2", Content: "Second", Status: PlanStatusInProgress},
		},
		Explanation: "trimmed",
	}, merged)
	require.Equal(t, merged, store.Get(""))
}

func TestMemoryPlanStore_MergeRejectsInvalidPlan(t *testing.T) {
	store := &MemoryPlanStore{}
	_, err := store.Replace("session-1", Plan{
		Steps: []PlanStep{
			{ID: "step-1", Content: "First", Status: PlanStatusInProgress},
			{ID: "step-2", Content: "Second", Status: PlanStatusPending},
		},
	})
	require.NoError(t, err)

	merged, err := store.Merge("session-1", []PartialPlanStep{
		{ID: "step-1", Status: new(PlanStatusCompleted)},
	}, "invalid", false)

	require.Equal(t, Plan{}, merged)
	require.EqualError(t, err, "exactly one step must be in_progress while active work remains")
	require.Equal(t, Plan{
		Steps: []PlanStep{
			{ID: "step-1", Content: "First", Status: PlanStatusInProgress},
			{ID: "step-2", Content: "Second", Status: PlanStatusPending},
		},
	}, store.Get("session-1"))
}

func TestMemoryPlanStore_MergeInitializesEmptyStore(t *testing.T) {
	store := &MemoryPlanStore{}

	merged, err := store.Merge("session-1", []PartialPlanStep{
		{ID: "step-1", Content: new("First"), Status: new(PlanStatusInProgress)},
	}, "created", false)

	require.NoError(t, err)
	require.Equal(t, Plan{
		Steps:       []PlanStep{{ID: "step-1", Content: "First", Status: PlanStatusInProgress}},
		Explanation: "created",
	}, merged)
}

func TestMemoryPlanStore_ClearEmptyStoreReturnsEmptyPlan(t *testing.T) {
	store := &MemoryPlanStore{}

	require.Equal(t, Plan{}, store.Clear("session-1"))
}

func TestMemoryPlanStore_HydrateStoresDefensiveCopy(t *testing.T) {
	store := &MemoryPlanStore{}
	plan := Plan{
		Steps:       []PlanStep{{ID: "step-1", Content: "First", Status: PlanStatusInProgress}},
		Explanation: "loaded",
	}

	store.Hydrate("session-1", plan)
	plan.Steps[0].Content = "mutated"

	current := store.Get("session-1")
	require.Equal(t, Plan{
		Steps:       []PlanStep{{ID: "step-1", Content: "First", Status: PlanStatusInProgress}},
		Explanation: "loaded",
	}, current)
}

func TestMemoryPlanStore_ValidatePlanRejectsMissingID(t *testing.T) {
	err := ValidatePlan(Plan{
		Steps: []PlanStep{{ID: " ", Content: "First", Status: PlanStatusInProgress}},
	})

	require.EqualError(t, err, "step 0 id is required")
}

func TestMemoryPlanStore_ValidatePlanRejectsInvalidStatus(t *testing.T) {
	err := ValidatePlan(Plan{
		Steps: []PlanStep{{ID: "step-1", Content: "First", Status: "unknown"}},
	})

	require.EqualError(t, err, "step 0 status is invalid")
}

func TestMemoryPlanStore_ValidatePlanRejectsMultipleInProgressSteps(t *testing.T) {
	err := ValidatePlan(Plan{
		Steps: []PlanStep{
			{ID: "step-1", Content: "First", Status: PlanStatusInProgress},
			{ID: "step-2", Content: "Second", Status: PlanStatusInProgress},
		},
	})

	require.EqualError(t, err, "only one step may be in_progress")
}

func TestMemoryPlanStore_ValidatePlanAllowsTerminalOnlyPlan(t *testing.T) {
	err := ValidatePlan(Plan{
		Steps: []PlanStep{
			{ID: "step-1", Content: "First", Status: PlanStatusCompleted},
			{ID: "step-2", Content: "Second", Status: PlanStatusCancelled},
		},
	})

	require.NoError(t, err)
}
