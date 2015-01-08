// Copyright 2013 Prometheus Team
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
	"github.com/prometheus/client_golang/extraction"

	clientmodel "github.com/prometheus/client_golang/model"
)

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

// Ingest ingests the provided extraction result by sending it to i.
func (i ChannelIngester) Ingest(s clientmodel.Samples) error {
	i <- s
	return nil
}
