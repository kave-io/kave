package cost

// PricingTable maps model names to their token costs (USD per million tokens).
type PricingTable struct {
	models map[string]ModelPricing
}

// ModelPricing holds input and output token costs for a model.
type ModelPricing struct {
	InputCost  float64 // USD per million input tokens
	OutputCost float64 // USD per million output tokens
}

// NewPricingTable creates a pricing table with default LLM model prices.
func NewPricingTable() *PricingTable {
	return &PricingTable{
		models: map[string]ModelPricing{
			// OpenAI GPT-4o
			"gpt-4o": {
				InputCost:  2.50,
				OutputCost: 10.00,
			},
			// OpenAI GPT-4o mini
			"gpt-4o-mini": {
				InputCost:  0.15,
				OutputCost: 0.60,
			},
			// Anthropic Claude Sonnet
			"claude-sonnet-4-5": {
				InputCost:  3.00,
				OutputCost: 15.00,
			},
			// Anthropic Claude Haiku
			"claude-haiku-4-5": {
				InputCost:  0.80,
				OutputCost: 4.00,
			},
			// Ollama models (local, free)
			"mistral": {
				InputCost:  0.00,
				OutputCost: 0.00,
			},
			"llama2": {
				InputCost:  0.00,
				OutputCost: 0.00,
			},
		},
	}
}

// GetPrice returns the pricing for a model, or zero cost if unknown.
func (p *PricingTable) GetPrice(model string) ModelPricing {
	if pricing, ok := p.models[model]; ok {
		return pricing
	}
	// Unknown models default to free (local/self-hosted)
	return ModelPricing{InputCost: 0.0, OutputCost: 0.0}
}

// Set registers or updates pricing for a model.
func (p *PricingTable) Set(model string, pricing ModelPricing) {
	p.models[model] = pricing
}

// CalculateCost computes the USD cost for token usage.
func CalculateCost(pricing ModelPricing, inputTokens, outputTokens int) float64 {
	inputCost := (float64(inputTokens) / 1_000_000) * pricing.InputCost
	outputCost := (float64(outputTokens) / 1_000_000) * pricing.OutputCost
	return inputCost + outputCost
}
