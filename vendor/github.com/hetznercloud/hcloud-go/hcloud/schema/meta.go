package schema

// Meta defines the schema of meta information which may be included
// in responses.
type Meta struct {
	Pagination *MetaPagination `json:"pagination"`
}

// MetaPagination defines the schema of pagination information.
type MetaPagination struct {
	Page         int `json:"page"`
	PerPage      int `json:"per_page"`
	PreviousPage int `json:"previous_page"`
	NextPage     int `json:"next_page"`
	LastPage     int `json:"last_page"`
	TotalEntries int `json:"total_entries"`
}

// MetaResponse defines the schema of a response containing
// meta information.
type MetaResponse struct {
	Meta Meta `json:"meta"`
}
