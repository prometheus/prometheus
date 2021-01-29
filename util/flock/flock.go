// Copyright 2016 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package flock provides portable file locking. It is essentially ripped out
// from the code of github.com/syndtr/goleveldb. Strange enough that the
// standard library does not provide this functionality. Once this package has
// proven to work as expected, we should probably turn it into a separate
// general purpose package for humanity.
package flock

import (
	"os"
	"path/filepath"
)

// Releaser provides the Release method to release a file lock.
type Releaser interface {
	Release() error
}

// New locks the file with the provided name. If the file does not exist, it is
// created. The returned Releaser is used to release the lock. existed is true
// if the file to lock already existed. A non-nil error is returned if the
// locking has failed. Neither this function nor the returned Releaser is
// goroutine-safe.
func New(fileName string) (r Releaser, existed bool, err error) {
	if err = os.MkdirAll(filepath.Dir(fileName), 0755); err != nil {
		return
	}

	_, err = os.Stat(fileName)
	existed = err == nil

	r, err = newLock(fileName)
	return
}
