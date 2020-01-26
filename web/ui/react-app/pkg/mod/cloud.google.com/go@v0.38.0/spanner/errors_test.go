/*
Copyright 2017 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package spanner

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestToSpannerError(t *testing.T) {
	for _, test := range []struct {
		err      error
		wantCode codes.Code
	}{
		{errors.New("wha?"), codes.Unknown},
		{context.Canceled, codes.Canceled},
		{context.DeadlineExceeded, codes.DeadlineExceeded},
		{status.Errorf(codes.ResourceExhausted, "so tired"), codes.ResourceExhausted},
		{spannerErrorf(codes.InvalidArgument, "bad"), codes.InvalidArgument},
	} {
		err := toSpannerError(test.err)
		if got, want := err.(*Error).Code, test.wantCode; got != want {
			t.Errorf("%v: got %s, want %s", test.err, got, want)
		}
		converted := status.Convert(err)
		if converted.Code() != test.wantCode {
			t.Errorf("%v: got status %v, want status %v", test.err, converted.Code(), test.wantCode)
		}
	}
}
