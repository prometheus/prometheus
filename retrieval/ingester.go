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

	"github.com/prometheus/client_golang/extraction"

	clientmodel "github.com/prometheus/client_golang/model"
)

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
// It returns an error if the channel is not ready to receive. This is important
// to fail fast and to not pile up ingestion requests in case of overload.
func (i ChannelIngester) Ingest(s clientmodel.Samples) error {
	select {
	case i <- s:
		return nil
	default:
		return errIngestChannelFull
	}
}
