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

package logging

import (
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/grafana/regexp"
	"github.com/stretchr/testify/require"
)

func TestJSONFileLogger_basic(t *testing.T) {
	f, err := ioutil.TempFile("", "logging")
	require.NoError(t, err)
	defer func() {
		require.NoError(t, f.Close())
		require.NoError(t, os.Remove(f.Name()))
	}()

	l, err := NewJSONFileLogger(f.Name())
	require.NoError(t, err)
	require.NotNil(t, l, "logger can't be nil")

	err = l.Log("test", "yes")
	require.NoError(t, err)
	r := make([]byte, 1024)
	_, err = f.Read(r)
	require.NoError(t, err)
	result, err := regexp.Match(`^{"test":"yes","ts":"[^"]+"}\n`, r)
	require.NoError(t, err)
	require.True(t, result, "unexpected content: %s", r)

	err = l.Close()
	require.NoError(t, err)

	err = l.file.Close()
	require.Error(t, err)
	require.True(t, strings.HasSuffix(err.Error(), os.ErrClosed.Error()), "file not closed")
}

func TestJSONFileLogger_parallel(t *testing.T) {
	f, err := ioutil.TempFile("", "logging")
	require.NoError(t, err)
	defer func() {
		require.NoError(t, f.Close())
		require.NoError(t, os.Remove(f.Name()))
	}()

	l, err := NewJSONFileLogger(f.Name())
	require.NoError(t, err)
	require.NotNil(t, l, "logger can't be nil")

	err = l.Log("test", "yes")
	require.NoError(t, err)

	l2, err := NewJSONFileLogger(f.Name())
	require.NoError(t, err)
	require.NotNil(t, l, "logger can't be nil")

	err = l2.Log("test", "yes")
	require.NoError(t, err)

	err = l.Close()
	require.NoError(t, err)

	err = l.file.Close()
	require.Error(t, err)
	require.True(t, strings.HasSuffix(err.Error(), os.ErrClosed.Error()), "file not closed")

	err = l2.Close()
	require.NoError(t, err)

	err = l2.file.Close()
	require.Error(t, err)
	require.True(t, strings.HasSuffix(err.Error(), os.ErrClosed.Error()), "file not closed")
}
