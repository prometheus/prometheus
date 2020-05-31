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

// This File includes code from the go/tools project which is governed by the following license:
// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package langserver

import (
	"context"

	"github.com/prometheus-community/promql-langserver/internal/vendored/go-tools/jsonrpc2"
	"github.com/prometheus-community/promql-langserver/internal/vendored/go-tools/lsp/protocol"
)

// DidOpen receives a call from the Client, telling that a files has been opened
// required by the protocol.Server interface.
func (s *server) DidOpen(ctx context.Context, params *protocol.DidOpenTextDocumentParams) error {
	_, err := s.cache.AddDocument(s.lifetime, &params.TextDocument)
	if err != nil {
		return err
	}

	if !s.headless {
		go s.diagnostics(params.TextDocument.URI)
	}

	return err
}

// DidClose receives a call from the Client, telling that a files has been closed
// required by the protocol.Server interface.
func (s *server) DidClose(_ context.Context, params *protocol.DidCloseTextDocumentParams) error {
	s.clearDiagnostics(s.lifetime, params.TextDocument.URI, 0)
	return s.cache.RemoveDocument(params.TextDocument.URI)
}

// DidChange receives a call from the Client, telling that a files has been changed
// required by the protocol.Server interface.
func (s *server) DidChange(ctx context.Context, params *protocol.DidChangeTextDocumentParams) error {
	//options := s.session.Options()
	if len(params.ContentChanges) < 1 {
		return jsonrpc2.NewErrorf(jsonrpc2.CodeInternalError, "no content changes provided")
	}

	uri := params.TextDocument.URI

	doc, err := s.cache.GetDocument(uri)
	if err != nil {
		return err
	}

	// Check if the client sent the full content of the file.
	// We accept a full content change even if the server expected incremental changes.
	text, isFullChange := fullChange(params.ContentChanges)

	if !isFullChange {
		// Determine the new file content.
		text, err = doc.ApplyIncrementalChanges(params.ContentChanges, params.TextDocument.Version)
		if err != nil {
			return err
		}
	}

	// Cache the new file content
	if err = doc.SetContent(s.lifetime, text, params.TextDocument.Version, false); err != nil {
		return err
	}

	if !s.headless {
		go s.diagnostics(uri)
	}

	return nil
}
func fullChange(changes []protocol.TextDocumentContentChangeEvent) (string, bool) {
	if len(changes) > 1 {
		return "", false
	}
	// The length of the changes must be 1 at this point.
	if changes[0].Range == nil && changes[0].RangeLength == 0 {
		return changes[0].Text, true
	}

	return "", false
}
