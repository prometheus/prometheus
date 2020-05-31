// Copyright 2020 Tobias Guggenmos
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

	"github.com/prometheus-community/promql-langserver/internal/vendored/go-tools/lsp/protocol"
	promql "github.com/prometheus/prometheus/promql/parser"
)

// Definition is required by the protocol.Server interface.
func (s *server) Definition(ctx context.Context, params *protocol.DefinitionParams) ([]protocol.Location, error) {
	location, err := s.cache.Find(&params.TextDocumentPositionParams)
	if err != nil {
		return nil, nil
	}

	defs := []protocol.Location{}

	switch n := location.Node.(type) { // nolint: gocritic
	case *promql.VectorSelector:
		queries, err := location.Doc.GetQueries()
		if err != nil {
			break
		}

		for _, q := range queries {
			if q.Record == n.Name {
				def := protocol.Location{
					URI: params.TextDocument.URI,
				}

				defLocation := *location

				defLocation.Node = q.Ast
				defLocation.Query = q

				def.Range, err = getEditRange(&defLocation, "")
				if err != nil {
					break
				}

				defs = append(defs, def)
			}
		}
	}

	return defs, nil
}
