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
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAppendPartialError_ToError(t *testing.T) {
	for _, tc := range []struct {
		name    string
		partial *AppendPartialError
		failed  bool
	}{
		{name: "nil"},
		{name: "empty", partial: &AppendPartialError{}},
		{name: "all nil", partial: &AppendPartialError{ExemplarErrors: make([]error, 3)}},
		{name: "sparse first", partial: &AppendPartialError{ExemplarErrors: []error{ErrOutOfOrderExemplar, nil}}, failed: true},
		{name: "sparse last", partial: &AppendPartialError{ExemplarErrors: []error{nil, ErrOutOfOrderExemplar}}, failed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !tc.failed {
				require.NoError(t, tc.partial.ToError())
			} else {
				require.Same(t, tc.partial, tc.partial.ToError())
			}
		})
	}
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

func TestAppendPartialError_FailedExemplarCount(t *testing.T) {
	for _, tc := range []struct {
		name   string
		errors []error
		want   int
	}{
		{name: "empty"},
		{name: "all nil", errors: make([]error, 3)},
		{name: "sparse", errors: []error{nil, ErrOutOfOrderExemplar, nil}, want: 1},
		{name: "complete", errors: []error{ErrOutOfOrderExemplar, ErrOutOfBounds}, want: 2},
		{name: "joined causes", errors: []error{nil, errors.Join(ErrOutOfOrderExemplar, ErrOutOfBounds)}, want: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			partial := &AppendPartialError{ExemplarErrors: tc.errors}
			require.Equal(t, tc.want, partial.FailedExemplarCount())
		})
	}
	t.Run("nil receiver", func(t *testing.T) {
		var partial *AppendPartialError
		require.Zero(t, partial.FailedExemplarCount())
	})
}

func TestAppendPartialError_Handle(t *testing.T) {
	first, second := ErrOutOfOrderExemplar, ErrOutOfBounds
	for _, tc := range []struct {
		name   string
		inputs [][]error
		want   [][]error
	}{
		{name: "disjoint", inputs: [][]error{{first, nil, nil}, {nil, second, nil}}, want: [][]error{{first}, {second}, nil}},
		{name: "overlap", inputs: [][]error{{nil, first, nil}, {nil, second, nil}, {nil, first, nil}}, want: [][]error{nil, {first, second}, nil}},
		{name: "all nil first", inputs: [][]error{make([]error, 1), {nil, second}}, want: [][]error{nil, {second}}},
		{name: "all nil last", inputs: [][]error{{first, nil}, make([]error, 1)}, want: [][]error{{first}, nil}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var aggregate *AppendPartialError
			for _, input := range tc.inputs {
				incoming := &AppendPartialError{ExemplarErrors: slices.Clone(input)}
				var err error
				aggregate, err = aggregate.Handle(fmt.Errorf("wrapped: %w", incoming))
				require.NoError(t, err)
				require.Equal(t, input, incoming.ExemplarErrors)
			}
			require.Len(t, aggregate.ExemplarErrors, len(tc.want))
			failed := 0
			for i, causes := range tc.want {
				if len(causes) == 0 {
					require.NoError(t, aggregate.ExemplarErrors[i])
					continue
				}
				failed++
				for _, cause := range causes {
					require.ErrorIs(t, aggregate.ExemplarErrors[i], cause)
				}
			}
			require.Equal(t, failed, aggregate.FailedExemplarCount())
		})
	}
	for _, tc := range []struct {
		name string
		err  error
	}{
		{name: "nil"},
		{name: "typed nil", err: (*AppendPartialError)(nil)},
		{name: "wrapped typed nil", err: fmt.Errorf("wrapped: %w", (*AppendPartialError)(nil))},
		{name: "empty", err: &AppendPartialError{}},
		{name: "all nil", err: &AppendPartialError{ExemplarErrors: make([]error, 3)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var aggregate *AppendPartialError
			aggregate, err := aggregate.Handle(tc.err)
			require.NoError(t, err)
			require.NoError(t, aggregate.ToError())
		})
	}
	t.Run("all nil receiver adopts failure shape", func(t *testing.T) {
		aggregate := &AppendPartialError{ExemplarErrors: make([]error, 1)}
		incoming := &AppendPartialError{ExemplarErrors: []error{nil, first}}
		got, err := aggregate.Handle(incoming)
		require.NoError(t, err)
		require.Equal(t, incoming.ExemplarErrors, got.ExemplarErrors)
		got.ExemplarErrors[1] = second
		require.Equal(t, []error{nil, first}, incoming.ExemplarErrors)
	})
	t.Run("ordinary error", func(t *testing.T) {
		aggregate := &AppendPartialError{ExemplarErrors: []error{first}}
		got, err := aggregate.Handle(second)
		require.Same(t, aggregate, got)
		require.Same(t, second, err)
	})
	t.Run("incompatible failures leave aggregate unchanged", func(t *testing.T) {
		aggregate := &AppendPartialError{ExemplarErrors: []error{first, nil}}
		incoming := &AppendPartialError{ExemplarErrors: []error{second}}
		got, err := aggregate.Handle(fmt.Errorf("wrapped: %w", incoming))
		require.Same(t, aggregate, got)
		require.Error(t, err)
		var partial *AppendPartialError
		require.NotErrorAs(t, err, &partial)
		require.Equal(t, []error{first, nil}, aggregate.ExemplarErrors)
		require.Equal(t, []error{second}, incoming.ExemplarErrors)
	})
	t.Run("independent slices", func(t *testing.T) {
		incoming := &AppendPartialError{ExemplarErrors: []error{nil, first}}
		var aggregate *AppendPartialError
		aggregate, err := aggregate.Handle(incoming)
		require.NoError(t, err)
		aggregate.ExemplarErrors[1] = second
		require.Equal(t, []error{nil, first}, incoming.ExemplarErrors)
	})
}
