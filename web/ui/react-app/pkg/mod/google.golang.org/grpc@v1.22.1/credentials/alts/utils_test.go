/*
 *
 * Copyright 2018 gRPC authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

package alts

import (
	"context"
	"io"
	"strings"
	"testing"

	altspb "google.golang.org/grpc/credentials/alts/internal/proto/grpc_gcp"
	"google.golang.org/grpc/peer"
)

func TestIsRunningOnGCP(t *testing.T) {
	for _, tc := range []struct {
		description string
		testOS      string
		testReader  io.Reader
		out         bool
	}{
		// Linux tests.
		{"linux: not a GCP platform", "linux", strings.NewReader("not GCP"), false},
		{"Linux: GCP platform (Google)", "linux", strings.NewReader("Google"), true},
		{"Linux: GCP platform (Google Compute Engine)", "linux", strings.NewReader("Google Compute Engine"), true},
		{"Linux: GCP platform (Google Compute Engine) with extra spaces", "linux", strings.NewReader("  Google Compute Engine        "), true},
		// Windows tests.
		{"windows: not a GCP platform", "windows", strings.NewReader("not GCP"), false},
		{"windows: GCP platform (Google)", "windows", strings.NewReader("Google"), true},
		{"windows: GCP platform (Google) with extra spaces", "windows", strings.NewReader("  Google     "), true},
	} {
		reverseFunc := setup(tc.testOS, tc.testReader)
		if got, want := isRunningOnGCP(), tc.out; got != want {
			t.Errorf("%v: isRunningOnGCP()=%v, want %v", tc.description, got, want)
		}
		reverseFunc()
	}
}

func setup(testOS string, testReader io.Reader) func() {
	tmpOS := runningOS
	tmpReader := manufacturerReader

	// Set test OS and reader function.
	runningOS = testOS
	manufacturerReader = func() (io.Reader, error) {
		return testReader, nil
	}

	return func() {
		runningOS = tmpOS
		manufacturerReader = tmpReader
	}
}

func TestAuthInfoFromContext(t *testing.T) {
	ctx := context.Background()
	altsAuthInfo := &fakeALTSAuthInfo{}
	p := &peer.Peer{
		AuthInfo: altsAuthInfo,
	}
	for _, tc := range []struct {
		desc    string
		ctx     context.Context
		success bool
		out     AuthInfo
	}{
		{
			"working case",
			peer.NewContext(ctx, p),
			true,
			altsAuthInfo,
		},
	} {
		authInfo, err := AuthInfoFromContext(tc.ctx)
		if got, want := (err == nil), tc.success; got != want {
			t.Errorf("%v: AuthInfoFromContext(_)=(err=nil)=%v, want %v", tc.desc, got, want)
		}
		if got, want := authInfo, tc.out; got != want {
			t.Errorf("%v:, AuthInfoFromContext(_)=(%v, _), want (%v, _)", tc.desc, got, want)
		}
	}
}

func TestAuthInfoFromPeer(t *testing.T) {
	altsAuthInfo := &fakeALTSAuthInfo{}
	p := &peer.Peer{
		AuthInfo: altsAuthInfo,
	}
	for _, tc := range []struct {
		desc    string
		p       *peer.Peer
		success bool
		out     AuthInfo
	}{
		{
			"working case",
			p,
			true,
			altsAuthInfo,
		},
	} {
		authInfo, err := AuthInfoFromPeer(tc.p)
		if got, want := (err == nil), tc.success; got != want {
			t.Errorf("%v: AuthInfoFromPeer(_)=(err=nil)=%v, want %v", tc.desc, got, want)
		}
		if got, want := authInfo, tc.out; got != want {
			t.Errorf("%v:, AuthInfoFromPeer(_)=(%v, _), want (%v, _)", tc.desc, got, want)
		}
	}
}

type fakeALTSAuthInfo struct{}

func (*fakeALTSAuthInfo) AuthType() string            { return "" }
func (*fakeALTSAuthInfo) ApplicationProtocol() string { return "" }
func (*fakeALTSAuthInfo) RecordProtocol() string      { return "" }
func (*fakeALTSAuthInfo) SecurityLevel() altspb.SecurityLevel {
	return altspb.SecurityLevel_SECURITY_NONE
}
func (*fakeALTSAuthInfo) PeerServiceAccount() string                   { return "" }
func (*fakeALTSAuthInfo) LocalServiceAccount() string                  { return "" }
func (*fakeALTSAuthInfo) PeerRPCVersions() *altspb.RpcProtocolVersions { return nil }
