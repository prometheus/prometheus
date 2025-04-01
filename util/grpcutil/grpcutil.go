// Copyright 2025 The Prometheus Authors
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

package grpcutil

import (
	"net/http"
)

type ErrorWithStatusCode struct {
	StatusCode int
	Err        error
}

func (e *ErrorWithStatusCode) GetStatusCode() int {
	return e.StatusCode
}

func (e *ErrorWithStatusCode) Error() string {
	return e.Err.Error()
}

func ErrorWithHTTPStatusCode(code int, err error) (error, bool) {
	if !isValidHTTPStatusCode(code) {
		return err, false
	}
	return &ErrorWithStatusCode{
		StatusCode: code,
		Err:        err,
	}, true
}

func HTTPStatusCode(err error) (int, bool) {
	if errWithCode, ok := err.(*ErrorWithStatusCode); ok {
		if !isValidHTTPStatusCode(errWithCode.StatusCode) {
			return 0, false
		}
		return errWithCode.StatusCode, true
	}

	return 0, false
}

func isValidHTTPStatusCode(code int) bool {
	return http.StatusText(code) != ""
}
