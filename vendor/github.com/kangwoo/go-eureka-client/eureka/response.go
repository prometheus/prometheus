package eureka

import "net/http"

type RawResponse struct {
	StatusCode int
	Body       []byte
	Header     http.Header
}

var (
	validHttpStatusCode = map[int]bool{
		http.StatusNoContent:          true,
		http.StatusCreated:            true,
		http.StatusOK:                 true,
		http.StatusBadRequest:         true,
		http.StatusNotFound:           true,
		http.StatusPreconditionFailed: true,
		http.StatusForbidden:          true,
	}
)
