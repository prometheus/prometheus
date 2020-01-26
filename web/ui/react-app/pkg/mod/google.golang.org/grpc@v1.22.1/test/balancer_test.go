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

package test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/grpclog"
	"google.golang.org/grpc/internal/balancerload"
	"google.golang.org/grpc/internal/testutils"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/resolver"
	testpb "google.golang.org/grpc/test/grpc_testing"
	"google.golang.org/grpc/testdata"
)

const testBalancerName = "testbalancer"

// testBalancer creates one subconn with the first address from resolved
// addresses.
//
// It's used to test options for NewSubConn are applies correctly.
type testBalancer struct {
	cc balancer.ClientConn
	sc balancer.SubConn

	newSubConnOptions balancer.NewSubConnOptions
	pickOptions       []balancer.PickOptions
	doneInfo          []balancer.DoneInfo
}

func (b *testBalancer) Build(cc balancer.ClientConn, opt balancer.BuildOptions) balancer.Balancer {
	b.cc = cc
	return b
}

func (*testBalancer) Name() string {
	return testBalancerName
}

func (b *testBalancer) HandleResolvedAddrs(addrs []resolver.Address, err error) {
	// Only create a subconn at the first time.
	if err == nil && b.sc == nil {
		b.sc, err = b.cc.NewSubConn(addrs, b.newSubConnOptions)
		if err != nil {
			grpclog.Errorf("testBalancer: failed to NewSubConn: %v", err)
			return
		}
		b.cc.UpdateBalancerState(connectivity.Connecting, &picker{sc: b.sc, bal: b})
		b.sc.Connect()
	}
}

func (b *testBalancer) HandleSubConnStateChange(sc balancer.SubConn, s connectivity.State) {
	grpclog.Infof("testBalancer: HandleSubConnStateChange: %p, %v", sc, s)
	if b.sc != sc {
		grpclog.Infof("testBalancer: ignored state change because sc is not recognized")
		return
	}
	if s == connectivity.Shutdown {
		b.sc = nil
		return
	}

	switch s {
	case connectivity.Ready, connectivity.Idle:
		b.cc.UpdateBalancerState(s, &picker{sc: sc, bal: b})
	case connectivity.Connecting:
		b.cc.UpdateBalancerState(s, &picker{err: balancer.ErrNoSubConnAvailable, bal: b})
	case connectivity.TransientFailure:
		b.cc.UpdateBalancerState(s, &picker{err: balancer.ErrTransientFailure, bal: b})
	}
}

func (b *testBalancer) Close() {
}

type picker struct {
	err error
	sc  balancer.SubConn
	bal *testBalancer
}

func (p *picker) Pick(ctx context.Context, opts balancer.PickOptions) (balancer.SubConn, func(balancer.DoneInfo), error) {
	if p.err != nil {
		return nil, nil, p.err
	}
	p.bal.pickOptions = append(p.bal.pickOptions, opts)
	return p.sc, func(d balancer.DoneInfo) { p.bal.doneInfo = append(p.bal.doneInfo, d) }, nil
}

func (s) TestCredsBundleFromBalancer(t *testing.T) {
	balancer.Register(&testBalancer{
		newSubConnOptions: balancer.NewSubConnOptions{
			CredsBundle: &testCredsBundle{},
		},
	})
	te := newTest(t, env{name: "creds-bundle", network: "tcp", balancer: ""})
	te.tapHandle = authHandle
	te.customDialOptions = []grpc.DialOption{
		grpc.WithBalancerName(testBalancerName),
	}
	creds, err := credentials.NewServerTLSFromFile(testdata.Path("server1.pem"), testdata.Path("server1.key"))
	if err != nil {
		t.Fatalf("Failed to generate credentials %v", err)
	}
	te.customServerOptions = []grpc.ServerOption{
		grpc.Creds(creds),
	}
	te.startServer(&testServer{})
	defer te.tearDown()

	cc := te.clientConn()
	tc := testpb.NewTestServiceClient(cc)
	if _, err := tc.EmptyCall(context.Background(), &testpb.Empty{}); err != nil {
		t.Fatalf("Test failed. Reason: %v", err)
	}
}

func (s) TestDoneInfo(t *testing.T) {
	for _, e := range listTestEnv() {
		testDoneInfo(t, e)
	}
}

