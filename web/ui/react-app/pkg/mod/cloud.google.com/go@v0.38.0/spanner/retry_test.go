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
	"fmt"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	edpb "google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// Test if runRetryable loop deals with various errors correctly.
func TestRetry(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	responses := []error{
		status.Errorf(codes.Internal, "transport is closing"),
		status.Errorf(codes.Unknown, "unexpected EOF"),
		status.Errorf(codes.Internal, "unexpected EOF"),
		status.Errorf(codes.Internal, "stream terminated by RST_STREAM with error code: 2"),
		status.Errorf(codes.Unavailable, "service is currently unavailable"),
		errRetry(fmt.Errorf("just retry it")),
	}
	err := runRetryable(context.Background(), func(ct context.Context) error {
		var r error
		if len(responses) > 0 {
			r = responses[0]
			responses = responses[1:]
		}
		return r
	})
	if err != nil {
		t.Errorf("runRetryable should be able to survive all retryable errors, but it returns %v", err)
	}
	// Unretryable errors
	injErr := errors.New("this is unretryable")
	err = runRetryable(context.Background(), func(ct context.Context) error {
		return injErr
	})
	if wantErr := toSpannerError(injErr); !testEqual(err, wantErr) {
		t.Errorf("runRetryable returns error %v, want %v", err, wantErr)
	}
	// Timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	retryErr := errRetry(fmt.Errorf("still retrying"))
	err = runRetryable(ctx, func(ct context.Context) error {
		// Expect to trigger timeout in retryable runner after 10 executions.
		<-time.After(100 * time.Millisecond)
		// Let retryable runner to retry so that timeout will eventually happen.
		return retryErr
	})
	// Check error code and error message
	if wantErrCode, wantErr := codes.DeadlineExceeded, errContextCanceled(ctx, retryErr); ErrCode(err) != wantErrCode || !testEqual(err, wantErr) {
		t.Errorf("<err code, err>=\n<%v, %v>, want:\n<%v, %v>", ErrCode(err), err, wantErrCode, wantErr)
	}
	// Cancellation
	ctx, cancel = context.WithCancel(context.Background())
	retries := 3
	retryErr = errRetry(fmt.Errorf("retry before cancel"))
	err = runRetryable(ctx, func(ct context.Context) error {
		retries--
		if retries == 0 {
			cancel()
		}
		return retryErr
	})
	// Check error code, error message, retry count
	if wantErrCode, wantErr := codes.Canceled, errContextCanceled(ctx, retryErr); ErrCode(err) != wantErrCode || !testEqual(err, wantErr) || retries != 0 {
		t.Errorf("<err code, err, retries>=\n<%v, %v, %v>, want:\n<%v, %v, %v>", ErrCode(err), err, retries, wantErrCode, wantErr, 0)
	}
}

func TestRetryInfo(t *testing.T) {
	b, _ := proto.Marshal(&edpb.RetryInfo{
		RetryDelay: ptypes.DurationProto(time.Second),
	})
	trailers := map[string]string{
		retryInfoKey: string(b),
	}
	gotDelay, ok := extractRetryDelay(errRetry(toSpannerErrorWithMetadata(status.Errorf(codes.Aborted, ""), metadata.New(trailers))))
	if !ok || !testEqual(time.Second, gotDelay) {
		t.Errorf("<ok, retryDelay> = <%t, %v>, want <true, %v>", ok, gotDelay, time.Second)
	}
}
