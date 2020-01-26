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

package spanner

import (
	"context"

	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
)

const statsPrefix = "cloud.google.com/go/spanner/"

func recordStat(ctx context.Context, m *stats.Int64Measure, n int64) {
	stats.Record(ctx, m.M(n))
}

var (
	// OpenSessionCount is a measure of the number of sessions currently opened.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	OpenSessionCount = stats.Int64(statsPrefix+"open_session_count", "Number of sessions currently opened",
		stats.UnitDimensionless)

	// OpenSessionCountView is a view of the last value of OpenSessionCount.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	OpenSessionCountView = &view.View{
		Name:        OpenSessionCount.Name(),
		Description: OpenSessionCount.Description(),
		Measure:     OpenSessionCount,
		Aggregation: view.LastValue(),
	}
)
