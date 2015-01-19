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
