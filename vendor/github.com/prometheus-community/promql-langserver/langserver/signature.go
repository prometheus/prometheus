// Copyright 2019 Tobias Guggenmos
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

package langserver

import (
	"context"
	"errors"

	"github.com/prometheus-community/promql-langserver/internal/vendored/go-tools/lsp/protocol"
	promql "github.com/prometheus/prometheus/promql/parser"
)

// SignatureHelp is required by the protocol.Server interface.
func (s *server) SignatureHelp(ctx context.Context, params *protocol.SignatureHelpParams) (*protocol.SignatureHelp, error) {
	location, err := s.cache.Find(&params.TextDocumentPositionParams)
	if err != nil {
		return nil, nil
	}

	call, ok := location.Node.(*promql.Call)
	if !ok {
		return nil, nil
	}

	signature, err := getSignature(call.Func.Name)
	if err != nil {
		return nil, nil
	}

	activeParameter := 0.

	for i, arg := range call.Args {
		if arg != nil && arg.PositionRange().End < promql.Pos(location.Pos-location.Query.Pos) {
			activeParameter = float64(i) + 1
		}
	}

	// For the label_join function, which has a variable number of arguments,
	// the "..." should be highlighted at some point.
	// For reference, the signature is:
	// label_join(v instant-vector, dst_label string, separator string, src_label_1 string, src_label_2 string, ...)
	if call.Func.Name == "label_join" && activeParameter >= 5 {
		activeParameter = 5
	}

	response := &protocol.SignatureHelp{
		Signatures:      []protocol.SignatureInformation{signature},
		ActiveParameter: activeParameter,
	}

	return response, nil
}

