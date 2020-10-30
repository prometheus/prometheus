// Copyright 2016 The etcd Authors
//
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

package errors

import (
	"bytes"
	stderrors "errors"
	"fmt"
	"io"
)

// nilOrMultiError type allows combining multiple errors into one.
type nilOrMultiError []error

// NewMulti returns nilOrMultiError with provided errors added if not nil.
func NewMulti(errs ...error) nilOrMultiError { // nolint:golint
	m := nilOrMultiError{}
	m.Add(errs...)
	return m
}

// Add adds single or many errors to the error list. Each error is added only if not nil.
// If the error is a multiError type, the errors inside multiError are added to the main nilOrMultiError.
func (es *nilOrMultiError) Add(errs ...error) {
	for _, err := range errs {
		if err == nil {
			continue
		}
		if merr, ok := err.(multiError); ok {
			*es = append(*es, merr.errs...)
			continue
		}
		*es = append(*es, err)
	}
}

// Err returns the error list as an error or nil if it is empty.
func (es nilOrMultiError) Err() MultiError {
	if len(es) == 0 {
		return nil
	}
	return multiError{errs: es}
}

// MultiError is extended error interface that allows to use returned read-only multi error in more advanced ways.
type MultiError interface {
	error

	// Errors returns underlying errors.
	Errors() []error

	// As finds the first error in multiError slice of error chains that matches target, and if so, sets
	// target to that error value and returns true. Otherwise, it returns false.
	//
	// An error matches target if the error's concrete value is assignable to the value
	// pointed to by target, or if the error has a method As(interface{}) bool such that
	// As(target) returns true. In the latter case, the As method is responsible for
	// setting target.
	As(target interface{}) bool
	// Is returns true if any error in multiError's slice of error chains matches the given target or
	// if the target is of multiError type.
	//
	// An error is considered to match a target if it is equal to that target or if
	// it implements a method Is(error) bool such that Is(target) returns true.
	Is(target error) bool
	// Count returns the number of multi error' errors that match the given target.
	// Matching is defined as in Is method.
	Count(target error) int
}

// multiError implements the error and MultiError interfaces, and it represents nilOrMultiError (in other words []error) with at least one error inside it.
// NOTE: This type is useful to make sure that nilOrMultiError is not accidentally used for err != nil check.
type multiError struct {
	errs []error
}

// Errors returns underlying errors.
func (e multiError) Errors() []error {
	return e.errs
}

// Error returns a concatenated string of the contained errors.
func (e multiError) Error() string {
	var buf bytes.Buffer

	if len(e.errs) > 1 {
		fmt.Fprintf(&buf, "%d errors: ", len(e.errs))
	}

	for i, err := range e.errs {
		if i != 0 {
			buf.WriteString("; ")
		}
		buf.WriteString(err.Error())
	}

	return buf.String()
}

// As finds the first error in multiError slice of error chains that matches target, and if so, sets
// target to that error value and returns true. Otherwise, it returns false.
//
// An error matches target if the error's concrete value is assignable to the value
// pointed to by target, or if the error has a method As(interface{}) bool such that
// As(target) returns true. In the latter case, the As method is responsible for
// setting target.
func (e multiError) As(target interface{}) bool {
	if t, ok := target.(*multiError); ok {
		*t = e
		return true
	}

	for _, err := range e.errs {
		if stderrors.As(err, target) {
			return true
		}
	}
	return false
}

// Is returns true if any error in multiError's slice of error chains matches the given target or
// if the target is of multiError type.
//
// An error is considered to match a target if it is equal to that target or if
// it implements a method Is(error) bool such that Is(target) returns true.
func (e multiError) Is(target error) bool {
	if m, ok := target.(multiError); ok {
		if len(m.errs) != len(e.errs) {
			return false
		}
		for i := 0; i < len(e.errs); i++ {
			if !stderrors.Is(m.errs[i], e.errs[i]) {
				return false
			}
		}
		return true
	}
	for _, err := range e.errs {
		if stderrors.Is(err, target) {
			return true
		}
	}
	return false
}

// Count returns the number of all multi error' errors that match the given target (including nested multi errors).
// Matching is defined as in Is method.
func (e multiError) Count(target error) (count int) {
	for _, err := range e.errs {
		if inner, ok := AsMulti(err); ok {
			count += inner.Count(target)
			continue
		}

		if stderrors.Is(err, target) {
			count++
		}
	}
	return count
}

// AsMulti casts error to multi error read only interface. It returns multi error and true if error matches multi error as
// defined by As method. If returns false if no multi error can be found.
func AsMulti(err error) (MultiError, bool) {
	m := multiError{}
	if !stderrors.As(err, &m) {
		return nil, false
	}
	return m, true
}

// CloseAll closes all given closers while recording error in multiError.
func CloseAll(cs []io.Closer) error {
	errs := NewMulti()
	for _, c := range cs {
		errs.Add(c.Close())
	}
	return errs.Err()
}
