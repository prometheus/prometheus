package schema

import "encoding/json"

// Error represents the schema of an error response.
type Error struct {
	Code       string          `json:"code"`
	Message    string          `json:"message"`
	DetailsRaw json.RawMessage `json:"details"`
	Details    interface{}
}

// UnmarshalJSON overrides default json unmarshalling.
func (e *Error) UnmarshalJSON(data []byte) (err error) {
	type Alias Error
	alias := (*Alias)(e)
	if err = json.Unmarshal(data, alias); err != nil {
		return
	}
	switch e.Code {
	case "invalid_input":
		details := ErrorDetailsInvalidInput{}
		if err = json.Unmarshal(e.DetailsRaw, &details); err != nil {
			return
		}
		alias.Details = details
	}
	return
}

// ErrorResponse defines the schema of a response containing an error.
type ErrorResponse struct {
	Error Error `json:"error"`
}

// ErrorDetailsInvalidInput defines the schema of the Details field
// of an error with code 'invalid_input'.
type ErrorDetailsInvalidInput struct {
	Fields []struct {
		Name     string   `json:"name"`
		Messages []string `json:"messages"`
	} `json:"fields"`
}
