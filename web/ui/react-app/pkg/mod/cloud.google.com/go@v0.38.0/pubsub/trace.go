// Copyright 2018 Google LLC
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

package pubsub

import (
	"context"
	"log"
	"sync"

	"go.opencensus.io/plugin/ocgrpc"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
)

func openCensusOptions() []option.ClientOption {
	return []option.ClientOption{
		option.WithGRPCDialOption(grpc.WithStatsHandler(&ocgrpc.ClientHandler{})),
	}
}

var subscriptionKey tag.Key

func init() {
	var err error
	if subscriptionKey, err = tag.NewKey("subscription"); err != nil {
		log.Fatal("cannot create 'subscription' key")
	}
}

const statsPrefix = "cloud.google.com/go/pubsub/"

var (
	// PullCount is a measure of the number of messages pulled.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	PullCount = stats.Int64(statsPrefix+"pull_count", "Number of PubSub messages pulled", stats.UnitDimensionless)

	// AckCount is a measure of the number of messages acked.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	AckCount = stats.Int64(statsPrefix+"ack_count", "Number of PubSub messages acked", stats.UnitDimensionless)

	// NackCount is a measure of the number of messages nacked.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	NackCount = stats.Int64(statsPrefix+"nack_count", "Number of PubSub messages nacked", stats.UnitDimensionless)

	// ModAckCount is a measure of the number of messages whose ack-deadline was modified.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	ModAckCount = stats.Int64(statsPrefix+"mod_ack_count", "Number of ack-deadlines modified", stats.UnitDimensionless)

	// ModAckTimeoutCount is a measure of the number ModifyAckDeadline RPCs that timed out.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	ModAckTimeoutCount = stats.Int64(statsPrefix+"mod_ack_timeout_count", "Number of ModifyAckDeadline RPCs that timed out", stats.UnitDimensionless)

	// StreamOpenCount is a measure of the number of times a streaming-pull stream was opened.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	StreamOpenCount = stats.Int64(statsPrefix+"stream_open_count", "Number of calls opening a new streaming pull", stats.UnitDimensionless)

	// StreamRetryCount is a measure of the number of times a streaming-pull operation was retried.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	StreamRetryCount = stats.Int64(statsPrefix+"stream_retry_count", "Number of retries of a stream send or receive", stats.UnitDimensionless)

	// StreamRequestCount is a measure of the number of requests sent on a streaming-pull stream.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	StreamRequestCount = stats.Int64(statsPrefix+"stream_request_count", "Number gRPC StreamingPull request messages sent", stats.UnitDimensionless)

	// StreamResponseCount is a measure of the number of responses received on a streaming-pull stream.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	StreamResponseCount = stats.Int64(statsPrefix+"stream_response_count", "Number of gRPC StreamingPull response messages received", stats.UnitDimensionless)

	// PullCountView is a cumulative sum of PullCount.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	PullCountView *view.View

	// AckCountView is a cumulative sum of AckCount.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	AckCountView *view.View

	// NackCountView is a cumulative sum of NackCount.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	NackCountView *view.View

	// ModAckCountView is a cumulative sum of ModAckCount.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	ModAckCountView *view.View

	// ModAckTimeoutCountView is a cumulative sum of ModAckTimeoutCount.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	ModAckTimeoutCountView *view.View

	// StreamOpenCountView is a cumulative sum of StreamOpenCount.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	StreamOpenCountView *view.View

	// StreamRetryCountView is a cumulative sum of StreamRetryCount.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	StreamRetryCountView *view.View

	// StreamRequestCountView is a cumulative sum of StreamRequestCount.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	StreamRequestCountView *view.View

	// StreamResponseCountView is a cumulative sum of StreamResponseCount.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	StreamResponseCountView *view.View
)

func init() {
	PullCountView = countView(PullCount)
	AckCountView = countView(AckCount)
	NackCountView = countView(NackCount)
	ModAckCountView = countView(ModAckCount)
	ModAckTimeoutCountView = countView(ModAckTimeoutCount)
	StreamOpenCountView = countView(StreamOpenCount)
	StreamRetryCountView = countView(StreamRetryCount)
	StreamRequestCountView = countView(StreamRequestCount)
	StreamResponseCountView = countView(StreamResponseCount)
}

func countView(m *stats.Int64Measure) *view.View {
	return &view.View{
		Name:        m.Name(),
		Description: m.Description(),
		TagKeys:     []tag.Key{subscriptionKey},
		Measure:     m,
		Aggregation: view.Sum(),
	}
}

var logOnce sync.Once

func withSubscriptionKey(ctx context.Context, subName string) context.Context {
	ctx, err := tag.New(ctx, tag.Upsert(subscriptionKey, subName))
	if err != nil {
		logOnce.Do(func() {
			log.Printf("pubsub: error creating tag map: %v", err)
		})
	}
	return ctx
}

func recordStat(ctx context.Context, m *stats.Int64Measure, n int64) {
	stats.Record(ctx, m.M(n))
}
