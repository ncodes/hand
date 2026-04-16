package planstore

import (
	"strings"
	"sync"

	envtypes "github.com/wandxy/hand/internal/environment/types"
)

type Store interface {
	Get(string) envtypes.Plan
	Replace(string, envtypes.Plan) (envtypes.Plan, error)
	Merge(string, []envtypes.PartialPlanStep, string, bool) (envtypes.Plan, error)
	Clear(string) envtypes.Plan
	Hydrate(string, envtypes.Plan)
}

type MemoryPlanStore struct {
	mu    sync.Mutex
	plans map[string]envtypes.Plan
}

func (s *MemoryPlanStore) Get(sessionID string) envtypes.Plan {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s == nil || len(s.plans) == 0 {
		return envtypes.Plan{}
	}

	return ClonePlan(s.plans[normalizeSessionID(sessionID)])
}

func (s *MemoryPlanStore) Replace(sessionID string, plan envtypes.Plan) (envtypes.Plan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.plans == nil {
		s.plans = make(map[string]envtypes.Plan)
	}

	if err := envtypes.ValidatePlan(plan); err != nil {
		return envtypes.Plan{}, err
	}

	normalized := normalizeSessionID(sessionID)
	cloned := ClonePlan(plan)
	s.plans[normalized] = cloned
	return ClonePlan(cloned), nil
}

func (s *MemoryPlanStore) Merge(
	sessionID string,
	updates []envtypes.PartialPlanStep,
	explanation string,
	clearCompleted bool,
) (envtypes.Plan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.plans == nil {
		s.plans = make(map[string]envtypes.Plan)
	}

	normalized := normalizeSessionID(sessionID)
	current := ClonePlan(s.plans[normalized])
	indexByID := make(map[string]int, len(current.Steps))
	for idx, step := range current.Steps {
		indexByID[step.ID] = idx
	}

	for _, update := range updates {
		if idx, ok := indexByID[update.ID]; ok {
			if update.Content != nil {
				current.Steps[idx].Content = strings.TrimSpace(*update.Content)
			}
			if update.Status != nil {
				current.Steps[idx].Status = strings.TrimSpace(*update.Status)
			}
			continue
		}

		step := envtypes.PlanStep{ID: strings.TrimSpace(update.ID)}
		if update.Content != nil {
			step.Content = strings.TrimSpace(*update.Content)
		}
		if update.Status != nil {
			step.Status = strings.TrimSpace(*update.Status)
		}
		current.Steps = append(current.Steps, step)
		indexByID[step.ID] = len(current.Steps) - 1
	}

	current.Explanation = strings.TrimSpace(explanation)
	if clearCompleted {
		filtered := current.Steps[:0]
		for _, step := range current.Steps {
			if step.Status == envtypes.PlanStatusCompleted || step.Status == envtypes.PlanStatusCancelled {
				continue
			}
			filtered = append(filtered, step)
		}
		current.Steps = append([]envtypes.PlanStep(nil), filtered...)
	}

	if err := envtypes.ValidatePlan(current); err != nil {
		return envtypes.Plan{}, err
	}

	s.plans[normalized] = ClonePlan(current)

	return ClonePlan(current), nil
}

func (s *MemoryPlanStore) Clear(sessionID string) envtypes.Plan {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.plans == nil {
		return envtypes.Plan{}
	}

	normalized := normalizeSessionID(sessionID)
	delete(s.plans, normalized)
	return envtypes.Plan{}
}

func (s *MemoryPlanStore) Hydrate(sessionID string, plan envtypes.Plan) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.plans == nil {
		s.plans = make(map[string]envtypes.Plan)
	}

	s.plans[normalizeSessionID(sessionID)] = ClonePlan(plan)
}

func ClonePlan(plan envtypes.Plan) envtypes.Plan {
	cloned := envtypes.Plan{Explanation: plan.Explanation}
	if len(plan.Steps) > 0 {
		cloned.Steps = append([]envtypes.PlanStep(nil), plan.Steps...)
	}
	return cloned
}

func normalizeSessionID(sessionID string) string {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return "default"
	}
	return sessionID
}
