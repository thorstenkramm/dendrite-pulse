package server

// ErrorResponse represents JSON:API error envelopes.
type ErrorResponse struct {
	Errors []ErrorObject `json:"errors"`
}

// ErrorObject describes a single JSON:API error.
type ErrorObject struct {
	Status string `json:"status"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
}