func testDoneInfo(t *testing.T, e env) {
	te := newTest(t, e)
	b := &testBalancer{}
	balancer.Register(b)
	te.customDialOptions = []grpc.DialOption{
		grpc.WithBalancerName(testBalancerName),
	}
	te.userAgent = failAppUA
	te.startServer(&testServer{security: e.security})
	defer te.tearDown()

	cc := te.clientConn()
	tc := testpb.NewTestServiceClient(cc)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	wantErr := detailedError
	if _, err := tc.EmptyCall(ctx, &testpb.Empty{}); !testutils.StatusErrEqual(err, wantErr) {
		t.Fatalf("TestService/EmptyCall(_, _) = _, %v, want _, %v", err, wantErr)
	}
	if _, err := tc.UnaryCall(ctx, &testpb.SimpleRequest{}); err != nil {
		t.Fatalf("TestService.UnaryCall(%v, _, _, _) = _, %v; want _, <nil>", ctx, err)
	}

	if len(b.doneInfo) < 1 || !testutils.StatusErrEqual(b.doneInfo[0].Err, wantErr) {
		t.Fatalf("b.doneInfo = %v; want b.doneInfo[0].Err = %v", b.doneInfo, wantErr)
	}
	if len(b.doneInfo) < 2 || !reflect.DeepEqual(b.doneInfo[1].Trailer, testTrailerMetadata) {
		t.Fatalf("b.doneInfo = %v; want b.doneInfo[1].Trailer = %v", b.doneInfo, testTrailerMetadata)
	}
	if len(b.pickOptions) != len(b.doneInfo) {
		t.Fatalf("Got %d picks, but %d doneInfo, want equal amount", len(b.pickOptions), len(b.doneInfo))
	}
	// To test done() is always called, even if it's returned with a non-Ready
	// SubConn.
	//
	// Stop server and at the same time send RPCs. There are chances that picker
	// is not updated in time, causing a non-Ready SubConn to be returned.
	finished := make(chan struct{})
	go func() {
		for i := 0; i < 20; i++ {
			tc.UnaryCall(ctx, &testpb.SimpleRequest{})
		}
		close(finished)
	}()
	te.srv.Stop()
	<-finished
	if len(b.pickOptions) != len(b.doneInfo) {
		t.Fatalf("Got %d picks, %d doneInfo, want equal amount", len(b.pickOptions), len(b.doneInfo))
	}
}

const loadMDKey = "X-Endpoint-Load-Metrics-Bin"

type testLoadParser struct{}

func (*testLoadParser) Parse(md metadata.MD) interface{} {
	vs := md.Get(loadMDKey)
	if len(vs) == 0 {
		return nil
	}
	return vs[0]
}

func init() {
	balancerload.SetParser(&testLoadParser{})
}

func (s) TestDoneLoads(t *testing.T) {
	for _, e := range listTestEnv() {
		testDoneLoads(t, e)
	}
}

func testDoneLoads(t *testing.T, e env) {
	b := &testBalancer{}
	balancer.Register(b)

	const testLoad = "test-load-,-should-be-orca"

	ss := &stubServer{
		emptyCall: func(ctx context.Context, in *testpb.Empty) (*testpb.Empty, error) {
			grpc.SetTrailer(ctx, metadata.Pairs(loadMDKey, testLoad))
			return &testpb.Empty{}, nil
		},
	}
	if err := ss.Start(nil, grpc.WithBalancerName(testBalancerName)); err != nil {
		t.Fatalf("error starting testing server: %v", err)
	}
	defer ss.Stop()

	tc := testpb.NewTestServiceClient(ss.cc)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := tc.EmptyCall(ctx, &testpb.Empty{}); err != nil {
		t.Fatalf("TestService/EmptyCall(_, _) = _, %v, want _, %v", err, nil)
	}

	poWant := []balancer.PickOptions{
		{FullMethodName: "/grpc.testing.TestService/EmptyCall"},
	}
	if !reflect.DeepEqual(b.pickOptions, poWant) {
		t.Fatalf("b.pickOptions = %v; want %v", b.pickOptions, poWant)
	}

	if len(b.doneInfo) < 1 {
		t.Fatalf("b.doneInfo = %v, want length 1", b.doneInfo)
	}
	gotLoad, _ := b.doneInfo[0].ServerLoad.(string)
	if gotLoad != testLoad {
		t.Fatalf("b.doneInfo[0].ServerLoad = %v; want = %v", b.doneInfo[0].ServerLoad, testLoad)
	}
}
