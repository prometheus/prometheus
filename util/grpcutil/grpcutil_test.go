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
	"testing"

	"github.com/stretchr/testify/require"
)

func TestErrorWithHTTPStatusCode(t *testing.T) {
	err := ErrorWithHTTPStatusCode(http.StatusTooManyRequests, errors.New("some error"))

	require.Equal(t, &errorWithStatusCode{
		statusCode: http.StatusTooManyRequests,
		err:        errors.New("some error"),
	}, err)

	err = ErrorWithHTTPStatusCode(999, errors.New("weird error"))

	require.Equal(t, errors.New("weird error"), err)
}

func TestHTTPStatusCode(t *testing.T) {
	code, ok := HTTPStatusCode(&errorWithStatusCode{
		statusCode: http.StatusTooManyRequests,
		err:        errors.New("some error"),
	})

	require.True(t, ok)
	require.Equal(t, http.StatusTooManyRequests, code)

	code, ok = HTTPStatusCode(&errorWithStatusCode{
		statusCode: 999,
		err:        errors.New("some error"),
	})

	require.False(t, ok)
	require.Zero(t, code)

	code, ok = HTTPStatusCode(errors.New("some error"))

	require.False(t, ok)
	require.Zero(t, code)
}
