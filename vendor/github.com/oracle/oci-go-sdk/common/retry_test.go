package common

import (
	"github.com/stretchr/testify/assert"
	"math"
	"net/http"
	"testing"
	"time"
)

// testing resource for mocking responses
type mockedResponse struct {
	RawResponse *http.Response
}

// HTTPResponse implements the OCIResponse interface
func (response mockedResponse) HTTPResponse() *http.Response {
	return response.RawResponse
}

func getMockedOCIOperationResponse(statusCode int, attemptNumber uint) OCIOperationResponse {
	httpResponse := http.Response{
		Header:     http.Header{},
		StatusCode: statusCode,
	}
	response := mockedResponse{
		RawResponse: &httpResponse,
	}
	return NewOCIOperationResponse(response, nil, attemptNumber)
}

func getExponentialBackoffRetryPolicy(attempts uint) RetryPolicy {
	shouldRetry := func(OCIOperationResponse) bool {
		return true
	}
	nextDuration := func(response OCIOperationResponse) time.Duration {
		return time.Duration(math.Pow(float64(2), float64(response.AttemptNumber-1))) * time.Second
	}
	return NewRetryPolicy(attempts, shouldRetry, nextDuration)
}

func TestNoRetryPolicyDefaults(t *testing.T) {
	response := getMockedOCIOperationResponse(200, 1)
	policy := NoRetryPolicy()
	assert.False(t, policy.ShouldRetryOperation(response))
}

func TestShouldContinueIssuingRequests(t *testing.T) {
	assert.True(t, shouldContinueIssuingRequests(uint(1), uint(2)))
	assert.True(t, shouldContinueIssuingRequests(uint(2), uint(2)))
	assert.True(t, shouldContinueIssuingRequests(uint(150), UnlimitedNumAttemptsValue))
}

func TestRetryPolicyExponentialBackoffNextDurationUnrolled(t *testing.T) {
	responses := []OCIOperationResponse{
		getMockedOCIOperationResponse(500, 1),
		getMockedOCIOperationResponse(500, 2),
		getMockedOCIOperationResponse(500, 3),
		getMockedOCIOperationResponse(500, 4),
		getMockedOCIOperationResponse(500, 5),
	}
	policy := getExponentialBackoffRetryPolicy(5)
	// unroll an exponential retry policy with a specified maximum
	// number of attempts so it's more obvious what's happening
	// request #1
	assert.True(t, shouldContinueIssuingRequests(1, policy.MaximumNumberAttempts))
	assert.True(t, policy.ShouldRetryOperation(responses[0]))
	assert.Equal(t, 1*time.Second, policy.NextDuration(responses[0]))
	// request #2
	assert.True(t, shouldContinueIssuingRequests(2, policy.MaximumNumberAttempts))
	assert.True(t, policy.ShouldRetryOperation(responses[1]))
	assert.Equal(t, 2*time.Second, policy.NextDuration(responses[1]))
	// request #3
	assert.True(t, shouldContinueIssuingRequests(3, policy.MaximumNumberAttempts))
	assert.True(t, policy.ShouldRetryOperation(responses[2]))
	assert.Equal(t, 4*time.Second, policy.NextDuration(responses[2]))
	// request #4
	assert.True(t, shouldContinueIssuingRequests(4, policy.MaximumNumberAttempts))
	assert.True(t, policy.ShouldRetryOperation(responses[3]))
	assert.Equal(t, 8*time.Second, policy.NextDuration(responses[3]))
	// request #5
	assert.True(t, shouldContinueIssuingRequests(5, policy.MaximumNumberAttempts))
	assert.True(t, policy.ShouldRetryOperation(responses[4]))
	assert.Equal(t, 16*time.Second, policy.NextDuration(responses[4]))
	// done
	assert.False(t, shouldContinueIssuingRequests(6, policy.MaximumNumberAttempts))
}
