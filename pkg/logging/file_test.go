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
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestJSONFileLogger_basic(t *testing.T) {
	f, err := ioutil.TempFile("", "logging")
	assert.NoError(t, err)
	defer func() {
		assert.NoError(t, f.Close())
		assert.NoError(t, os.Remove(f.Name()))
	}()

	l, err := NewJSONFileLogger(f.Name())
	assert.NoError(t, err)
	assert.NotNil(t, l, "logger can't be nil")

	err = l.Log("test", "yes")
	assert.NoError(t, err)
	r := make([]byte, 1024)
	_, err = f.Read(r)
	assert.NoError(t, err)
	result, err := regexp.Match(`^{"test":"yes","ts":"[^"]+"}\n`, r)
	assert.NoError(t, err)
	assert.True(t, result, "unexpected content: %s", r)

	err = l.Close()
	assert.NoError(t, err)

	err = l.file.Close()
	assert.Error(t, err)
	assert.True(t, strings.HasSuffix(err.Error(), os.ErrClosed.Error()), "file not closed")
}

func TestJSONFileLogger_parallel(t *testing.T) {
	f, err := ioutil.TempFile("", "logging")
	assert.NoError(t, err)
	defer func() {
		assert.NoError(t, f.Close())
		assert.NoError(t, os.Remove(f.Name()))
	}()

	l, err := NewJSONFileLogger(f.Name())
	assert.NoError(t, err)
	assert.NotNil(t, l, "logger can't be nil")

	err = l.Log("test", "yes")
	assert.NoError(t, err)

	l2, err := NewJSONFileLogger(f.Name())
	assert.NoError(t, err)
	assert.NotNil(t, l, "logger can't be nil")

	err = l2.Log("test", "yes")
	assert.NoError(t, err)

	err = l.Close()
	assert.NoError(t, err)

	err = l.file.Close()
	assert.Error(t, err)
	assert.True(t, strings.HasSuffix(err.Error(), os.ErrClosed.Error()), "file not closed")

	err = l2.Close()
	assert.NoError(t, err)

	err = l2.file.Close()
	assert.Error(t, err)
	assert.True(t, strings.HasSuffix(err.Error(), os.ErrClosed.Error()), "file not closed")
}
