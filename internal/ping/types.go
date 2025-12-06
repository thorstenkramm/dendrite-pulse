package ping

// Response represents the JSON:API payload for the ping endpoint.
type Response struct {
	Meta  PaginationMeta  `json:"meta"`
	Links PaginationLinks `json:"links"`
	Data  Resource        `json:"data"`
}

// PaginationMeta contains paging information for JSON:API responses.
type PaginationMeta struct {
	Page PageInfo `json:"page"`
}

// PageInfo holds pagination counters.
type PageInfo struct {
	CurrentPage int `json:"currentPage"`
	From        int `json:"from"`
	LastPage    int `json:"lastPage"`
	PerPage     int `json:"perPage"`
	To          int `json:"to"`
	Total       int `json:"total"`
}

// PaginationLinks contains pagination URLs.
type PaginationLinks struct {
	Self  string `json:"self"`
	First string `json:"first"`
	Last  string `json:"last"`
	Next  string `json:"next,omitempty"`
	Prev  string `json:"prev,omitempty"`
}

// Resource is the JSON:API resource representing a ping response.
type Resource struct {
	Type       string     `json:"type"`
	ID         string     `json:"id"`
	Attributes Attributes `json:"attributes"`
}

// Attributes holds ping-specific attributes.
type Attributes struct {
	Message string `json:"message"`
}
