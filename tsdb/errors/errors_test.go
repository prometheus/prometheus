package errors

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

var anError = errors.New("test error")

func TestMultiError_Is(t *testing.T) {
	customErr := fmt.Errorf("my error")

	testCases := map[string]struct {
		sourceErrors []error
		target       error
		is           bool
	}{
		"adding a context cancellation doesn't lose the information": {
			sourceErrors: []error{context.Canceled},
			target:       context.Canceled,
			is:           true,
		},
		"adding multiple context cancellations doesn't lose the information": {
			sourceErrors: []error{context.Canceled, context.Canceled},
			target:       context.Canceled,
			is:           true,
		},
		"adding wrapped context cancellations doesn't lose the information": {
			sourceErrors: []error{errors.New("some error"), fmt.Errorf("some message: %w", context.Canceled)},
			target:       context.Canceled,
			is:           true,
		},
		"adding a nil error doesn't lose the information": {
			sourceErrors: []error{errors.New("some error"), fmt.Errorf("some message: %w", context.Canceled), nil},
			target:       context.Canceled,
			is:           true,
		},
		"errors with no context cancellation error are not a context canceled error": {
			sourceErrors: []error{errors.New("first error"), errors.New("second error")},
			target:       context.Canceled,
			is:           false,
		},
		"no errors are not a context canceled error": {
			sourceErrors: nil,
			target:       context.Canceled,
			is:           false,
		},
		"no errors are a nil error": {
			sourceErrors: nil,
			target:       nil,
			is:           true,
		},
		"nested multi-error contains anError": {
			sourceErrors: []error{
				anError,
				NewMulti(
					customErr,
					fmt.Errorf("wrapped %w", context.Canceled),
				).Err(),
			},
			target: anError,
			is:     true,
		},
		"nested multi-error contains custom error": {
			sourceErrors: []error{
				anError,
				NewMulti(
					customErr,
					fmt.Errorf("wrapped %w", context.Canceled),
				).Err(),
			},
			target: customErr,
			is:     true,
		},
		"nested multi-error contains wrapped context.Canceled": {
			sourceErrors: []error{
				anError,
				NewMulti(
					customErr,
					fmt.Errorf("wrapped %w", context.Canceled),
				).Err(),
			},
			target: context.Canceled,
			is:     true,
		},
		"nested multi-error does not contain context.DeadlineExceeded": {
			sourceErrors: []error{
				anError,
				NewMulti(
					customErr,
					fmt.Errorf("wrapped %w", context.Canceled),
				).Err(),
			},
			target: context.DeadlineExceeded,
			is:     false, // make sure we still return false in valid cases
		},
	}

	for testName, testCase := range testCases {
		t.Run(testName, func(t *testing.T) {
			mErr := NewMulti(testCase.sourceErrors...)
			require.Equal(t, testCase.is, errors.Is(mErr.Err(), testCase.target))
		})
	}
}

func TestMultiError_As(t *testing.T) {
	tE1 := testError{"error cause 1"}
	tE2 := testError{"error cause 2"}
	var target testError
	testCases := map[string]struct {
		sourceErrors []error
		target       error
		as           bool
	}{
		"MultiError containing only a testError can be cast to that testError": {
			sourceErrors: []error{tE1},
			target:       tE1,
			as:           true,
		},
		"MultiError containing multiple testErrors can be cast to the first testError added": {
			sourceErrors: []error{tE1, tE2},
			target:       tE1,
			as:           true,
		},
		"MultiError containing multiple errors can be cast to the first testError added": {
			sourceErrors: []error{context.Canceled, tE1, context.DeadlineExceeded, tE2},
			target:       tE1,
			as:           true,
		},
		"MultiError non containing a testError cannot be cast to a testError": {
			sourceErrors: []error{context.Canceled, context.DeadlineExceeded},
			as:           false,
		},
	}

	for testName, testCase := range testCases {
		t.Run(testName, func(t *testing.T) {
			mErr := NewMulti(testCase.sourceErrors...).Err()
			if testCase.as {
				require.True(t, errors.As(mErr, &target))
				require.Equal(t, testCase.target, target)
			} else {
				require.False(t, errors.As(mErr, &target))
			}
		})
	}
}

type testError struct {
	cause string
}

func (e testError) Error() string {
	return fmt.Sprintf("testError[cause: %s]", e.cause)
}
