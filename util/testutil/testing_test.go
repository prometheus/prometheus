// Copyright 2020 The Prometheus Authors
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

package testutil

import (
	"errors"
	"testing"
)

type testTB struct {
	isTestPass bool
}

func (t *testTB) Helper() {
	t.isTestPass = true
}

func (t *testTB) Fatalf(string, ...interface{}) {
	t.isTestPass = false
}

func TestAssert(t *testing.T) {
	cases := []struct {
		condition bool
		expected  bool
	}{
		{
			condition: true,
			expected:  true,
		},
		{
			condition: false,
			expected:  false,
		},
	}

	for _, c := range cases {
		var tb testTB
		Assert(&tb, c.condition, "test")
		if c.expected != tb.isTestPass {
			t.Errorf("expected: %v, got: %v", c.expected, tb.isTestPass)
		}
	}
}

func TestOk(t *testing.T) {
	cases := []struct {
		err      error
		expected bool
	}{
		{
			err:      nil,
			expected: true,
		},
		{
			err:      errors.New("got err"),
			expected: false,
		},
	}

	for _, c := range cases {
		var tb testTB
		Ok(&tb, c.err)
		if c.expected != tb.isTestPass {
			t.Errorf("expected: %v, got: %v", c.expected, tb.isTestPass)
		}
	}
}

func TestNotOk(t *testing.T) {
	cases := []struct {
		err      error
		expected bool
	}{
		{
			err:      nil,
			expected: false,
		},
		{
			err:      errors.New("got err"),
			expected: true,
		},
	}

	for _, c := range cases {
		var tb testTB
		NotOk(&tb, c.err)
		if c.expected != tb.isTestPass {
			t.Errorf("expected: %v, got: %v", c.expected, tb.isTestPass)
		}
	}
}

func TestEquals(t *testing.T) {
	cases := []struct {
		inputA   interface{}
		inputB   interface{}
		expected bool
	}{
		{
			inputA:   "equal",
			inputB:   "equal",
			expected: true,
		},
		{
			inputA:   "equal",
			inputB:   "not equal",
			expected: false,
		},
		{
			inputA:   "equal",
			inputB:   []byte("equal"),
			expected: false,
		},
		{
			inputA:   new(int),
			inputB:   new(int),
			expected: true,
		},
		{
			inputA:   new(int),
			inputB:   new(float32),
			expected: false,
		},
	}

	for _, c := range cases {
		var tb testTB
		Equals(&tb, c.inputA, c.inputB)
		if c.expected != tb.isTestPass {
			t.Errorf("expected: %v, got: %v", c.expected, tb.isTestPass)
		}
	}
}

func TestErrorEqual(t *testing.T) {
	cases := []struct {
		inputA   error
		inputB   error
		expected bool
	}{
		{
			inputA:   errors.New("equal"),
			inputB:   errors.New("equal"),
			expected: true,
		},
		{
			inputA:   errors.New("equal"),
			inputB:   errors.New("not equal"),
			expected: false,
		},
		{
			inputA:   errors.New("equal"),
			inputB:   nil,
			expected: false,
		},
		{
			inputA:   nil,
			inputB:   nil,
			expected: true,
		},
	}

	for _, c := range cases {
		var tb testTB
		ErrorEqual(&tb, c.inputA, c.inputB)
		if c.expected != tb.isTestPass {
			t.Errorf("expected: %v, got: %v", c.expected, tb.isTestPass)
		}
	}
}
