// Copyright 2013 Prometheus Team
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

package test

import (
	"io/ioutil"
	"os"
	"testing"
)

const (
	// The base directory used for test emissions, which instructs the operating
	// system to use the default temporary directory as the base or TMPDIR
	// environment variable.
	defaultDirectory = ""

	// A NO-OP Closer.
	NilCloser = nilCloser(true)
)

type (
	Closer interface {
		// Close reaps the underlying directory and its children.  The directory
		// could be deleted by its users already.
		Close()
	}

	nilCloser bool

	// TemporaryDirectory models a closeable path for transient POSIX disk
	// activities.
	TemporaryDirectory interface {
		Closer

		// Path returns the underlying path for access.
		Path() string
	}

	// temporaryDirectory is kept as a private type due to private fields and
	// their interactions.
	temporaryDirectory struct {
		path   string
		tester testing.TB
	}
)

func (c nilCloser) Close() {
}

func (t temporaryDirectory) Close() {
	err := os.RemoveAll(t.path)
	if err != nil {
		switch {
		case os.IsNotExist(err):
			return
		default:
			t.tester.Fatal(err)
		}
	}
}

func (t temporaryDirectory) Path() string {
	return t.path
}

// NewTemporaryDirectory creates a new temporary directory for transient POSIX
// activities.
func NewTemporaryDirectory(name string, t testing.TB) (handler TemporaryDirectory) {
	var (
		directory string
		err       error
	)

	directory, err = ioutil.TempDir(defaultDirectory, name)
	if err != nil {
		t.Fatal(err)
	}

	handler = temporaryDirectory{
		path:   directory,
		tester: t,
	}

	return
}
