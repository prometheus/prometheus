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
	"fmt"
	"go/token"
	"sort"
	"strconv"
	"strings"

	"github.com/pkg/errors"

	"github.com/prometheus-community/promql-langserver/internal/vendored/go-tools/lsp/protocol"
	"github.com/prometheus-community/promql-langserver/langserver/cache"
	promql "github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/util/strutil"
	"github.com/sahilm/fuzzy"
)

// Completion is required by the protocol.Server interface.
func (s *server) Completion(ctx context.Context, params *protocol.CompletionParams) (ret *protocol.CompletionList, err error) {
	location, err := s.cache.Find(&params.TextDocumentPositionParams)
	if err != nil {
		return nil, nil
	}

	ret = &protocol.CompletionList{}

	completions := &ret.Items

	switch n := location.Node.(type) {
	case *promql.Call:
		var name string

		name, err = location.Doc.GetSubstring(
			location.Query.Pos+token.Pos(location.Node.PositionRange().Start),
			location.Query.Pos+token.Pos(location.Node.PositionRange().End),
		)

		i := 0
		for j, c := range name {
			if 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' {
				i = j
			} else {
				break
			}
		}

		name = name[:i]

		if err != nil {
			return
		}
		if err = s.completeFunctionName(completions, location, name); err != nil {
			return
		}
	case *promql.VectorSelector:
		metricName := n.Name

		if posRange := n.PositionRange(); int(posRange.End-posRange.Start) == len(n.Name) {
			if err = s.completeFunctionName(completions, location, metricName); err != nil {
				return
			}
		}
		if location.Query.Pos+token.Pos(location.Node.PositionRange().Start)+token.Pos(len(metricName)) >= location.Pos {
			if err = s.completeMetricName(ctx, completions, location, metricName); err != nil {
				return
			}
		} else {
			if err = s.completeLabels(ctx, completions, location); err != nil {
				return
			}
		}
	case *promql.AggregateExpr, *promql.BinaryExpr:
		if err = s.completeLabels(ctx, completions, location); err != nil {
			return
		}
	}

	return //nolint: nakedret
}

func (s *server) completeMetricName(ctx context.Context, completions *[]protocol.CompletionItem, location *cache.Location, metricName string) error {
	allMetadata, err := s.prometheusClient.AllMetadata(ctx)
	if err != nil {
		return err
	}
	editRange, err := getEditRange(location, metricName)
	if err != nil {
		return err
	}

	names := make([]string, len(allMetadata))
	i := 0
	for name := range allMetadata {
		names[i] = name
		i++
	}
	matches := fuzzy.Find(metricName, names)
	for _, match := range matches {
		item := protocol.CompletionItem{
			Label:         match.Str,
			SortText:      fmt.Sprintf("__3__%d", match.Score),
			Kind:          12, //Value
			Documentation: allMetadata[match.Str][0].Help,
			Detail:        string(allMetadata[match.Str][0].Type),
			TextEdit: &protocol.TextEdit{
				Range:   editRange,
				NewText: match.Str,
			},
		}
		*completions = append(*completions, item)
	}

	queries, err := location.Doc.GetQueries()
	if err != nil {
		return err
	}

	for _, q := range queries {
		if rec := q.Record; rec != "" && strings.HasPrefix(rec, metricName) {
			item := protocol.CompletionItem{
				Label:            rec,
				SortText:         "__2__" + rec,
				Kind:             3, //Value
				InsertTextFormat: 2, //Snippet
				TextEdit: &protocol.TextEdit{
					Range:   editRange,
					NewText: rec,
				},
			}
			*completions = append(*completions, item)
		}
	}

	return nil
}

