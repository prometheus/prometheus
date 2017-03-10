// Copyright 2017 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package remote

import (
	"sync"
	"time"

	"github.com/prometheus/common/model"
	"golang.org/x/net/context"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/storage/local"
	"github.com/prometheus/prometheus/storage/metric"
)

// Reader allows reading from multiple remote sources.
type Reader struct {
	mtx     sync.Mutex
	clients []*Client
}

// ApplyConfig updates the state as the new config requires.
func (r *Reader) ApplyConfig(conf *config.Config) error {
	clients := []*Client{}
	for i, rrConf := range conf.RemoteReadConfigs {
		c, err := NewClient(i, &clientConfig{
			url:       rrConf.URL,
			tlsConfig: rrConf.TLSConfig,
			proxyURL:  &rrConf.ProxyURL,
			basicAuth: rrConf.BasicAuth,
			timeout:   rrConf.RemoteTimeout,
		})
		if err != nil {
			return err
		}
		clients = append(clients, c)
	}

	r.mtx.Lock()
	defer r.mtx.Unlock()

	r.clients = clients

	return nil
}

// Queriers returns a list of Queriers for the currently configured
// remote read endpoints.
func (r *Reader) Queriers() []local.Querier {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	queriers := make([]local.Querier, 0, len(r.clients))
	for _, c := range r.clients {
		queriers = append(queriers, &querier{client: c})
	}
	return queriers
}

// querier is an adapter to make a Client usable as a promql.Querier.
type querier struct {
	client *Client
}

func (q *querier) QueryRange(ctx context.Context, from, through model.Time, matchers ...*metric.LabelMatcher) ([]local.SeriesIterator, error) {
	return matrixToIterators(q.client.Read(ctx, from, through, matchers))
}

func (q *querier) QueryInstant(ctx context.Context, ts model.Time, stalenessDelta time.Duration, matchers ...*metric.LabelMatcher) ([]local.SeriesIterator, error) {
	return matrixToIterators(q.client.Read(ctx, ts.Add(-stalenessDelta), ts, matchers))
}

func matrixToIterators(m model.Matrix, err error) ([]local.SeriesIterator, error) {
	if err != nil {
		return nil, err
	}

	its := make([]local.SeriesIterator, 0, len(m))
	for _, ss := range m {
		its = append(its, sampleStreamIterator{
			ss: ss,
		})
	}
	return its, nil
}

func (q *querier) MetricsForLabelMatchers(ctx context.Context, from, through model.Time, matcherSets ...metric.LabelMatchers) ([]metric.Metric, error) {
	// TODO: Implement remote metadata querying.
	return nil, nil
}

func (q *querier) LastSampleForLabelMatchers(ctx context.Context, cutoff model.Time, matcherSets ...metric.LabelMatchers) (model.Vector, error) {
	// TODO: Implement remote last sample querying.
	return nil, nil
}

func (q *querier) LabelValuesForLabelName(ctx context.Context, ln model.LabelName) (model.LabelValues, error) {
	// TODO: Implement remote metadata querying.
	return nil, nil
}

func (q *querier) Close() error {
	return nil
}
