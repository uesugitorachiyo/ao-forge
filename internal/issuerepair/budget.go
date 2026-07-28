package issuerepair

type Budget struct {
	MaxPages   int  `json:"max_pages"`
	PagesUsed  int  `json:"pages_used"`
	MaxWrites  int  `json:"max_writes"`
	WritesUsed int  `json:"writes_used"`
	Exhausted  bool `json:"exhausted"`
}

func (budget *Budget) consumePage() error {
	if budget.Exhausted || budget.PagesUsed >= budget.MaxPages {
		budget.Exhausted = true
		return ErrBudgetExhausted
	}
	budget.PagesUsed++
	return nil
}

func (budget *Budget) consumeWrite() error {
	if budget.Exhausted || budget.WritesUsed >= budget.MaxWrites {
		budget.Exhausted = true
		return ErrBudgetExhausted
	}
	budget.WritesUsed++
	return nil
}
