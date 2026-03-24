package environment

// IterationBudget represents the budget for the number of iterations the agent can make.
type IterationBudget struct {
	remaining int
}

// NewIterationBudget creates a new iteration budget with the given limit.
func NewIterationBudget(limit int) IterationBudget {
	return IterationBudget{remaining: limit}
}

// Consume consumes one iteration from the budget.
func (b *IterationBudget) Consume() bool {
	if b == nil || b.remaining <= 0 {
		return false
	}
	b.remaining--
	return true
}

// Remaining returns the number of iterations remaining in the budget.
func (b IterationBudget) Remaining() int {
	return b.remaining
}


