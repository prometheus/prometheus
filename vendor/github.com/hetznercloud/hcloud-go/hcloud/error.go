package hcloud

import "fmt"

// ErrorCode represents an error code returned from the API.
type ErrorCode string

// Error codes returned from the API.
const (
	ErrorCodeServiceError          ErrorCode = "service_error"           // Generic service error
	ErrorCodeRateLimitExceeded     ErrorCode = "rate_limit_exceeded"     // Rate limit exceeded
	ErrorCodeUnknownError          ErrorCode = "unknown_error"           // Unknown error
	ErrorCodeNotFound              ErrorCode = "not_found"               // Resource not found
	ErrorCodeInvalidInput          ErrorCode = "invalid_input"           // Validation error
	ErrorCodeForbidden             ErrorCode = "forbidden"               // Insufficient permissions
	ErrorCodeJSONError             ErrorCode = "json_error"              // Invalid JSON in request
	ErrorCodeLocked                ErrorCode = "locked"                  // Item is locked (Another action is running)
	ErrorCodeResourceLimitExceeded ErrorCode = "resource_limit_exceeded" // Resource limit exceeded
	ErrorCodeResourceUnavailable   ErrorCode = "resource_unavailable"    // Resource currently unavailable
	ErrorCodeUniquenessError       ErrorCode = "uniqueness_error"        // One or more fields must be unique
	ErrorCodeProtected             ErrorCode = "protected"               // The actions you are trying is protected
	ErrorCodeMaintenance           ErrorCode = "maintenance"             // Cannot perform operation due to maintenance
	ErrorCodeConflict              ErrorCode = "conflict"                // The resource has changed during the request, please retry
	ErrorCodeServerAlreadyAttached ErrorCode = "server_already_attached" // The server is already attached to the resource

	// Deprecated error codes

	// The actual value of this error code is limit_reached. The new error code
	// rate_limit_exceeded for ratelimiting was introduced before Hetzner Cloud
	// launched into the public. To make clients using the old error code still
	// work as expected, we set the value of the old error code to that of the
	// new error code.
	ErrorCodeLimitReached = ErrorCodeRateLimitExceeded
)

// Error is an error returned from the API.
type Error struct {
	Code    ErrorCode
	Message string
	Details interface{}
}

func (e Error) Error() string {
	return fmt.Sprintf("%s (%s)", e.Message, e.Code)
}

// ErrorDetailsInvalidInput contains the details of an 'invalid_input' error.
type ErrorDetailsInvalidInput struct {
	Fields []ErrorDetailsInvalidInputField
}

// ErrorDetailsInvalidInputField contains the validation errors reported on a field.
type ErrorDetailsInvalidInputField struct {
	Name     string
	Messages []string
}

// IsError returns whether err is an API error with the given error code.
func IsError(err error, code ErrorCode) bool {
	apiErr, ok := err.(Error)
	return ok && apiErr.Code == code
}
