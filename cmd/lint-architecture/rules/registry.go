package rules

func All() []Rule {
	return []Rule{
		&B1LayerDirection{},
		&B2NoSchedulers{},
		&B2NoTickers{},
		&B2EgressChokepoint{},
		&B3BudgetCardinality{},
		&B4NoAuthInPolicy{},
		&B4NoPolicyInAuth{},
		&B5FrameworkLLMSeparation{},
		&B6NoCrossStoreTx{},
		&B6SingleSpanWriter{},
		&B7NoFloatMoney{},
		&B8NoUUID{},
		&B8NoManualPrefix{},
		&B9NoTimeInModels{},
		&B10HTTPAllowlist{},
	}
}
