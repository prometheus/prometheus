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

package tsdb

import (
	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/tsdb/index"
)

func labelValuesWithMatchers(r IndexReader, name string, matchers ...*labels.Matcher) ([]string, error) {
	// We're only interested in metrics which have the label <name>.
	requireLabel, err := labels.NewMatcher(labels.MatchNotEqual, name, "")
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to instantiate label matcher")
	}

	var p index.Postings
	p, err = PostingsForMatchers(r, append(matchers, requireLabel)...)
	if err != nil {
		return nil, err
	}

	dedupe := map[string]interface{}{}
	for p.Next() {
		v, err := r.LabelValueFor(p.At(), name)
		if err != nil {
			return nil, err
		}
		dedupe[v] = nil
	}

	if err = p.Err(); err != nil {
		return nil, err
	}

	values := make([]string, 0, len(dedupe))
	for value := range dedupe {
		values = append(values, value)
	}

	return values, nil
}
