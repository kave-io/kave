package contract

// SchemaVersion is the public JSON contract version.
const SchemaVersion = 1

// Page describes cursor pagination metadata on list responses.
type Page struct {
	NextCursor *string `json:"next_cursor"`
	Limit      int     `json:"limit"`
}

// Warning is a non-fatal response warning.
type Warning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// SuccessEnvelope is the top-level JSON response shape for successful requests.
type SuccessEnvelope struct {
	SchemaVersion int       `json:"schema_version"`
	Kind          string    `json:"kind"`
	Data          any       `json:"data"`
	Page          *Page     `json:"page"`
	Warnings      []Warning `json:"warnings"`
}

// ErrorPayload is the machine-readable error body.
type ErrorPayload struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

// ErrorEnvelope is the top-level JSON response shape for failed requests.
type ErrorEnvelope struct {
	SchemaVersion int          `json:"schema_version"`
	Kind          string       `json:"kind"`
	Error         ErrorPayload `json:"error"`
}

// Money is the canonical money shape in JSON contracts.
type Money struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}
