// Package jsonapi provides shared JSON:API response types.
package jsonapi

// CollectionResponse is a JSON:API envelope for resource collections.
type CollectionResponse[T any] struct {
	Meta  *PaginationMeta  `json:"meta,omitempty"`
	Data  []T              `json:"data"`
	Links *PaginationLinks `json:"links,omitempty"`
}

// SingleResponse is a JSON:API envelope for a single resource.
type SingleResponse[T any] struct {
	Data T `json:"data"`
}

// PaginationMeta contains pagination metadata.
type PaginationMeta struct {
	TotalCount int `json:"total_count"`
	Offset     int `json:"offset"`
	Limit      int `json:"limit"`
}

// PaginationLinks contains pagination links.
type PaginationLinks struct {
	Self  string  `json:"self"`
	First string  `json:"first"`
	Last  string  `json:"last"`
	Prev  *string `json:"prev,omitempty"`
	Next  *string `json:"next,omitempty"`
}

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
