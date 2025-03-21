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
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	spb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestStatusCodeFromErrorChain(t *testing.T) {
	err1 := status.Error(codes.ResourceExhausted, "resource exhausted")
	err2 := errors.Wrapf(err1, "some error")

	code, ok := HTTPStatusCodeFromErrorChain(err2)
	require.True(t, ok)
	require.Equal(t, int32(codes.ResourceExhausted), code)

	err1 = status.ErrorProto(&spb.Status{
		Code:    429,
		Message: "resource exhausted",
	})
	err2 = errors.Wrapf(err1, "some error")

	code, ok = HTTPStatusCodeFromErrorChain(err2)
	require.True(t, ok)
	require.Equal(t, int32(429), code)

	err1 = errors.New("error without code")
	err2 = errors.Wrapf(err1, "some error")

	code, ok = HTTPStatusCodeFromErrorChain(err2)
	require.False(t, ok)
}
