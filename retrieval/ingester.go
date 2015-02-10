// Copyright 2013 The Prometheus Authors
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

package retrieval

import (
	"errors"
	"time"

	"github.com/prometheus/client_golang/extraction"

	clientmodel "github.com/prometheus/client_golang/model"
)

const ingestTimeout = 100 * time.Millisecond // TODO(beorn7): Adjust this to a fraction of the actual HTTP timeout.

var errIngestChannelFull = errors.New("ingestion channel full")

// MergeLabelsIngester merges a labelset ontop of a given extraction result and
// passes the result on to another ingester. Label collisions are avoided by
// appending a label prefix to any newly merged colliding labels.
type MergeLabelsIngester struct {
	Labels          clientmodel.LabelSet
	CollisionPrefix clientmodel.LabelName

	Ingester extraction.Ingester
}

// Ingest ingests the provided extraction result by merging in i.Labels and then
// handing it over to i.Ingester.
func (i *MergeLabelsIngester) Ingest(samples clientmodel.Samples) error {
	for _, s := range samples {
		s.Metric.MergeFromLabelSet(i.Labels, i.CollisionPrefix)
	}

	return i.Ingester.Ingest(samples)
}

// ChannelIngester feeds results into a channel without modifying them.
type ChannelIngester chan<- clientmodel.Samples

// Ingest ingests the provided extraction result by sending it to its channel.
// If the channel was not able to receive the samples within the ingestTimeout,
// an error is returned. This is important to fail fast and to not pile up
// ingestion requests in case of overload.
func (i ChannelIngester) Ingest(s clientmodel.Samples) error {
	// Since the regular case is that i is ready to receive, first try
	// without setting a timeout so that we don't need to allocate a timer
	// most of the time.
	select {
	case i <- s:
		return nil
	default:
		select {
		case i <- s:
			return nil
		case <-time.After(ingestTimeout):
			return errIngestChannelFull
		}
	}
}
