package common

import (
	"context"
	"fmt"
	"math/rand"
	"time"
)

const (
	// UnlimitedNumAttemptsValue is the value for indicating unlimited attempts for reaching success
	UnlimitedNumAttemptsValue = uint(0)

	// number of characters contained in the generated retry token
	generatedRetryTokenLength = 30
)

// OCIRetryableRequest represents a request that can be reissued according to the specified policy.
type OCIRetryableRequest interface {
	// Any retryable request must implement the OCIRequest interface
	OCIRequest

	// Each operation specifies default retry behavior. By passing no arguments to this method, the default retry
	// behavior, as determined on a per-operation-basis, will be honored. Variadic retry policy option arguments
	// passed to this method will override the default behavior.
	RetryPolicy() *RetryPolicy
}

// OCIOperationResponse represents the output of an OCIOperation, with additional context of error message
// and operation attempt number.
type OCIOperationResponse struct {
	// Response from OCI Operation
	Response OCIResponse

	// Error from OCI Operation
	Error error

	// Operation Attempt Number (one-based)
	AttemptNumber uint
}

// NewOCIOperationResponse assembles an OCI Operation Response object.
func NewOCIOperationResponse(response OCIResponse, err error, attempt uint) OCIOperationResponse {
	return OCIOperationResponse{
		Response:      response,
		Error:         err,
		AttemptNumber: attempt,
	}
}

// RetryPolicy is the class that holds all relevant information for retrying operations.
type RetryPolicy struct {
	// MaximumNumberAttempts is the maximum number of times to retry a request. Zero indicates an unlimited
	// number of attempts.
	MaximumNumberAttempts uint

	// ShouldRetryOperation inspects the http response, error, and operation attempt number, and
	// - returns true if we should retry the operation
	// - returns false otherwise
	ShouldRetryOperation func(OCIOperationResponse) bool

	// GetNextDuration computes the duration to pause between operation retries.
	NextDuration func(OCIOperationResponse) time.Duration
}

// NoRetryPolicy is a helper method that assembles and returns a return policy that indicates an operation should
// never be retried (the operation is performed exactly once).
func NoRetryPolicy() RetryPolicy {
	dontRetryOperation := func(OCIOperationResponse) bool { return false }
	zeroNextDuration := func(OCIOperationResponse) time.Duration { return 0 * time.Second }
	return NewRetryPolicy(uint(1), dontRetryOperation, zeroNextDuration)
}

// NewRetryPolicy is a helper method for assembling a Retry Policy object.
func NewRetryPolicy(attempts uint, retryOperation func(OCIOperationResponse) bool, nextDuration func(OCIOperationResponse) time.Duration) RetryPolicy {
	return RetryPolicy{
		MaximumNumberAttempts: attempts,
		ShouldRetryOperation:  retryOperation,
		NextDuration:          nextDuration,
	}
}

// shouldContinueIssuingRequests returns true if we should continue retrying a request, based on the current attempt
// number and the maximum number of attempts specified, or false otherwise.
func shouldContinueIssuingRequests(current, maximum uint) bool {
	return maximum == UnlimitedNumAttemptsValue || current <= maximum
}

// generateRetryToken generates a retry token that must be included on any request passed to the Retry method.
func generateRetryToken() string {
	alphanumericChars := []rune("abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ")
	retryToken := make([]rune, generatedRetryTokenLength)
	for i := range retryToken {
		retryToken[i] = alphanumericChars[rand.Intn(len(alphanumericChars))]
	}
	return string(retryToken)
}

// Retry is a package-level operation that executes the retryable request using the specified operation and retry policy.
func Retry(ctx context.Context, request OCIRetryableRequest, operation OCIOperation, policy RetryPolicy) (OCIResponse, error) {
	// use a one-based counter because it's easier to think about operation retry in terms of attempt numbering
	for currentOperationAttempt := uint(1); shouldContinueIssuingRequests(currentOperationAttempt, policy.MaximumNumberAttempts); currentOperationAttempt++ {
		Debugln(fmt.Sprintf("operation attempt #%v", currentOperationAttempt))
		response, err := operation(ctx, request)
		operationResponse := NewOCIOperationResponse(response, err, currentOperationAttempt)

		select {
		case <-ctx.Done():
			// return why the request was aborted (could be user interrupted or deadline exceeded)
			// => include last received response for information (user may choose to re-issue request)
			return response, ctx.Err()
		default:
			// non-blocking select
		}

		if policy.ShouldRetryOperation(operationResponse) {
			// this conditional is explicitly not added to the encompassing if condition to retry based on response
			// => it is only to determine if, on the last round of this loop, we still skip sleeping (if we're the
			//    last attempt, then there's no point sleeping before we round the loop again and fall out to the
			//    Maximum Number Attempts exceeded error)
			if currentOperationAttempt != policy.MaximumNumberAttempts {
				// sleep before retrying the operation
				duration := policy.NextDuration(operationResponse)
				if deadline, ok := ctx.Deadline(); ok && time.Now().Add(duration).After(deadline) {
					// we want to retry the operation, but the policy is telling us to wait for a duration that exceeds
					// the specified overall deadline for the operation => instead of waiting for however long that
					// time period is and then aborting, abort now and save the cycles
					return response, DeadlineExceededByBackoff
				}
				Debugln(fmt.Sprintf("waiting %v before retrying operation", duration))
				time.Sleep(duration)
			}
		} else {
			// we should NOT retry operation based on response and/or error => return
			return response, err
		}
	}
	return nil, fmt.Errorf("maximum number of attempts exceeded (%v)", policy.MaximumNumberAttempts)
}
