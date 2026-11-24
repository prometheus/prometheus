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

package yamlutil

import (
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type testConfig struct {
	Known string   `yaml:"known"`
	Seq   []string `yaml:"seq"`
}

// The empty and comment-only cases are the reason Unmarshal and UnmarshalStrict
// exist: yaml/v4 reports io.EOF for them, while yaml/v2 reported success.
func TestUnmarshalEmptyInput(t *testing.T) {
	for _, in := range []string{"", "\n", "  \n\t\n", "# only a comment\n", "---\n", "---\n# comment\n"} {
		t.Run(fmt.Sprintf("%q", in), func(t *testing.T) {
			var cfg testConfig
			require.NoError(t, Unmarshal([]byte(in), &cfg))
			require.NoError(t, UnmarshalStrict([]byte(in), &cfg))
			require.Equal(t, testConfig{}, cfg)
		})
	}
}

func TestUnmarshalNullDocument(t *testing.T) {
	var cfg testConfig
	require.NoError(t, UnmarshalStrict([]byte("null\n"), &cfg))
	require.Equal(t, testConfig{}, cfg)
}

func TestUnmarshalStrict(t *testing.T) {
	t.Run("known fields", func(t *testing.T) {
		var cfg testConfig
		require.NoError(t, UnmarshalStrict([]byte("known: a\nseq: [b]\n"), &cfg))
		require.Equal(t, testConfig{Known: "a", Seq: []string{"b"}}, cfg)
	})

	t.Run("unknown field is rejected", func(t *testing.T) {
		var cfg testConfig
		err := UnmarshalStrict([]byte("unknown: a\n"), &cfg)
		require.ErrorContains(t, err, `field unknown not found in type yamlutil.testConfig`)
	})

	t.Run("unknown field is accepted when not strict", func(t *testing.T) {
		var cfg testConfig
		require.NoError(t, Unmarshal([]byte("unknown: a\n"), &cfg))
		require.Equal(t, testConfig{}, cfg)
	})

	// Both v2 and v4 refuse duplicate mapping keys, regardless of strictness.
	t.Run("duplicate key", func(t *testing.T) {
		var cfg testConfig
		require.ErrorContains(t, UnmarshalStrict([]byte("known: a\nknown: b\n"), &cfg), "already defined")
		require.ErrorContains(t, Unmarshal([]byte("known: a\nknown: b\n"), &cfg), "already defined")
	})

	t.Run("syntax error", func(t *testing.T) {
		var cfg testConfig
		require.ErrorContains(t, UnmarshalStrict([]byte("known: [a\n"), &cfg), "did not find expected")
	})
}

// Multi-document input is truncated rather than rejected, matching yaml/v2. The
// behavior is pinned here so that a future yaml bump cannot silently change what
// Prometheus does with such input.
func TestUnmarshalMultiDocument(t *testing.T) {
	var cfg testConfig
	require.NoError(t, UnmarshalStrict([]byte("known: a\n---\nknown: b\n"), &cfg))
	require.Equal(t, testConfig{Known: "a"}, cfg)
}

func TestMarshal(t *testing.T) {
	tests := []struct {
		name     string
		in       any
		expected string
	}{
		{
			name:     "scalar",
			in:       "value",
			expected: "value\n",
		},
		{
			name:     "nil",
			in:       nil,
			expected: "null\n",
		},
		{
			// yaml/v2 did not indent sequences under their parent key, yaml/v4
			// does. This is user visible through the HTTP API and promtool, so it
			// is pinned here.
			name:     "sequence indentation",
			in:       testConfig{Known: "a", Seq: []string{"b", "c"}},
			expected: "known: a\nseq:\n  - b\n  - c\n",
		},
		{
			name:     "empty map",
			in:       map[string]string{},
			expected: "{}\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			out, err := Marshal(test.in)
			require.NoError(t, err)
			require.Equal(t, test.expected, string(out))
		})
	}
}

// eofUnmarshaler returns io.EOF, like a custom unmarshaler that reads from a
// stream would. The decoder wraps it as a type error, which must not be mistaken
// for the io.EOF signalling an empty document.
type eofUnmarshaler struct{}

// UnmarshalYAML implements yaml.Unmarshaler.
func (*eofUnmarshaler) UnmarshalYAML(_ func(any) error) error {
	return io.EOF
}

