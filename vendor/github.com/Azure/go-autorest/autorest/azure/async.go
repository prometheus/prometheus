package azure

import (
	"fmt"
	"net/http"
	"time"

	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/date"
)

const (
	// HeaderAsyncOperation is the Azure header providing the URL from which to obtain the
	// OperationResource for an asynchronous (aka long running) operation.
	HeaderAsyncOperation = "Azure-AsyncOperation"
)

const (
	// OperationCanceled says the underlying operation was canceled.
	OperationCanceled string = "Canceled"
	// OperationFailed says the underlying operation failed.
	OperationFailed string = "Failed"
	// OperationSucceeded says the underlying opertion succeeded.
	OperationSucceeded string = "Succeeded"
)

// OperationError provides additional detail when an operation fails or is canceled.
type OperationError struct {
	// Code provides an invariant error code useful for troubleshooting, aggregration, and analysis.
	Code string `json:"code"`
	// Message indicates what error occurred and what can be done to address the issue.
	Message string `json:"message"`
}

// Error implements the error interface returnin a string containing the code and message.
func (oe OperationError) Error() string {
	return fmt.Sprintf("Azure Operation Error: Code=%q Message=%q", oe.Code, oe.Message)
}

// OperationResource defines a resource describing the state of a long-running operation.
type OperationResource struct {
	// Id is the identifier used in a GET for the underlying resource.
	ID string `json:"id"`
	// Name matches the last segment of the Id field (typically system generated).
	Name string `json:"name"`
	// Status provides the state of the operation. Non-terminal states vary by resource;
	Status string `json:"status"`
	// Properties, on operation success, optionally contains per-service / per-resource values.
	Properties map[string]interface{} `json:"properties"`
	// Error provides additional detail if the operation is canceled or failed.
	OperationError OperationError `json:"error"`
	// StartTime optionally provides the time the operation started.
	StartTime date.Time `json:"startTime"`
	// EndTime optionally provides the time the operation completed.
	EndTime date.Time `json:"endTime"`
	// PercentComplete optionally provides the percent complete between 0 and 100.
	PercentComplete float64 `json:"percentComplete"`
}

// HasSucceeded returns true if the operation has succeeded; false otherwise.
func (or OperationResource) HasSucceeded() bool {
	return or.Status == OperationSucceeded
}

// HasTerminated returns true if the operation has terminated; false otherwise.
func (or OperationResource) HasTerminated() bool {
	switch or.Status {
	case OperationCanceled, OperationFailed, OperationSucceeded:
		return true
	default:
		return false
	}
}

// GetError returns an error if the operation was canceled or failed and nil otherwise.
func (or OperationResource) GetError() error {
	switch or.Status {
	case OperationCanceled, OperationFailed:
		return or.OperationError
	default:
		return nil
	}
}

// GetAsyncOperation retrieves the long-running URL from which to retrieve the OperationResource.
func GetAsyncOperation(resp *http.Response) string {
	return resp.Header.Get(http.CanonicalHeaderKey(HeaderAsyncOperation))
}

// IsAsynchronousResponse returns true if the passed response indicates that the request will
// complete asynchronously. Such responses have either an http.StatusCreated or an
// http.StatusAccepted status code and provide the Azure-AsyncOperation header.
func IsAsynchronousResponse(resp *http.Response) bool {
	return autorest.ResponseHasStatusCode(resp, http.StatusCreated, http.StatusAccepted) &&
		GetAsyncOperation(resp) != ""
}

// NewOperationResourceRequest allocates and returns a new http.Request to retrieve the
// OperationResource for an asynchronous operation.
func NewOperationResourceRequest(resp *http.Response, cancel <-chan struct{}) (*http.Request, error) {
	location := GetAsyncOperation(resp)
	if location == "" {
		return nil, autorest.NewErrorWithResponse("azure", "NewOperationResourceRequest", resp, "Azure-AsyncOperation header missing from response that requires polling")
	}

	req, err := autorest.Prepare(&http.Request{Cancel: cancel},
		autorest.AsGet(),
		autorest.WithBaseURL(location))
	if err != nil {
		return nil, autorest.NewErrorWithError(err, "azure", "NewOperationResourceRequest", nil, "Failure creating poll request to %s", location)
	}

	return req, nil
}

// DoPollForAsynchronous returns a SendDecorator that polls if the http.Response is for an Azure
// long-running operation. It will poll until the time passed is equal to or greater than
// the supplied duration. It will delay between requests for the duration specified in the
// RetryAfter header or, if the header is absent, the passed delay. Polling may be canceled by
// closing the optional channel on the http.Request.
func DoPollForAsynchronous(duration time.Duration, delay time.Duration) autorest.SendDecorator {
	return func(s autorest.Sender) autorest.Sender {
		return autorest.SenderFunc(func(r *http.Request) (resp *http.Response, err error) {
			resp, err = s.Do(r)
			if err != nil || !IsAsynchronousResponse(resp) {
				return resp, err
			}

			r, err = NewOperationResourceRequest(resp, r.Cancel)
			if err != nil {
				return resp, autorest.NewErrorWithError(err, "azure", "DoPollForAsynchronous", resp, "Failure occurred creating OperationResource request")
			}

			or := &OperationResource{}
			for err == nil && !or.HasTerminated() {
				autorest.Respond(resp,
					autorest.ByClosing())

				resp, err = autorest.SendWithSender(s, r,
					autorest.AfterDelay(autorest.GetRetryAfter(resp, delay)))
				if err != nil {
					return resp, autorest.NewErrorWithError(err, "azure", "DoPollForAsynchronous", resp, "Failure occurred retrieving OperationResource")
				}

				err = autorest.Respond(resp,
					autorest.ByUnmarshallingJSON(or))
				if err != nil {
					return resp, autorest.NewErrorWithError(err, "azure", "DoPollForAsynchronous", resp, "Failure occurred unmarshalling an OperationResource")
				}
			}

			if err == nil && or.HasTerminated() {
				err = or.GetError()
			}

			return resp, err
		})
	}
}
