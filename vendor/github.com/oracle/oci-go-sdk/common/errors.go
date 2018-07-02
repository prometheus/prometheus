// Copyright (c) 2016, 2018, Oracle and/or its affiliates. All rights reserved.

package common

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
)

// ServiceError models all potential errors generated the service call
type ServiceError interface {
	// The http status code of the error
	GetHTTPStatusCode() int

	// The human-readable error string as sent by the service
	GetMessage() string

	// A short error code that defines the error, meant for programmatic parsing.
	// See https://docs.us-phoenix-1.oraclecloud.com/Content/API/References/apierrors.htm
	GetCode() string
}

type servicefailure struct {
	StatusCode int
	Code       string `json:"code,omitempty"`
	Message    string `json:"message,omitempty"`
}

func newServiceFailureFromResponse(response *http.Response) error {
	var err error

	//If there is an error consume the body, entirely
	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return servicefailure{
			StatusCode: response.StatusCode,
			Code:       "BadErrorResponse",
			Message:    fmt.Sprintf("The body of the response was not readable, due to :%s", err.Error()),
		}
	}

	se := servicefailure{StatusCode: response.StatusCode}
	err = json.Unmarshal(body, &se)
	if err != nil {
		Debugf("Error response could not be parsed due to: %s", err.Error())
		return servicefailure{
			StatusCode: response.StatusCode,
			Code:       "BadErrorResponse",
			Message:    fmt.Sprintf("Error while parsing failure from response"),
		}
	}
	return se
}

func (se servicefailure) Error() string {
	return fmt.Sprintf("Service error:%s. %s. http status code: %d",
		se.Code, se.Message, se.StatusCode)
}

func (se servicefailure) GetHTTPStatusCode() int {
	return se.StatusCode

}

func (se servicefailure) GetMessage() string {
	return se.Message
}

func (se servicefailure) GetCode() string {
	return se.Code
}

// IsServiceError returns false if the error is not service side, otherwise true
// additionally it returns an interface representing the ServiceError
func IsServiceError(err error) (failure ServiceError, ok bool) {
	failure, ok = err.(servicefailure)
	return
}

type deadlineExceededByBackoffError struct{}

func (deadlineExceededByBackoffError) Error() string {
	return "now() + computed backoff duration exceeds request deadline"
}

// DeadlineExceededByBackoff is the error returned by Call() when GetNextDuration() returns a time.Duration that would
// force the user to wait past the request deadline before re-issuing a request. This enables us to exit early, since
// we cannot succeed based on the configured retry policy.
var DeadlineExceededByBackoff error = deadlineExceededByBackoffError{}
