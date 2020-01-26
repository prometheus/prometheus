// Copyright 2017 Google LLC
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

package trace

import (
	"context"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"strings"
	"testing"

	pb "cloud.google.com/go/trace/testdata/helloworld"
	"google.golang.org/grpc"
)

func TestGRPCInterceptors(t *testing.T) {
	t.Skip("hangs forever for go < 1.9")

	tc := newTestClient(&noopTransport{})

	// default sampling with global=1.
	parent := tc.SpanFromHeader("parent", "7f27601f17b7a2873739efd18ff83872/123;o=1")
	testGRPCInterceptor(t, tc, parent, func(t *testing.T, out, in *Span) {
		if in == nil {
			t.Fatalf("missing span in the incoming context")
		}
		if got, want := in.TraceID(), out.TraceID(); got != want {
			t.Errorf("incoming call is not tracing the outgoing trace; TraceID = %q; want %q", got, want)
		}
		if !in.Traced() {
			t.Errorf("incoming span is not traced; want traced")
		}
	})

	// default sampling with global=0.
	parent = tc.SpanFromHeader("parent", "7f27601f17b7a2873739efd18ff83872/123;o=0")
	testGRPCInterceptor(t, tc, parent, func(t *testing.T, out, in *Span) {
		if in == nil {
			t.Fatalf("missing span in the incoming context")
		}
		if got, want := in.TraceID(), out.TraceID(); got != want {
			t.Errorf("incoming call is not tracing the outgoing trace; TraceID = %q; want %q", got, want)
		}
		if in.Traced() {
			t.Errorf("incoming span is traced; want not traced")
		}
	})

	// sampling all with global=1.
	all, _ := NewLimitedSampler(1.0, 1<<32)
	tc.SetSamplingPolicy(all)
	parent = tc.SpanFromHeader("parent", "7f27601f17b7a2873739efd18ff83872/123;o=1")
	testGRPCInterceptor(t, tc, parent, func(t *testing.T, out, in *Span) {
		if in == nil {
			t.Fatalf("missing span in the incoming context")
		}
		if got, want := in.TraceID(), out.TraceID(); got != want {
			t.Errorf("incoming call is not tracing the outgoing trace; TraceID = %q; want %q", got, want)
		}
		if !in.Traced() {
			t.Errorf("incoming span is not traced; want traced")
		}
	})

	// sampling none with global=1.
	none, _ := NewLimitedSampler(0, 0)
	tc.SetSamplingPolicy(none)
	parent = tc.SpanFromHeader("parent", "7f27601f17b7a2873739efd18ff83872/123;o=1")
	testGRPCInterceptor(t, tc, parent, func(t *testing.T, out, in *Span) {
		if in == nil {
			t.Fatalf("missing span in the incoming context")
		}
		if got, want := in.TraceID(), out.TraceID(); got != want {
			t.Errorf("incoming call is not tracing the outgoing trace; TraceID = %q; want %q", got, want)
		}
		if in.Traced() {
			t.Errorf("incoming span is traced; want not traced")
		}
	})

	// sampling all with no parent span.
	tc.SetSamplingPolicy(all)
	testGRPCInterceptor(t, tc, nil, func(t *testing.T, out, in *Span) {
		if in == nil {
			t.Fatalf("missing span in the incoming context")
		}
		if in.TraceID() == "" {
			t.Errorf("incoming call TraceID is empty")
		}
		if !in.Traced() {
			t.Errorf("incoming span is not traced; want traced")
		}
	})

	// sampling none with no parent span.
	tc.SetSamplingPolicy(none)
	testGRPCInterceptor(t, tc, nil, func(t *testing.T, out, in *Span) {
		if in == nil {
			t.Fatalf("missing span in the incoming context")
		}
		if in.TraceID() == "" {
			t.Errorf("incoming call TraceID is empty")
		}
		if in.Traced() {
			t.Errorf("incoming span is traced; want not traced")
		}
	})
}

func testGRPCInterceptor(t *testing.T, tc *Client, parent *Span, assert func(t *testing.T, out, in *Span)) {
	incomingCh := make(chan *Span, 1)
	addrCh := make(chan net.Addr, 1)
	go func() {
		lis, err := net.Listen("tcp", "")
		if err != nil {
			t.Errorf("Failed to listen: %v", err)
		}
		addrCh <- lis.Addr()

		s := grpc.NewServer(grpc.UnaryInterceptor(tc.GRPCServerInterceptor()))
		pb.RegisterGreeterServer(s, &grpcServer{
			fn: func(ctx context.Context, in *pb.HelloRequest) (*pb.HelloReply, error) {
				incomingCh <- FromContext(ctx)
				return &pb.HelloReply{}, nil
			},
		})
		if err := s.Serve(lis); err != nil {
			t.Errorf("Failed to serve: %v", err)
		}
	}()

	addr := <-addrCh
	conn, err := grpc.Dial(addr.String(), grpc.WithInsecure(), grpc.WithBlock(), grpc.WithUnaryInterceptor(tc.GRPCClientInterceptor()))
	if err != nil {
		t.Fatalf("Did not connect: %v", err)
	}
	defer conn.Close()
	c := pb.NewGreeterClient(conn)

	outgoingCtx := NewContext(context.Background(), parent)
	_, err = c.SayHello(outgoingCtx, &pb.HelloRequest{})
	if err != nil {
		log.Fatalf("Could not SayHello: %v", err)
	}

	assert(t, parent, <-incomingCh)
}

type noopTransport struct{}

func (rt *noopTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp := &http.Response{
		Status:     "200 OK",
		StatusCode: 200,
		Body:       ioutil.NopCloser(strings.NewReader("{}")),
	}
	return resp, nil
}

type grpcServer struct {
	fn func(ctx context.Context, in *pb.HelloRequest) (*pb.HelloReply, error)
}

func (s *grpcServer) SayHello(ctx context.Context, in *pb.HelloRequest) (*pb.HelloReply, error) {
	return s.fn(ctx, in)
}
