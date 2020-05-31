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

package cache

import (
	"go/token"

	"github.com/prometheus-community/promql-langserver/internal/vendored/go-tools/lsp/protocol"
	promql "github.com/prometheus/prometheus/promql/parser"
)

// promQLErrToProtocolDiagnostic converts a promql.ParseErr to a protocol.Diagnostic
//
// The position of the query must be passed as the first argument.
func (d *DocumentHandle) promQLErrToProtocolDiagnostic(queryPos token.Pos, promQLErr *promql.ParseErr) (*protocol.Diagnostic, error) {
	start, err := d.PosToProtocolPosition(
		queryPos + token.Pos(promQLErr.PositionRange.Start))
	if err != nil {
		return nil, err
	}

	end, err := d.PosToProtocolPosition(
		queryPos + token.Pos(promQLErr.PositionRange.End))
	if err != nil {
		return nil, err
	}

	message := &protocol.Diagnostic{
		Range: protocol.Range{
			Start: start,
			End:   end,
		},
		Severity: 1, // Error
		Source:   "promql-lsp",
		Message:  promQLErr.Err.Error(),
	}

	return message, nil
}

// warnQuotedYaml adds a warnign about a quoted PromQL expression to the diagnostics
// of a document.
func (d *DocumentHandle) warnQuotedYaml(start token.Pos, end token.Pos) error {
	var startPosition token.Position

	var endPosition token.Position

	var err error

	startPosition, err = d.tokenPosToTokenPosition(start)
	if err != nil {
		return err
	}

	endPosition, err = d.tokenPosToTokenPosition(end)
	if err != nil {
		return err
	}

	message := &protocol.Diagnostic{
		Severity: 2, // Warning
		Source:   "promql-lsp",
		Message:  "Quoted queries are not supported by the language server",
	}

	if message.Range.Start, err = d.tokenPositionToProtocolPosition(startPosition); err != nil {
		return err
	}

	if message.Range.End, err = d.tokenPositionToProtocolPosition(endPosition); err != nil {
		return err
	}

	return d.addDiagnostic(message)
}

// addDiagnostic adds a protocol.Diagnostic to the diagnostics of a Document.
//
//If the context is expired the diagnostic is discarded.
func (d *DocumentHandle) addDiagnostic(diagnostic *protocol.Diagnostic) error {
	d.doc.mu.Lock()
	defer d.doc.mu.Unlock()

	select {
	case <-d.ctx.Done():
		return d.ctx.Err()
	default:
		d.doc.diagnostics = append(d.doc.diagnostics, *diagnostic)
		return nil
	}
}