// nolint: funlen
func getSignature(name string) (protocol.SignatureInformation, error) {
	var signatures = map[string]protocol.SignatureInformation{
		"abs": {
			Label: "abs(v instant-vector)",
			Parameters: []protocol.ParameterInformation{
				{Label: "v instant-vector"},
			},
		},
		"absent": {
			Label: "absent(v instant-vector)",
			Parameters: []protocol.ParameterInformation{
				{Label: "v instant-vector"},
			},
		},
		"ceil": {
			Label: "ceil(v instant-vector)",
			Parameters: []protocol.ParameterInformation{
				{Label: "v instant-vector"},
			},
		},
		"clamp_max": {
			Label: "clamp_max(v instant-vector, max scalar)",
			Parameters: []protocol.ParameterInformation{
				{Label: "v instant-vector"},
				{Label: "max scalar"},
			},
		},
		"clamp_min": {
			Label: "clamp_min(v instant-vector, min scalar)",
			Parameters: []protocol.ParameterInformation{
				{Label: "v instant-vector"},
				{Label: "min scalar"},
			},
		},
		"day_of_month": {
			Label: "day_of_month(v=vector(time()) instant-vector)",
			Parameters: []protocol.ParameterInformation{
				{Label: "v=vector(time()) instant-vector"},
			},
		},
		"day_of_week": {
			Label: "day_of_week(v=vector(time()) instant-vector)",
			Parameters: []protocol.ParameterInformation{
				{Label: "v=vector(time()) instant-vector"},
			},
		},
		"day_in_month": {
			Label: "day_in_month(v=vector(time()) instant-vector)",
			Parameters: []protocol.ParameterInformation{
				{Label: "v=vector(time()) instant-vector"},
			},
		},
		"delta": {
			Label: "delta(v range-vector)",
			Parameters: []protocol.ParameterInformation{
				{Label: "v range-vector"},
			},
		},
		"deriv": {
			Label: "deriv(v range-vector)",
			Parameters: []protocol.ParameterInformation{
				{Label: "v range-vector"},
			},
		},
		"exp": {
			Label: "exp(v instant-vector)",
			Parameters: []protocol.ParameterInformation{
				{Label: "v instant-vector"},
			},
		},
		"floor": {
			Label: "floor(v instant-vector)",
			Parameters: []protocol.ParameterInformation{
				{Label: "v instant-vector"},
			},
		},
		"histogram_quantile": {
			Label: "histogram_quantile(φ float, b instant-vector)",
			Parameters: []protocol.ParameterInformation{
				{Label: "φ float"},
				{Label: "b instant-vector"},
			},
		},
		"holt_winters": {
			Label: "holt_winters(v range-vector, sf scalar, tf scalar)",
			Parameters: []protocol.ParameterInformation{
				{Label: "v range-vector"},
				{Label: "sf scalar"},
				{Label: "tf scalar"},
			},
		},
		"hour": {
			Label: "hour(v=vector(time()) instant-vector)",
			Parameters: []protocol.ParameterInformation{
				{Label: "v=vector(time()) instant-vector"},
			},
		},
		"idelta": {
			Label: "idelta(v range-vector)",
			Parameters: []protocol.ParameterInformation{
				{Label: "v range-vector"},
			},
		},
		"increase": {
			Label: "increase(v range-vector)",
			Parameters: []protocol.ParameterInformation{
				{Label: "v range-vector"},
			},
		},
		"irate": {
			Label: "irate(v range-vector)",
			Parameters: []protocol.ParameterInformation{
				{Label: "v range-vector"},
			},
		},
		"label_join": {
			Label: "label_join(v instant-vector, dst_label string, separator string, src_label_1 string, src_label_2 string, ...)",
			Parameters: []protocol.ParameterInformation{
				{Label: "v instant-vector"},
				{Label: "dst_label string"},
				{Label: "separator string"},
				{Label: "src_label_1 string"},
				{Label: "src_label_2 string"},
				{Label: "..."},
			},
		},
		"label_replace": {
			Label: "label_replace(v instant-vector, dst_label string, replacement string, src_label string, regex string)",
			Parameters: []protocol.ParameterInformation{
				{Label: "v instant-vector"},
				{Label: "dst_label string"},
				{Label: "replacement string"},
				{Label: "src_label string"},
				{Label: "regex string"},
			},
		},
		"ln": {
			Label: "ln(v instant-vector)",
			Parameters: []protocol.ParameterInformation{
				{Label: "v instant-vector"},
			},
		},
		"log2": {
			Label: "log2(v instant-vector)",
			Parameters: []protocol.ParameterInformation{
				{Label: "v instant-vector"},
			},
		},
		"log10": {
			Label: "log10(v instant-vector)",
			Parameters: []protocol.ParameterInformation{
				{Label: "v instant-vector"},
			},
		},
		"minute": {
			Label: "minute(v=vector(time()) instant-vector)",
			Parameters: []protocol.ParameterInformation{
				{Label: "v=vector(time()) instant-vector"},
			},
		},
		"month": {
			Label: "month(v=vector(time()) instant-vector)",
			Parameters: []protocol.ParameterInformation{
				{Label: "v=vector(time()) instant-vector"},
			},
		},
		"predict_linear": {
			Label: "predict_linear(v range-vector, t scalar)",
			Parameters: []protocol.ParameterInformation{
				{Label: "v range-vector"},
				{Label: "t scalar"},
			},
		},
		"rate": {
			Label: "rate(v range-vector)",
			Parameters: []protocol.ParameterInformation{
				{Label: "v range-vector"},
			},
		},
		"resets": {
			Label: "resets(v range-vector)",
			Parameters: []protocol.ParameterInformation{
				{Label: "v range-vector"},
			},
		},
		"round": {
			Label: "round(v instant-vector, to_nearest=1 scalar)",
			Parameters: []protocol.ParameterInformation{
				{Label: "v instant-vector"},
				{Label: " to_nearest=1 scalar"},
			},
		},
		"scalar": {
			Label: "scalar(v instant-vector)",
			Parameters: []protocol.ParameterInformation{
				{Label: "v instant-vector"},
			},
		},
		"sort": {
			Label: "sort(v instant-vector)",
			Parameters: []protocol.ParameterInformation{
				{Label: "v instant-vector"},
			},
		},
		"sort_desc": {
			Label: "sort_desc(v instant-vector)",
			Parameters: []protocol.ParameterInformation{
				{Label: "v instant-vector"},
			},
		},
		"time": {
			Label:      "time()",
			Parameters: []protocol.ParameterInformation{},
		},
		"timestamp": {
			Label: "timestamp(v instant-vector)",
			Parameters: []protocol.ParameterInformation{
				{Label: "v instant-vector"},
			},
		},
		"vector": {
			Label: "vector(s scalar)",
			Parameters: []protocol.ParameterInformation{
				{Label: "s scalar"},
			},
		},
		"year": {
			Label: "year(v=vector(time()) instant-vector)",
			Parameters: []protocol.ParameterInformation{
				{Label: "v=vector(time()) instant-vector"},
			},
		},
		"avg_over_time": {
			Label: "avg_over_time(v range-vector)",
			Parameters: []protocol.ParameterInformation{
				{Label: "v range-vector"},
			},
		},
		"sum_over_time": {
			Label: "sum_over_time(v range-vector)",
			Parameters: []protocol.ParameterInformation{
				{Label: "v range-vector"},
			},
		},
		"min_over_time": {
			Label: "min_over_time(v range-vector)",
			Parameters: []protocol.ParameterInformation{
				{Label: "v range-vector"},
			},
		},
		"max_over_time": {
			Label: "max_over_time(v range-vector)",
			Parameters: []protocol.ParameterInformation{
				{Label: "v range-vector"},
			},
		},
		"count_over_time": {
			Label: "count_over_time(v range-vector)",
			Parameters: []protocol.ParameterInformation{
				{Label: "v range-vector"},
			},
		},
		"stddev_over_time": {
			Label: "stddev_over_time(v range-vector)",
			Parameters: []protocol.ParameterInformation{
				{Label: "v range-vector"},
			},
		},
		"stdvar_over_time": {
			Label: "stdvar_over_time(v range-vector)",
			Parameters: []protocol.ParameterInformation{
				{Label: "v range-vector"},
			},
		},
		"qunatile_over_time": {
			Label: "quantile_over_time(s scalar, v range-vector)",
			Parameters: []protocol.ParameterInformation{
				{Label: "s scalar"},
				{Label: "v range-vector"},
			},
		},
	}

	ret, ok := signatures[name]
	if !ok {
		return protocol.SignatureInformation{}, errors.New("no signature found")
	}

	return ret, nil
}