func (s *server) completeFunctionName(completions *[]protocol.CompletionItem, location *cache.Location, metricName string) error {
	editRange, err := getEditRange(location, metricName)
	if err != nil {
		return err
	}

	for name := range promql.Functions {
		if strings.HasPrefix(strings.ToLower(name), metricName) {
			item := protocol.CompletionItem{
				Label:            name,
				SortText:         "__1__" + name,
				Kind:             3, //Function
				InsertTextFormat: 2, //Snippet
				TextEdit: &protocol.TextEdit{
					Range:   editRange,
					NewText: name + "($1)",
				},
				Command: &protocol.Command{
					// This might create problems with non VS Code clients
					Command: "editor.action.triggerParameterHints",
				},
			}
			*completions = append(*completions, item)
		}
	}

	for name, desc := range aggregators {
		if strings.HasPrefix(strings.ToLower(name), metricName) {
			item := protocol.CompletionItem{
				Label:            name,
				SortText:         "__1__" + name,
				Kind:             3, //Function
				InsertTextFormat: 2, //Snippet
				Detail:           desc,
				TextEdit: &protocol.TextEdit{
					Range:   editRange,
					NewText: name + "($1)",
				},
			}
			*completions = append(*completions, item)
		}
	}

	return nil
}

var aggregators = map[string]string{ // nolint:gochecknoglobals
	"sum":          "calculate sum over dimensions",
	"max":          "select maximum over dimensions",
	"min":          "select minimum over dimensions",
	"avg":          "calculate the average over dimensions",
	"stddev":       "calculate population standard deviation over dimensions",
	"stdvar":       "calculate population standard variance over dimensions",
	"count":        "count number of elements in the vector",
	"count_values": "count number of elements with the same value",
	"bottomk":      "smallest k elements by sample value",
	"topk":         "largest k elements by sample value",
	"quantile":     "calculate φ-quantile (0 ≤ φ ≤ 1) over dimensions",
}

// nolint: funlen
func (s *server) completeLabels(ctx context.Context, completions *[]protocol.CompletionItem, location *cache.Location) error {
	offset := location.Node.PositionRange().Start
	l := promql.Lex(location.Query.Content[offset:])

	var (
		lastItem     promql.Item
		item         promql.Item
		lastLabel    string
		insideParen  bool
		insideBraces bool
		isLabel      bool
		isValue      bool
		wantValue    bool
	)

	for token.Pos(item.Pos)+token.Pos(len(item.Val))+token.Pos(offset)+location.Query.Pos < location.Pos {
		isLabel = false
		isValue = false

		lastItem = item
		l.NextItem(&item)

		if overscan := item.Pos + offset + promql.Pos(location.Query.Pos) - promql.Pos(location.Pos); overscan >= 0 {
			item = lastItem
			break
		}

		switch item.Typ {
		case promql.AVG, promql.BOOL, promql.BOTTOMK, promql.BY, promql.COUNT, promql.COUNT_VALUES, promql.GROUP_LEFT, promql.GROUP_RIGHT, promql.IDENTIFIER, promql.IGNORING, promql.LAND, promql.LOR, promql.LUNLESS, promql.MAX, promql.METRIC_IDENTIFIER, promql.MIN, promql.OFFSET, promql.QUANTILE, promql.STDDEV, promql.STDVAR, promql.SUM, promql.TOPK:
			if insideParen || insideBraces {
				lastLabel = item.Val
				isLabel = true
			}
		case promql.EQL, promql.NEQ:
			wantValue = true
		case promql.EQL_REGEX, promql.NEQ_REGEX:
			wantValue = false
		case promql.LEFT_PAREN:
			insideParen = true
			lastLabel = ""
		case promql.RIGHT_PAREN:
			insideParen = false
			lastLabel = ""
		case promql.LEFT_BRACE:
			insideBraces = true
			lastLabel = ""
		case promql.RIGHT_BRACE:
			insideBraces = false
			lastLabel = ""
		case promql.STRING:
			if wantValue {
				isValue = true
			}
		case promql.COMMA:
			lastLabel = ""
		case 0:
			return nil
		}
	}

	vs, ok := location.Node.(*promql.VectorSelector)
	if !ok {
		vs = nil
	}

	item.Pos += offset

	loc := *location

	if isLabel {
		loc.Node = &item
		return s.completeLabel(ctx, completions, &loc, vs)
	}

	if item.Typ == promql.COMMA || item.Typ == promql.LEFT_PAREN || item.Typ == promql.LEFT_BRACE {
		loc.Node = &promql.Item{Pos: item.Pos + 1}
		return s.completeLabel(ctx, completions, &loc, vs)
	}

	if isValue && lastLabel != "" {
		loc.Node = &item
		return s.completeLabelValue(ctx, completions, &loc, lastLabel)
	}

	if item.Typ == promql.EQL || item.Typ == promql.NEQ {
		loc.Node = &promql.Item{Pos: item.Pos + promql.Pos(len(item.Val))}
		return s.completeLabelValue(ctx, completions, &loc, lastLabel)
	}

	return nil
}

