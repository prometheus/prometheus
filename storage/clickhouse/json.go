// Copyright 2017, 2018 Percona LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package clickhouse

import (
	"encoding/json"

	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage/utils/gofuzz"
)

// marshalLabels marshals Prometheus labels into JSON, appending it to b.
// It preserves an order. It is also significantly faster then json.Marshal.
// It is compatible with ClickHouse JSON functions: https://clickhouse.yandex/docs/en/functions/json_functions.html
func marshalLabels(labels []*prompb.Label, b []byte) []byte {
	if len(labels) == 0 {
		return append(b, '{', '}')
	}

	b = append(b, '{')
	for _, l := range labels {
		// add label name which can't contain runes that should be escaped
		b = append(b, '"')
		b = append(b, l.Name...)
		b = append(b, '"', ':', '"')

		// FIXME we don't handle Unicode runes correctly here (first byte >= 0x80)
		// FIXME should we escape \b? \f? What are exact rules?
		// https://github.com/Percona-Lab/PromHouse/issues/19

		// add label value while escaping some runes
		for _, c := range []byte(l.Value) {
			switch c {
			case '\\', '"':
				b = append(b, '\\', c)
			case '\n':
				b = append(b, '\\', 'n')
			case '\r':
				b = append(b, '\\', 'r')
			case '\t':
				b = append(b, '\\', 't')
			default:
				b = append(b, c)
			}
		}

		b = append(b, '"', ',')
	}
	b[len(b)-1] = '}' // replace last comma

	gofuzz.AddToCorpus("json", b)
	return b
}

// unmarshalLabels unmarshals JSON into Prometheus labels.
// It does not preserves an order.
func unmarshalLabels(b []byte) ([]prompb.Label, string, error) {
	var metricName string
	m := make(map[string]string)
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, metricName, err
	}
	res := make([]prompb.Label, 0, len(m))
	for n, v := range m {
		if n == "__name__" {
			metricName = v
		}
		res = append(res, prompb.Label{
			Name:  n,
			Value: v,
		})
	}
	return res, metricName, nil
}
