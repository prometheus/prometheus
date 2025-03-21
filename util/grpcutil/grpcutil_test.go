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
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestStatusCodeFromErrorChain(t *testing.T) {
	var err1, err2 error
	var code int32
	var ok bool

	err1 = status.Error(http.StatusTooManyRequests, "resource exhausted")
	err2 = fmt.Errorf("some error: %w", err1)

	code, ok = HTTPStatusCodeFromErrorChain(err2)
	require.True(t, ok)
	require.Equal(t, int32(http.StatusTooManyRequests), code)

	err1 = status.Error(codes.ResourceExhausted, "resource exhausted")
	err2 = fmt.Errorf("some error: %w", err1)

	_, ok = HTTPStatusCodeFromErrorChain(err2)
	require.False(t, ok) // codes.ResourceExhausted is not a valid HTTP status

	err1 = errors.New("error without code")
	err2 = fmt.Errorf("some error: %w", err1)

	_, ok = HTTPStatusCodeFromErrorChain(err2)
	require.False(t, ok)
}