func (s *server) completeLabel(ctx context.Context, completions *[]protocol.CompletionItem, location *cache.Location, vs *promql.VectorSelector) error {
	metricName := ""

	if vs != nil {
		metricName = vs.Name
	}
	allNames, err := s.prometheusClient.LabelNames(ctx, metricName)
	if err != nil {
		// nolint: errcheck
		s.client.LogMessage(s.lifetime, &protocol.LogMessageParams{
			Type:    protocol.Error,
			Message: errors.Wrapf(err, "could not get label data from prometheus").Error(),
		})
		return err
	}

	sort.Strings(allNames)

	editRange, err := getEditRange(location, "")
	if err != nil {
		return err
	}

OUTER:
	for i, name := range allNames {
		// Skip duplicates
		if i > 0 && allNames[i-1] == name {
			continue
		}

		if strings.HasPrefix(name, location.Node.(*promql.Item).Val) {
			// Skip labels that already have matchers
			if vs != nil {
				for _, m := range vs.LabelMatchers {
					if m != nil && m.Name == name {
						continue OUTER
					}
				}
			}

			item := protocol.CompletionItem{
				Label: name,
				Kind:  12, //Value
				TextEdit: &protocol.TextEdit{
					Range:   editRange,
					NewText: name,
				},
			}

			*completions = append(*completions, item)
		}
	}

	return nil
}

// nolint: funlen
func (s *server) completeLabelValue(ctx context.Context, completions *[]protocol.CompletionItem, location *cache.Location, labelName string) error {
	labelValues, err := s.prometheusClient.LabelValues(ctx, labelName)
	if err != nil {
		// nolint: errcheck
		s.client.LogMessage(s.lifetime, &protocol.LogMessageParams{
			Type:    protocol.Error,
			Message: errors.Wrapf(err, "could not get label value data from Prometheus").Error(),
		})
		return err
	}

	editRange, err := getEditRange(location, "")
	if err != nil {
		return err
	}

	quoted := location.Node.(*promql.Item).Val

	var quote byte

	var unquoted string

	if len(quoted) != 0 {
		quote = quoted[0]

		unquoted, err = strutil.Unquote(quoted)
		if err != nil {
			return nil
		}
	} else {
		quote = '"'
	}

	for _, name := range labelValues {
		if strings.HasPrefix(string(name), unquoted) {
			var quoted string

			if quote == '`' {
				if strings.ContainsRune(string(name), '`') {
					quote = '"'
				} else {
					quoted = fmt.Sprint("`", name, "`")
				}
			}

			if quoted == "" {
				quoted = strconv.Quote(string(name))
			}

			if quote == '\'' {
				quoted = quoted[1 : len(quoted)-1]

				quoted = strings.ReplaceAll(quoted, `\"`, `"`)
				quoted = strings.ReplaceAll(quoted, `'`, `\'`)
				quoted = fmt.Sprint("'", quoted, "'")
			}

			item := protocol.CompletionItem{
				Label: quoted,
				Kind:  12, //Value
				TextEdit: &protocol.TextEdit{
					Range:   editRange,
					NewText: quoted,
				},
			}
			*completions = append(*completions, item)
		}
	}

	return nil
}

// getEditRange computes the editRange for a completion. In case the completion area is shorter than
// the node, the oldname of the token to be completed must be provided. The latter mechanism only
// works if oldname is an ASCII string, which can be safely assumed for metric and function names.
func getEditRange(location *cache.Location, oldname string) (editRange protocol.Range, err error) {
	editRange.Start, err = location.Doc.PosToProtocolPosition(location.Query.Pos + token.Pos(location.Node.PositionRange().Start))
	if err != nil {
		return
	}

	if oldname == "" {
		editRange.End, err = location.Doc.PosToProtocolPosition(location.Query.Pos + token.Pos(location.Node.PositionRange().End))
		if err != nil {
			return
		}
	} else {
		editRange.End = editRange.Start
		editRange.End.Character += float64(len(oldname))
	}

	return
}
