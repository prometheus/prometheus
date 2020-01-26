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
	"io"
	"sync"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	durationpb "github.com/golang/protobuf/ptypes/duration"
	structpb "github.com/golang/protobuf/ptypes/struct"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/balancer/xds/internal"
	basepb "google.golang.org/grpc/balancer/xds/internal/proto/envoy/api/v2/core/base"
	lrsgrpc "google.golang.org/grpc/balancer/xds/internal/proto/envoy/service/load_stats/v2/lrs"
	lrspb "google.golang.org/grpc/balancer/xds/internal/proto/envoy/service/load_stats/v2/lrs"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/resolver"
	"google.golang.org/grpc/status"
)

type lrsServer struct {
	mu                sync.Mutex
	dropTotal         uint64
	drops             map[string]uint64
	reportingInterval *durationpb.Duration
}

func (lrss *lrsServer) StreamLoadStats(stream lrsgrpc.LoadReportingService_StreamLoadStatsServer) error {
	req, err := stream.Recv()
	if err != nil {
		return err
	}
	if !proto.Equal(req, &lrspb.LoadStatsRequest{
		Node: &basepb.Node{
			Metadata: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					internal.GrpcHostname: {
						Kind: &structpb.Value_StringValue{StringValue: testServiceName},
					},
				},
			},
		},
	}) {
		return status.Errorf(codes.FailedPrecondition, "unexpected req: %+v", req)
	}
	if err := stream.Send(&lrspb.LoadStatsResponse{
		Clusters:              []string{testServiceName},
		LoadReportingInterval: lrss.reportingInterval,
	}); err != nil {
		return err
	}

	for {
		req, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		stats := req.ClusterStats[0]
		lrss.mu.Lock()
		lrss.dropTotal += stats.TotalDroppedRequests
		for _, d := range stats.DroppedRequests {
			lrss.drops[d.Category] += d.DroppedCount
		}
		lrss.mu.Unlock()
	}
}

func (s) TestXdsLoadReporting(t *testing.T) {
	originalNewEDSBalancer := newEDSBalancer
	newEDSBalancer = newFakeEDSBalancer
	defer func() {
		newEDSBalancer = originalNewEDSBalancer
	}()

	builder := balancer.Get(xdsName)
	cc := newTestClientConn()
	lb, ok := builder.Build(cc, balancer.BuildOptions{Target: resolver.Target{Endpoint: testServiceName}}).(*xdsBalancer)
	if !ok {
		t.Fatalf("unable to type assert to *xdsBalancer")
	}
	defer lb.Close()

	addr, td, lrss, cleanup := setupServer(t)
	defer cleanup()

	const intervalNano = 1000 * 1000 * 50
	lrss.reportingInterval = &durationpb.Duration{
		Seconds: 0,
		Nanos:   intervalNano,
	}

	cfg := &xdsConfig{
		BalancerName: addr,
		ChildPolicy:  &loadBalancingConfig{Name: fakeBalancerA}, // Set this to skip cds.
	}
	lb.UpdateClientConnState(balancer.ClientConnState{BalancerConfig: cfg})
	td.sendResp(&response{resp: testEDSRespWithoutEndpoints})
	var (
		i     int
		edsLB *fakeEDSBalancer
	)
	for i = 0; i < 10; i++ {
		edsLB = getLatestEdsBalancer()
		if edsLB != nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if i == 10 {
		t.Fatal("edsBalancer instance has not been created and assigned to lb.xdsLB after 1s")
	}

	var dropCategories = []string{"drop_for_real", "drop_for_fun"}
	drops := map[string]uint64{
		dropCategories[0]: 31,
		dropCategories[1]: 41,
	}

	for c, d := range drops {
		for i := 0; i < int(d); i++ {
			edsLB.loadStore.CallDropped(c)
			time.Sleep(time.Nanosecond * intervalNano / 10)
		}
	}
	time.Sleep(time.Nanosecond * intervalNano * 2)

	lrss.mu.Lock()
	defer lrss.mu.Unlock()
	if !cmp.Equal(lrss.drops, drops) {
		t.Errorf("different: %v %v %v", lrss.drops, drops, cmp.Diff(lrss.drops, drops))
	}
}
