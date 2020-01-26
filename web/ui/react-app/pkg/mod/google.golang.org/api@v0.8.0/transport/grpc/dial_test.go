// Copyright 2016 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package grpc

import (
	"context"
	"errors"
	"net"
	"os"
	"testing"
	"time"

	"golang.org/x/oauth2"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
)

// Check that user optioned grpc.WithDialer option overwrites App Engine dialer
func TestGRPCHook(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	expected := false

	appengineDialerHook = (func(ctx context.Context) grpc.DialOption {
		return grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
			t.Error("did not expect a call to appengine dialer, got one")
			cancel()
			return nil, errors.New("not expected")
		})
	})
	defer func() {
		appengineDialerHook = nil
	}()

	expectedDialer := grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
		expected = true
		cancel()
		return nil, errors.New("expected")
	})

	conn, err := Dial(ctx,
		option.WithTokenSource(oauth2.StaticTokenSource(nil)), // No creds.
		option.WithGRPCDialOption(expectedDialer),
		option.WithEndpoint("example.google.com:443"))
	if err != nil {
		t.Errorf("DialGRPC: error %v, want nil", err)
	}
	defer conn.Close()

	// gRPC doesn't connect before the first call.
	grpc.Invoke(ctx, "foo", nil, nil, conn)

	if !expected {
		t.Error("expected a call to expected dialer, didn't get one")
	}
}

func TestIsDirectPathEnabled(t *testing.T) {
	for _, testcase := range []struct {
		name     string
		endpoint string
		envVar   string
		want     bool
	}{
		{
			name:     "matches",
			endpoint: "some-api",
			envVar:   "some-api",
			want:     true,
		},
		{
			name:     "does not match",
			endpoint: "some-api",
			envVar:   "some-other-api",
			want:     false,
		},
		{
			name:     "matches in list",
			endpoint: "some-api-2",
			envVar:   "some-api-1,some-api-2,some-api-3",
			want:     true,
		},
		{
			name:     "empty env var",
			endpoint: "",
			envVar:   "",
			want:     false,
		},
		{
			name:     "trailing comma",
			endpoint: "",
			envVar:   "foo,bar,",
			want:     false,
		},
		{
			name:     "dns schemes are allowed",
			endpoint: "dns:///foo",
			envVar:   "dns:///foo",
			want:     true,
		},
		{
			name:     "non-dns schemes are disallowed",
			endpoint: "https://foo",
			envVar:   "https://foo",
			want:     false,
		},
	} {
		t.Run(testcase.name, func(t *testing.T) {
			if err := os.Setenv("GOOGLE_CLOUD_ENABLE_DIRECT_PATH", testcase.envVar); err != nil {
				t.Fatal(err)
			}

			if got := isDirectPathEnabled(testcase.endpoint); got != testcase.want {
				t.Fatalf("got %v, want %v", got, testcase.want)
			}
		})
	}
}
