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
	"errors"
	"net/http"

	"google.golang.org/grpc/status"
)

// HTTPStatusCodeFromErrorChain retrieves the first HTTP status code from the chain of gRPC error
func HTTPStatusCodeFromErrorChain(err error) (int32, bool) {
	if err == nil || len(err.Error()) == 0 {
		return 0, false
	}

	code, ok := statusCodeFromError(err)
	if ok && http.StatusText(int(code)) != "" {
		return code, true
	}
	return HTTPStatusCodeFromErrorChain(errors.Unwrap(err))
}

// statusCodeFromError retrieves the status code from the gRPC error
func statusCodeFromError(err error) (int32, bool) {
	s, ok := status.FromError(err)
	if !ok {
		return 0, false
	}

	return s.Proto().GetCode(), true
}
