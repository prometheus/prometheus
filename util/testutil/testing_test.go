package testutil

import (
	"errors"
	"testing"
)

type testTB struct {
	isTestSuccess bool
}

func (t *testTB) Helper() {
	t.isTestSuccess = true
}

func (t *testTB) Fatalf(string, ...interface{}) {
	t.isTestSuccess = false
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
		if c.expected != tb.isTestSuccess {
			t.Errorf("expected: %v, got: %v", c.expected, tb.isTestSuccess)
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
		if c.expected != tb.isTestSuccess {
			t.Errorf("expected: %v, got: %v", c.expected, tb.isTestSuccess)
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
		if c.expected != tb.isTestSuccess {
			t.Errorf("expected: %v, got: %v", c.expected, tb.isTestSuccess)
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
		if c.expected != tb.isTestSuccess {
			t.Errorf("expected: %v, got: %v", c.expected, tb.isTestSuccess)
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
		if c.expected != tb.isTestSuccess {
			t.Errorf("expected: %v, got: %v", c.expected, tb.isTestSuccess)
		}
	}
}
