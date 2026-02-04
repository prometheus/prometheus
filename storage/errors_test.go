// Copyright The Prometheus Authors
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

package storage

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAppendPartialErrorToError(t *testing.T) {
	// nil receiver returns nil.
	var nilErr *AppendPartialError
	require.NoError(t, nilErr.ToError())

	// Empty ExemplarErrors returns nil.
	emptyErr := &AppendPartialError{}
	require.NoError(t, emptyErr.ToError())

	// Also test explicitly empty slice.
	emptySliceErr := &AppendPartialError{ExemplarErrors: []error{}}
	require.NoError(t, emptySliceErr.ToError())

	// Non-empty ExemplarErrors returns the error.
	nonEmptyErr := &AppendPartialError{ExemplarErrors: []error{ErrOutOfOrderExemplar}}
	require.ErrorIs(t, nonEmptyErr.ToError(), nonEmptyErr)
}

func TestErrDuplicateSampleForTimestamp(t *testing.T) {
	// All errDuplicateSampleForTimestamp are ErrDuplicateSampleForTimestamp
	require.ErrorIs(t, ErrDuplicateSampleForTimestamp, errDuplicateSampleForTimestamp{})

	// Same type only is if it has same properties.
	err := NewDuplicateFloatErr(1_000, 10, 20)
	sameErr := NewDuplicateFloatErr(1_000, 10, 20)
	differentErr := NewDuplicateFloatErr(1_001, 30, 40)

	require.ErrorIs(t, err, sameErr)
	require.NotErrorIs(t, err, differentErr)

	// Also works when err is wrapped.
	require.ErrorIs(t, fmt.Errorf("failed: %w", err), sameErr)
	require.NotErrorIs(t, fmt.Errorf("failed: %w", err), differentErr)
}
