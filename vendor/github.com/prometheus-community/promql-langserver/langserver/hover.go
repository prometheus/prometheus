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
	"bytes"
	"context"
	"fmt"
	"go/token"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/pkg/errors"
	"github.com/prometheus-community/promql-langserver/internal/vendored/go-tools/lsp/protocol"
	promql "github.com/prometheus/prometheus/promql/parser"
	"github.com/rakyll/statik/fs"

	"github.com/prometheus-community/promql-langserver/langserver/cache"
	// Do not remove! Side effects of init() needed
	_ "github.com/prometheus-community/promql-langserver/langserver/documentation/functions_statik"
)

//nolint: gochecknoglobals
var functionDocumentationFS = initializeFunctionDocumentation()

func initializeFunctionDocumentation() http.FileSystem {
	ret, err := fs.New()
	if err != nil {
		log.Fatal(err)
	}

	return ret
}

// Hover shows documentation on hover
// required by the protocol.Server interface.
func (s *server) Hover(ctx context.Context, params *protocol.HoverParams) (*protocol.Hover, error) {
	location, err := s.cache.Find(&params.TextDocumentPositionParams)
	if err != nil || location.Node == nil {
		return nil, nil
	}

	markdown := s.nodeToDocMarkdown(ctx, location)
	hoverRange, err := getEditRange(location, "")
	if err != nil {
		return nil, nil
	}

	return &protocol.Hover{
		Contents: protocol.MarkupContent{
			Kind:  "markdown",
			Value: markdown,
		},
		Range: hoverRange,
	}, nil
}

func (s *server) nodeToDocMarkdown(ctx context.Context, location *cache.Location) string { //nolint: funlen
	var ret bytes.Buffer

	switch n := location.Node.(type) {
	case *promql.AggregateExpr:
		name := strings.ToLower(n.Op.String())

		if _, err := ret.WriteString("## "); err != nil {
			return ""
		}

		if _, err := ret.WriteString(name); err != nil {
			return ""
		}

		if desc, ok := aggregators[name]; ok {
			if _, err := ret.WriteString("\n\n"); err != nil {
				return ""
			}

			if _, err := ret.WriteString(desc); err != nil {
				return ""
			}
		}

	case *promql.Call:
		doc := funcDocStrings(n.Func.Name)

		if _, err := ret.WriteString(doc); err != nil {
			return ""
		}

	case *promql.VectorSelector:
		metric := n.Name

		if metric != "" {
			doc, err := s.getRecordingRuleDocs(location.Doc, metric)
			if err != nil {
				// nolint: errcheck
				s.client.LogMessage(s.lifetime, &protocol.LogMessageParams{
					Type:    protocol.Error,
					Message: errors.Wrapf(err, "failed to get recording rule data").Error(),
				})
			}

			if doc == "" {
				doc, err = s.getMetricDocs(ctx, metric)
				if err != nil {
					// nolint: errcheck
					s.client.LogMessage(s.lifetime, &protocol.LogMessageParams{
						Type:    protocol.Error,
						Message: errors.Wrapf(err, "failed to get metric data").Error(),
					})
				}
			}

			if _, err := ret.WriteString(doc); err != nil {
				return ""
			}
		}
	default:
	}

	if expr, ok := location.Node.(promql.Expr); ok {
		_, err := ret.WriteString(fmt.Sprintf("\n\n__PromQL Type:__ %s\n\n", promql.DocumentedType(expr.Type())))
		if err != nil {
			return ""
		}
	}

	promURL := s.metadataService.GetURL()

	if promURL != "" && !s.headless {
		loc := *location

		loc.Node = loc.Query.Ast

		qText, err := location.Doc.GetSubstring(loc.Query.Pos+token.Pos(loc.Node.PositionRange().Start), loc.Query.Pos+token.Pos(loc.Node.PositionRange().End))
		if err != nil {
			return ""
		}

		qTextEncoded := url.QueryEscape(qText)

		target := fmt.Sprint(promURL, "/graph?g0.expr=", qTextEncoded)

		linkText := fmt.Sprintf("---\n[evaluate query](%s)\n\n", target)

		_, err = ret.WriteString(linkText)
		if err != nil {
			return ""
		}
	}

	return ret.String()
}

func funcDocStrings(name string) string {
	name = strings.ToLower(name)

	file, err := functionDocumentationFS.Open(fmt.Sprintf("/%s.md", name))

	if err != nil {
		return ""
	}

	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return ""
	}

	ret := make([]byte, stat.Size())

	_, err = file.Read(ret)
	if err != nil {
		return ""
	}

	return string(ret)
}

func (s *server) getMetricDocs(ctx context.Context, metric string) (string, error) {
	var ret strings.Builder
	ret.WriteString(fmt.Sprintf("### %s\n\n", metric))

	metadata, err := s.metadataService.MetricMetadata(ctx, metric)
	if err != nil {
		return "", err
	}

	if len(metadata.Help) > 0 {
		ret.WriteString(fmt.Sprintf("__Metric Help:__ %s\n\n", metadata.Help))
	}

	if len(metadata.Type) > 0 {
		ret.WriteString(fmt.Sprintf("__Metric Type:__  %s\n\n", metadata.Type))
	}

	if len(metadata.Unit) > 0 {
		ret.WriteString(fmt.Sprintf("__Metric Unit:__  %s\n\n", metadata.Unit))
	}

	return ret.String(), nil
}

func (s *server) getRecordingRuleDocs(doc *cache.DocumentHandle, metric string) (string, error) {
	var ret strings.Builder

	queries, err := doc.GetQueries()
	if err != nil {
		return "", err
	}

	var records []*cache.CompiledQuery

	for _, q := range queries {
		if q.Record == metric {
			records = append(records, q)
		}
	}

	if len(records) > 0 {
		fmt.Fprintf(&ret, "### %s\n\n", metric)
		fmt.Fprintf(&ret, "__Metric Type:__  %s\n\n", "Recording Rule")

		if len(records) == 1 {
			fmt.Fprintf(&ret, "__Underlying Metric:__  \n```\n%s\n```\n\n", records[0].Content)
		} else {
			fmt.Fprintf(&ret, "__Recording rule defined multiple times__\n\n")
		}
	}

	return ret.String(), nil
}
