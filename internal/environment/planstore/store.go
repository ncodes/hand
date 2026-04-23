package planstore

import (
	"strings"
	"sync"
)

type Store interface {
	Get(string) Plan
	Replace(string, Plan) (Plan, error)
	Merge(string, []PartialPlanStep, string, bool) (Plan, error)
	Clear(string) Plan
	Hydrate(string, Plan)
}

type MemoryPlanStore struct {
	mu    sync.Mutex
	plans map[string]Plan
}

func (s *MemoryPlanStore) Get(sessionID string) Plan {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s == nil || len(s.plans) == 0 {
		return Plan{}
	}

	return ClonePlan(s.plans[normalizeSessionID(sessionID)])
}

func (s *MemoryPlanStore) Replace(sessionID string, plan Plan) (Plan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.plans == nil {
		s.plans = make(map[string]Plan)
	}

	if err := ValidatePlan(plan); err != nil {
		return Plan{}, err
	}

	normalized := normalizeSessionID(sessionID)
	cloned := ClonePlan(plan)
	s.plans[normalized] = cloned
	return ClonePlan(cloned), nil
}

func (s *MemoryPlanStore) Merge(
	sessionID string,
	updates []PartialPlanStep,
	explanation string,
	clearCompleted bool,
) (Plan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.plans == nil {
		s.plans = make(map[string]Plan)
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

		step := PlanStep{ID: strings.TrimSpace(update.ID)}
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
			if step.Status == PlanStatusCompleted || step.Status == PlanStatusCancelled {
				continue
			}
			filtered = append(filtered, step)
		}
		current.Steps = append([]PlanStep(nil), filtered...)
	}

	if err := ValidatePlan(current); err != nil {
		return Plan{}, err
	}

	s.plans[normalized] = ClonePlan(current)

	return ClonePlan(current), nil
}

func (s *MemoryPlanStore) Clear(sessionID string) Plan {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.plans == nil {
		return Plan{}
	}

	normalized := normalizeSessionID(sessionID)
	delete(s.plans, normalized)
	return Plan{}
}

func (s *MemoryPlanStore) Hydrate(sessionID string, plan Plan) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.plans == nil {
		s.plans = make(map[string]Plan)
	}

	s.plans[normalizeSessionID(sessionID)] = ClonePlan(plan)
}

func ClonePlan(plan Plan) Plan {
	cloned := Plan{Explanation: plan.Explanation}
	if len(plan.Steps) > 0 {
		cloned.Steps = append([]PlanStep(nil), plan.Steps...)
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
