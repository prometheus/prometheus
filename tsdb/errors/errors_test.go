// Copyright 2025 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package errors

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMultiError_Is(t *testing.T) {
	customErr1 := errors.New("test error 1")
	customErr2 := errors.New("test error 2")

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
		"nested multi-error contains customErr1": {
			sourceErrors: []error{
				customErr1,
				NewMulti(
					customErr2,
					fmt.Errorf("wrapped %w", context.Canceled),
				).Err(),
			},
			target: customErr1,
			is:     true,
		},
		"nested multi-error contains customErr2": {
			sourceErrors: []error{
				customErr1,
				NewMulti(
					customErr2,
					fmt.Errorf("wrapped %w", context.Canceled),
				).Err(),
			},
			target: customErr2,
			is:     true,
		},
		"nested multi-error contains wrapped context.Canceled": {
			sourceErrors: []error{
				customErr1,
				NewMulti(
					customErr2,
					fmt.Errorf("wrapped %w", context.Canceled),
				).Err(),
			},
			target: context.Canceled,
			is:     true,
		},
		"nested multi-error does not contain context.DeadlineExceeded": {
			sourceErrors: []error{
				customErr1,
				NewMulti(
					customErr2,
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
		"MultiError not containing a testError cannot be cast to a testError": {
			sourceErrors: []error{context.Canceled, context.DeadlineExceeded},
			as:           false,
		},
	}

	for testName, testCase := range testCases {
		t.Run(testName, func(t *testing.T) {
			mErr := NewMulti(testCase.sourceErrors...).Err()
			if testCase.as {
				require.ErrorAs(t, mErr, &target)
				require.Equal(t, testCase.target, target)
			} else {
				require.NotErrorAs(t, mErr, &target)
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
