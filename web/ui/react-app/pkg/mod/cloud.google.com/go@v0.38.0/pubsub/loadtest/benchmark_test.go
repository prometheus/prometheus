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

package loadtest

// Performance benchmarks for pubsub.
// Run with
//   go test -bench . -cpu 1

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/pubsub"
	"google.golang.org/api/option"
	gtransport "google.golang.org/api/transport/grpc"
	pb "google.golang.org/genproto/googleapis/pubsub/v1"
	"google.golang.org/grpc"
)

// These constants are designed to match the "throughput" test in
// https://github.com/GoogleCloudPlatform/pubsub/blob/master/load-test-framework/run.py
// and
// https://github.com/GoogleCloudPlatform/pubsub/blob/master/load-test-framework/src/main/java/com/google/pubsub/clients/experimental/CPSPublisherTask.java

const (
	nMessages               = 1e5
	messageSize             = 10000 // size of msg data in bytes
	batchSize               = 10
	batchDuration           = 50 * time.Millisecond
	serverDelay             = 200 * time.Millisecond
	maxOutstandingPublishes = 1600 // max_outstanding_messages in run.py
)

func BenchmarkPublishThroughput(b *testing.B) {
	b.SetBytes(nMessages * messageSize)
	client := perfClient(serverDelay, 1, b)

	lts := &PubServer{ID: "xxx"}
	lts.init(client, "t", messageSize, batchSize, batchDuration)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runOnce(lts)
	}
}

func runOnce(lts *PubServer) {
	nRequests := int64(nMessages / batchSize)
	var nPublished int64
	var wg sync.WaitGroup
	// The Java loadtest framework is rate-limited to 1 billion Execute calls a
	// second (each Execute call corresponding to a publishBatch call here),
	// but we can ignore this because of the following.
	// The framework runs 10,000 threads, each calling Execute in a loop, but
	// we can ignore this too.
	// The framework caps the number of outstanding calls to Execute at
	// maxOutstandingPublishes. That is what we simulate here.
	for i := 0; i < maxOutstandingPublishes; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for atomic.AddInt64(&nRequests, -1) >= 0 {
				latencies, err := lts.publishBatch()
				if err != nil {
					log.Fatalf("publishBatch: %v", err)
				}
				atomic.AddInt64(&nPublished, int64(len(latencies)))
			}
		}()
	}
	wg.Wait()
	sent := atomic.LoadInt64(&nPublished)
	if sent != nMessages {
		log.Fatalf("sent %d messages, expected %d", sent, int(nMessages))
	}
}

func perfClient(pubDelay time.Duration, nConns int, f interface {
	Fatal(...interface{})
}) *pubsub.Client {
	ctx := context.Background()
	srv, err := newPerfServer(pubDelay)
	if err != nil {
		f.Fatal(err)
	}
	conn, err := gtransport.DialInsecure(ctx,
		option.WithEndpoint(srv.Addr),
		option.WithGRPCConnectionPool(nConns),

		// TODO(grpc/grpc-go#1388) using connection pool without WithBlock
		// can cause RPCs to fail randomly. We can delete this after the issue is fixed.
		option.WithGRPCDialOption(grpc.WithBlock()))
	if err != nil {
		f.Fatal(err)
	}
	client, err := pubsub.NewClient(ctx, "projectID", option.WithGRPCConn(conn))
	if err != nil {
		f.Fatal(err)
	}
	return client
}

type perfServer struct {
	pb.PublisherServer
	pb.SubscriberServer

	Addr     string
	pubDelay time.Duration

	mu            sync.Mutex
	activePubs    int
	maxActivePubs int
}

func newPerfServer(pubDelay time.Duration) (*perfServer, error) {
	srv, err := testutil.NewServer(grpc.MaxMsgSize(pubsub.MaxPublishRequestBytes))
	if err != nil {
		return nil, err
	}
	perf := &perfServer{Addr: srv.Addr, pubDelay: pubDelay}
	pb.RegisterPublisherServer(srv.Gsrv, perf)
	pb.RegisterSubscriberServer(srv.Gsrv, perf)
	srv.Start()
	return perf, nil
}

var doLog = false

func (p *perfServer) incActivePubs(n int) (int, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.activePubs += n
	newMax := false
	if p.activePubs > p.maxActivePubs {
		p.maxActivePubs = p.activePubs
		newMax = true
	}
	return p.activePubs, newMax
}

func (p *perfServer) Publish(ctx context.Context, req *pb.PublishRequest) (*pb.PublishResponse, error) {
	a, newMax := p.incActivePubs(1)
	defer p.incActivePubs(-1)
	if newMax && doLog {
		log.Printf("max %d active publish calls", a)
	}
	if doLog {
		log.Printf("%p -> Publish %d", p, len(req.Messages))
	}
	res := &pb.PublishResponse{MessageIds: make([]string, len(req.Messages))}
	for i := range res.MessageIds {
		res.MessageIds[i] = "x"
	}
	time.Sleep(p.pubDelay)
	if doLog {
		log.Printf("%p <- Publish %d", p, len(req.Messages))
	}
	return res, nil
}
