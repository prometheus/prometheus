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

type errorWithStatusCode struct {
	statusCode int
	err        error
}

func (e *errorWithStatusCode) GetStatusCode() int {
	return e.statusCode
}

func (e *errorWithStatusCode) Error() string {
	return e.err.Error()
}

func ErrorWithHTTPStatusCode(code int, err error) (error, bool) {
	if !isValidHTTPStatusCode(code) {
		return err, false
	}
	return &errorWithStatusCode{
		statusCode: code,
		err:        err,
	}, true
}

func HTTPStatusCode(err error) (int, bool) {
	if errWithCode, ok := err.(*errorWithStatusCode); ok {
		if !isValidHTTPStatusCode(errWithCode.statusCode) {
			return 0, false
		}
		return errWithCode.statusCode, true
	}

	return 0, false
}

func isValidHTTPStatusCode(code int) bool {
	return http.StatusText(code) != ""
}