func TestUnmarshalDoesNotSwallowEOFFromUnmarshaler(t *testing.T) {
	type wrapper struct {
		F eofUnmarshaler `yaml:"f"`
	}

	var w wrapper
	err := UnmarshalStrict([]byte("f:\n  a: 1\n"), &w)
	require.Error(t, err, "io.EOF from a custom unmarshaler must not be treated as empty input")
	require.ErrorIs(t, err, io.EOF, "the original cause should stay in the chain")
}

// TestMarshalWrapping pins the line wrapping of long plain scalars. yaml/v2 wrapped
// them at 80 columns and yaml/v4's Marshal does not (it uses an unlimited line
// width), so Marshal has to keep using an Encoder. Wrapping also changes the text
// that Prometheus serves at /api/v1/status/config, hence the exact expectation.
func TestMarshalWrapping(t *testing.T) {
	long := strings.TrimSpace(strings.Repeat("word ", 30) + "end")

	out, err := Marshal(map[string]string{"value": long})
	require.NoError(t, err)
	// Byte for byte what yaml/v2 emitted for the same input.
	require.Equal(t, "value: word word word word word word word word word word word word word word word\n"+
		"  word word word word word word word word word word word word word word word end\n", string(out))

	// The wrapped scalar must fold back to the original value.
	var back map[string]string
	require.NoError(t, Unmarshal(out, &back))
	require.Equal(t, long, back["value"])
}

func TestIsTypeError(t *testing.T) {
	var cfg testConfig

	// A construct error, e.g. an unknown field or a value of the wrong type.
	err := UnmarshalStrict([]byte("unknown: a\n"), &cfg)
	require.True(t, IsTypeError(err))

	err = UnmarshalStrict([]byte("seq: a\n"), &cfg)
	require.True(t, IsTypeError(err))

	// A syntax error is a *yaml.LoadError, not a *yaml.LoadErrors.
	err = UnmarshalStrict([]byte("known: [a\n"), &cfg)
	require.Error(t, err)
	require.False(t, IsTypeError(err))

	require.False(t, IsTypeError(nil))
	require.False(t, IsTypeError(errors.New("plain error")))

	// Wrapped errors are still recognized.
	err = fmt.Errorf("parsing YAML: %w", UnmarshalStrict([]byte("unknown: a\n"), &cfg))
	require.True(t, IsTypeError(err))
}

func TestReplaceTypeNames(t *testing.T) {
	oldTyp := reflect.TypeFor[testConfig]()
	newTyp := reflect.TypeFor[map[string]string]()

	t.Run("rewrites message", func(t *testing.T) {
		var cfg testConfig
		err := ReplaceTypeNames(
			UnmarshalStrict([]byte("unknown: a\n"), &cfg),
			oldTyp, newTyp,
		)
		require.ErrorContains(t, err, `field unknown not found in type map[string]string`)
		require.NotContains(t, err.Error(), "yamlutil.testConfig")
	})

	// A wrapper such as fmt.Errorf has already formatted the message, so the
	// rewriting has to happen on the reported message rather than in place.
	t.Run("rewrites message of a wrapped error", func(t *testing.T) {
		var cfg testConfig
		err := fmt.Errorf("parsing YAML file x.yml: %w", UnmarshalStrict([]byte("unknown: a\n"), &cfg))
		err = ReplaceTypeNames(err, oldTyp, newTyp)
		require.ErrorContains(t, err, `parsing YAML file x.yml: yaml: construct errors`)
		require.ErrorContains(t, err, `field unknown not found in type map[string]string`)
		require.NotContains(t, err.Error(), "yamlutil.testConfig")
	})

	t.Run("keeps the error chain intact", func(t *testing.T) {
		var cfg testConfig
		inner := UnmarshalStrict([]byte("unknown: a\n"), &cfg)
		err := ReplaceTypeNames(fmt.Errorf("wrapped: %w", inner), oldTyp, newTyp)
		require.True(t, IsTypeError(err))
		require.ErrorIs(t, err, inner)
	})

	t.Run("leaves other errors alone", func(t *testing.T) {
		err := errors.New("plain error")
		require.Equal(t, err, ReplaceTypeNames(err, oldTyp, newTyp))
		require.NoError(t, ReplaceTypeNames(nil, oldTyp, newTyp))

		syntaxErr := UnmarshalStrict([]byte("known: [a\n"), &testConfig{})
		require.Equal(t, syntaxErr, ReplaceTypeNames(syntaxErr, oldTyp, newTyp))

		// A type error that does not mention the old type name is left alone.
		var cfg testConfig
		other := UnmarshalStrict([]byte("unknown: a\n"), &cfg)
		require.Equal(t, other, ReplaceTypeNames(other, reflect.TypeFor[map[string]string](), newTyp))
	})
}
