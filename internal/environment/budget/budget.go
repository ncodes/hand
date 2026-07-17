package budget

// IterationBudget describes iteration budget.
type IterationBudget struct {
	remaining int
}

// New returns an iteration budget with the supplied limit.
func New(limit int) IterationBudget {
	return IterationBudget{remaining: limit}
}

func (b *IterationBudget) Consume() bool {
	if b == nil || b.remaining <= 0 {
		return false
	}

	b.remaining--
	return true
}

func (b IterationBudget) Remaining() int {
	return b.remaining
}
