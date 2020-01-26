/*
 *
 * Copyright 2019 gRPC authors.
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

package xds

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	anypb "github.com/golang/protobuf/ptypes/any"
	durationpb "github.com/golang/protobuf/ptypes/duration"
	structpb "github.com/golang/protobuf/ptypes/struct"
	wrpb "github.com/golang/protobuf/ptypes/wrappers"
	"google.golang.org/grpc"
	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/balancer/xds/internal"
	cdspb "google.golang.org/grpc/balancer/xds/internal/proto/envoy/api/v2/cds"
	addresspb "google.golang.org/grpc/balancer/xds/internal/proto/envoy/api/v2/core/address"
	basepb "google.golang.org/grpc/balancer/xds/internal/proto/envoy/api/v2/core/base"
	discoverypb "google.golang.org/grpc/balancer/xds/internal/proto/envoy/api/v2/discovery"
	edspb "google.golang.org/grpc/balancer/xds/internal/proto/envoy/api/v2/eds"
	endpointpb "google.golang.org/grpc/balancer/xds/internal/proto/envoy/api/v2/endpoint/endpoint"
	adsgrpc "google.golang.org/grpc/balancer/xds/internal/proto/envoy/service/discovery/v2/ads"
	lrsgrpc "google.golang.org/grpc/balancer/xds/internal/proto/envoy/service/load_stats/v2/lrs"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/resolver"
	"google.golang.org/grpc/status"
)

var (
	testServiceName = "test/foo"
	testCDSReq      = &discoverypb.DiscoveryRequest{
		Node: &basepb.Node{
			Metadata: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					internal.GrpcHostname: {
						Kind: &structpb.Value_StringValue{StringValue: testServiceName},
					},
				},
			},
		},
		TypeUrl: cdsType,
	}
	testEDSReq = &discoverypb.DiscoveryRequest{
		Node: &basepb.Node{
			Metadata: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					internal.GrpcHostname: {
						Kind: &structpb.Value_StringValue{StringValue: testServiceName},
					},
					endpointRequired: {
						Kind: &structpb.Value_BoolValue{BoolValue: true},
					},
				},
			},
		},
		TypeUrl: edsType,
	}
	testEDSReqWithoutEndpoints = &discoverypb.DiscoveryRequest{
		Node: &basepb.Node{
			Metadata: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					internal.GrpcHostname: {
						Kind: &structpb.Value_StringValue{StringValue: testServiceName},
					},
					endpointRequired: {
						Kind: &structpb.Value_BoolValue{BoolValue: false},
					},
				},
			},
		},
		TypeUrl: edsType,
	}
	testCluster = &cdspb.Cluster{
		Name:                 testServiceName,
		ClusterDiscoveryType: &cdspb.Cluster_Type{Type: cdspb.Cluster_EDS},
		LbPolicy:             cdspb.Cluster_ROUND_ROBIN,
	}
	marshaledCluster, _ = proto.Marshal(testCluster)
	testCDSResp         = &discoverypb.DiscoveryResponse{
		Resources: []*anypb.Any{
			{
				TypeUrl: cdsType,
				Value:   marshaledCluster,
			},
		},
		TypeUrl: cdsType,
	}
	testClusterLoadAssignment = &edspb.ClusterLoadAssignment{
		ClusterName: testServiceName,
		Endpoints: []*endpointpb.LocalityLbEndpoints{
			{
				Locality: &basepb.Locality{
					Region:  "asia-east1",
					Zone:    "1",
					SubZone: "sa",
				},
				LbEndpoints: []*endpointpb.LbEndpoint{
					{
						HostIdentifier: &endpointpb.LbEndpoint_Endpoint{
							Endpoint: &endpointpb.Endpoint{
								Address: &addresspb.Address{
									Address: &addresspb.Address_SocketAddress{
										SocketAddress: &addresspb.SocketAddress{
											Address: "1.1.1.1",
											PortSpecifier: &addresspb.SocketAddress_PortValue{
												PortValue: 10001,
											},
											ResolverName: "dns",
										},
									},
								},
								HealthCheckConfig: nil,
							},
						},
						Metadata: &basepb.Metadata{
							FilterMetadata: map[string]*structpb.Struct{
								"xx.lb": {
									Fields: map[string]*structpb.Value{
										"endpoint_name": {
											Kind: &structpb.Value_StringValue{
												StringValue: "some.endpoint.name",
											},
										},
									},
								},
							},
						},
					},
				},
				LoadBalancingWeight: &wrpb.UInt32Value{
					Value: 1,
				},
				Priority: 0,
			},
		},
	}
	marshaledClusterLoadAssignment, _ = proto.Marshal(testClusterLoadAssignment)
	testEDSResp                       = &discoverypb.DiscoveryResponse{
		Resources: []*anypb.Any{
			{
				TypeUrl: edsType,
				Value:   marshaledClusterLoadAssignment,
			},
		},
		TypeUrl: edsType,
	}
	testClusterLoadAssignmentWithoutEndpoints = &edspb.ClusterLoadAssignment{
		ClusterName: testServiceName,
		Endpoints: []*endpointpb.LocalityLbEndpoints{
			{
				Locality: &basepb.Locality{
					SubZone: "sa",
				},
				LoadBalancingWeight: &wrpb.UInt32Value{
					Value: 128,
				},
				Priority: 0,
			},
		},
		Policy: nil,
	}
	marshaledClusterLoadAssignmentWithoutEndpoints, _ = proto.Marshal(testClusterLoadAssignmentWithoutEndpoints)
	testEDSRespWithoutEndpoints                       = &discoverypb.DiscoveryResponse{
		Resources: []*anypb.Any{
			{
				TypeUrl: edsType,
				Value:   marshaledClusterLoadAssignmentWithoutEndpoints,
			},
		},
		TypeUrl: edsType,
	}
)

type testTrafficDirector struct {
	reqChan  chan *request
	respChan chan *response
}

type request struct {
	req *discoverypb.DiscoveryRequest
	err error
}

type response struct {
	resp *discoverypb.DiscoveryResponse
	err  error
}

func (ttd *testTrafficDirector) StreamAggregatedResources(s adsgrpc.AggregatedDiscoveryService_StreamAggregatedResourcesServer) error {
	for {
		req, err := s.Recv()
		if err != nil {
			ttd.reqChan <- &request{
				req: nil,
				err: err,
			}
			if err == io.EOF {
				return nil
			}
			return err
		}
		ttd.reqChan <- &request{
			req: req,
			err: nil,
		}
		if req.TypeUrl == edsType {
			break
		}
	}

	for {
		select {
		case resp := <-ttd.respChan:
			if resp.err != nil {
				return resp.err
			}
			if err := s.Send(resp.resp); err != nil {
				return err
			}
		case <-s.Context().Done():
			return s.Context().Err()
		}
	}
}

func (ttd *testTrafficDirector) DeltaAggregatedResources(adsgrpc.AggregatedDiscoveryService_DeltaAggregatedResourcesServer) error {
	return status.Error(codes.Unimplemented, "")
}

func (ttd *testTrafficDirector) sendResp(resp *response) {
	ttd.respChan <- resp
}

func (ttd *testTrafficDirector) getReq() *request {
	return <-ttd.reqChan
}

func newTestTrafficDirector() *testTrafficDirector {
	return &testTrafficDirector{
		reqChan:  make(chan *request, 10),
		respChan: make(chan *response, 10),
	}
}

type testConfig struct {
	doCDS                bool
	expectedRequests     []*discoverypb.DiscoveryRequest
	responsesToSend      []*discoverypb.DiscoveryResponse
	expectedADSResponses []proto.Message
	adsErr               error
	svrErr               error
}

func setupServer(t *testing.T) (addr string, td *testTrafficDirector, lrss *lrsServer, cleanup func()) {
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("listen failed due to: %v", err)
	}
	svr := grpc.NewServer()
	td = newTestTrafficDirector()
	lrss = &lrsServer{
		drops: make(map[string]uint64),
		reportingInterval: &durationpb.Duration{
			Seconds: 60 * 60, // 1 hour, each test can override this to a shorter duration.
			Nanos:   0,
		},
	}
	adsgrpc.RegisterAggregatedDiscoveryServiceServer(svr, td)
	lrsgrpc.RegisterLoadReportingServiceServer(svr, lrss)
	go svr.Serve(lis)
	return lis.Addr().String(), td, lrss, func() {
		svr.Stop()
		lis.Close()
	}
}

func (s) TestXdsClientResponseHandling(t *testing.T) {
	for _, test := range []*testConfig{
		{
			doCDS:                true,
			expectedRequests:     []*discoverypb.DiscoveryRequest{testCDSReq, testEDSReq},
			responsesToSend:      []*discoverypb.DiscoveryResponse{testCDSResp, testEDSResp},
			expectedADSResponses: []proto.Message{testCluster, testClusterLoadAssignment},
		},
		{
			doCDS:                false,
			expectedRequests:     []*discoverypb.DiscoveryRequest{testEDSReqWithoutEndpoints},
			responsesToSend:      []*discoverypb.DiscoveryResponse{testEDSRespWithoutEndpoints},
			expectedADSResponses: []proto.Message{testClusterLoadAssignmentWithoutEndpoints},
		},
	} {
		testXdsClientResponseHandling(t, test)
	}
}

func testXdsClientResponseHandling(t *testing.T, test *testConfig) {
	addr, td, _, cleanup := setupServer(t)
	defer cleanup()
	adsChan := make(chan proto.Message, 10)
	newADS := func(ctx context.Context, i proto.Message) error {
		adsChan <- i
		return nil
	}
	client := newXDSClient(addr, test.doCDS, balancer.BuildOptions{Target: resolver.Target{Endpoint: testServiceName}}, nil, newADS, func(context.Context) {}, func() {})
	defer client.close()
	go client.run()

	for _, expectedReq := range test.expectedRequests {
		req := td.getReq()
		if req.err != nil {
			t.Fatalf("ads RPC failed with err: %v", req.err)
		}
		if !proto.Equal(req.req, expectedReq) {
			t.Fatalf("got ADS request %T %v, expected: %T %v", req.req, req.req, expectedReq, expectedReq)
		}
	}

	for i, resp := range test.responsesToSend {
		td.sendResp(&response{resp: resp})
		ads := <-adsChan
		if !proto.Equal(ads, test.expectedADSResponses[i]) {
			t.Fatalf("received unexpected ads response, got %v, want %v", ads, test.expectedADSResponses[i])
		}
	}
}

func (s) TestXdsClientLoseContact(t *testing.T) {
	for _, test := range []*testConfig{
		{
			doCDS:           true,
			responsesToSend: []*discoverypb.DiscoveryResponse{},
		},
		{
			doCDS:           false,
			responsesToSend: []*discoverypb.DiscoveryResponse{testEDSRespWithoutEndpoints},
		},
	} {
		testXdsClientLoseContactRemoteClose(t, test)
	}

	for _, test := range []*testConfig{
		{
			doCDS:           false,
			responsesToSend: []*discoverypb.DiscoveryResponse{testCDSResp}, // CDS response when in custom mode.
		},
		{
			doCDS:           true,
			responsesToSend: []*discoverypb.DiscoveryResponse{{}}, // response with 0 resources is an error case.
		},
		{
			doCDS:           true,
			responsesToSend: []*discoverypb.DiscoveryResponse{testCDSResp},
			adsErr:          errors.New("some ads parsing error from xdsBalancer"),
		},
	} {
		testXdsClientLoseContactADSRelatedErrorOccur(t, test)
	}
}

func testXdsClientLoseContactRemoteClose(t *testing.T, test *testConfig) {
	addr, td, _, cleanup := setupServer(t)
	defer cleanup()
	adsChan := make(chan proto.Message, 10)
	newADS := func(ctx context.Context, i proto.Message) error {
		adsChan <- i
		return nil
	}
	contactChan := make(chan *loseContact, 10)
	loseContactFunc := func(context.Context) {
		contactChan <- &loseContact{}
	}
	client := newXDSClient(addr, test.doCDS, balancer.BuildOptions{Target: resolver.Target{Endpoint: testServiceName}}, nil, newADS, loseContactFunc, func() {})
	defer client.close()
	go client.run()

	// make sure server side get the request (i.e stream created successfully on client side)
	td.getReq()

	for _, resp := range test.responsesToSend {
		td.sendResp(&response{resp: resp})
		// make sure client side receives it
		<-adsChan
	}
	cleanup()

	select {
	case <-contactChan:
	case <-time.After(2 * time.Second):
		t.Fatal("time out when expecting lost contact signal")
	}
}

func testXdsClientLoseContactADSRelatedErrorOccur(t *testing.T, test *testConfig) {
	addr, td, _, cleanup := setupServer(t)
	defer cleanup()

	adsChan := make(chan proto.Message, 10)
	newADS := func(ctx context.Context, i proto.Message) error {
		adsChan <- i
		return test.adsErr
	}
	contactChan := make(chan *loseContact, 10)
	loseContactFunc := func(context.Context) {
		contactChan <- &loseContact{}
	}
	client := newXDSClient(addr, test.doCDS, balancer.BuildOptions{Target: resolver.Target{Endpoint: testServiceName}}, nil, newADS, loseContactFunc, func() {})
	defer client.close()
	go client.run()

	// make sure server side get the request (i.e stream created successfully on client side)
	td.getReq()

	for _, resp := range test.responsesToSend {
		td.sendResp(&response{resp: resp})
	}

	select {
	case <-contactChan:
	case <-time.After(2 * time.Second):
		t.Fatal("time out when expecting lost contact signal")
	}
}

func (s) TestXdsClientExponentialRetry(t *testing.T) {
	cfg := &testConfig{
		svrErr: status.Errorf(codes.Aborted, "abort the stream to trigger retry"),
	}
	addr, td, _, cleanup := setupServer(t)
	defer cleanup()

	adsChan := make(chan proto.Message, 10)
	newADS := func(ctx context.Context, i proto.Message) error {
		adsChan <- i
		return nil
	}
	contactChan := make(chan *loseContact, 10)
	loseContactFunc := func(context.Context) {
		contactChan <- &loseContact{}
	}
	client := newXDSClient(addr, cfg.doCDS, balancer.BuildOptions{Target: resolver.Target{Endpoint: testServiceName}}, nil, newADS, loseContactFunc, func() {})
	defer client.close()
	go client.run()

	var secondRetry, thirdRetry time.Time
	for i := 0; i < 3; i++ {
		// make sure server side get the request (i.e stream created successfully on client side)
		td.getReq()
		td.sendResp(&response{err: cfg.svrErr})

		select {
		case <-contactChan:
			if i == 1 {
				secondRetry = time.Now()
			}
			if i == 2 {
				thirdRetry = time.Now()
			}
		case <-time.After(2 * time.Second):
			t.Fatal("time out when expecting lost contact signal")
		}
	}
	if thirdRetry.Sub(secondRetry) < 1*time.Second {
		t.Fatalf("interval between second and third retry is %v, expected > 1s", thirdRetry.Sub(secondRetry))
	}
}
