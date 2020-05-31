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
	"bytes"
	"context"
	"errors"
	"go/token"
	"sync"

	"github.com/prometheus-community/promql-langserver/internal/vendored/go-tools/jsonrpc2"
	"github.com/prometheus-community/promql-langserver/internal/vendored/go-tools/lsp/protocol"
	"github.com/prometheus-community/promql-langserver/internal/vendored/go-tools/span"
)

// document caches content, metadata and compile results of a document.
type document struct {
	posData *token.File

	uri        protocol.DocumentURI
	languageID string
	version    float64
	content    string

	mu sync.RWMutex

	// A context.Context that expires when the document changes.
	versionCtx context.Context
	// The corresponding cancel function.
	obsoleteVersion context.CancelFunc

	// The queries found in the document.
	queries []*CompiledQuery
	// The YAML documents found in the document.
	yamls []*yamlDoc

	// The diagnostics created when parsing the document.
	diagnostics []protocol.Diagnostic

	// Wait for this before accessing the compile results or diagnostics.
	compilers waitGroup
}

// DocumentHandle bundles a Document together with a context.Context that expires
// when the document changes.
//
// All exported document access methods must be threadsafe and fail if the associated
// context has expired, unless otherwise documented.
type DocumentHandle struct {
	doc *document
	ctx context.Context
}

// ApplyIncrementalChanges applies given changes to a given document content.
// The context in the DocumentHandle is ignored.
func (d *DocumentHandle) ApplyIncrementalChanges(changes []protocol.TextDocumentContentChangeEvent, version float64) (string, error) {
	d.doc.mu.RLock()
	defer d.doc.mu.RUnlock()

	if version <= d.doc.version {
		return "", jsonrpc2.NewErrorf(jsonrpc2.CodeInvalidParams, "Update to file didn't increase version number")
	}

	content := []byte(d.doc.content)
	uri := d.doc.uri

	for _, change := range changes {
		// Update column mapper along with the content.
		converter := span.NewContentConverter(string(uri), content)
		m := &protocol.ColumnMapper{
			URI:       span.URI(d.doc.uri),
			Converter: converter,
			Content:   content,
		}

		spn, err := m.RangeSpan(*change.Range)

		if err != nil {
			return "", err
		}

		if !spn.HasOffset() {
			return "", jsonrpc2.NewErrorf(jsonrpc2.CodeInternalError, "invalid range for content change")
		}

		start, end := spn.Start().Offset(), spn.End().Offset()
		if end < start {
			return "", jsonrpc2.NewErrorf(jsonrpc2.CodeInternalError, "invalid range for content change")
		}

		var buf bytes.Buffer

		buf.Write(content[:start])
		buf.WriteString(change.Text)
		buf.Write(content[end:])

		content = buf.Bytes()
	}

	return string(content), nil
}

// SetContent sets the content of a document.
//
// This triggers async parsing of the document.
func (d *DocumentHandle) SetContent(serverLifetime context.Context, content string, version float64, new bool) error {
	d.doc.mu.Lock()
	defer d.doc.mu.Unlock()

	if !new && version <= d.doc.version {
		return jsonrpc2.NewErrorf(jsonrpc2.CodeInvalidParams, "Update to file didn't increase version number")
	}

	if len(content) > maxDocumentSize {
		return jsonrpc2.NewErrorf(jsonrpc2.CodeInternalError, "cache/SetContent: Provided.document to large.")
	}

	if !new {
		d.doc.obsoleteVersion()
	}

	d.doc.versionCtx, d.doc.obsoleteVersion = context.WithCancel(serverLifetime)

	d.doc.content = content
	d.doc.version = version

	// An additional newline is appended, to make sure the last line is indexed
	d.doc.posData.SetLinesForContent(append([]byte(content), '\n'))

	d.doc.queries = []*CompiledQuery{}
	d.doc.yamls = []*yamlDoc{}
	d.doc.diagnostics = []protocol.Diagnostic{}

	d.doc.compilers.Add(1)

	// We need to create a new document handler here since the old one
	// still carries the deprecated version context
	go (&DocumentHandle{d.doc, d.doc.versionCtx}).compile() //nolint:errcheck

	return nil
}

// GetContent returns the content of a document.
func (d *DocumentHandle) GetContent() (string, error) {
	d.doc.mu.RLock()
	defer d.doc.mu.RUnlock()

	select {
	case <-d.ctx.Done():
		return "", d.ctx.Err()
	default:
		return d.doc.content, nil
	}
}

// GetSubstring returns a substring of the content of a document.
//
// The parameters are the start and end of the substring, encoded
// as token.Pos.
func (d *DocumentHandle) GetSubstring(pos token.Pos, endPos token.Pos) (string, error) {
	d.doc.mu.RLock()
	defer d.doc.mu.RUnlock()

	select {
	case <-d.ctx.Done():
		return "", d.ctx.Err()
	default:
		content := d.doc.content
		base := d.doc.posData.Base()
		pos -= token.Pos(base)
		endPos -= token.Pos(base)

		if pos < 0 || pos > endPos || int(endPos) > len(content) {
			return "", errors.New("invalid range")
		}

		return content[pos:endPos], nil
	}
}

// GetQueries returns the compiled queries of a document.
//
// It blocks until all compile tasks are finished.
func (d *DocumentHandle) GetQueries() ([]*CompiledQuery, error) {
	d.doc.compilers.Wait()

	d.doc.mu.RLock()

	defer d.doc.mu.RUnlock()

	select {
	case <-d.ctx.Done():
		return nil, d.ctx.Err()
	default:
		return d.doc.queries, nil
	}
}

// getQuery returns a successfully compiled query at the given position, if there is one.
//
// Otherwise an error will be returned.
//
// It blocks until all compile tasks are finished.
func (d *DocumentHandle) getQuery(pos token.Pos) (*CompiledQuery, error) {
	queries, err := d.GetQueries()
	if err != nil {
		return nil, err
	}

	for _, query := range queries {
		if query.Ast != nil && query.Pos <= pos && query.Pos+token.Pos(query.Ast.PositionRange().End) >= pos {
			return query, nil
		}
	}

	return nil, errors.New("no query found at given position")
}

// GetVersion returns the version of a document.
func (d *DocumentHandle) GetVersion() (float64, error) {
	d.doc.mu.RLock()
	defer d.doc.mu.RUnlock()

	select {
	case <-d.ctx.Done():
		return 0, d.ctx.Err()
	default:
		return d.doc.version, nil
	}
}

// GetLanguageID returns the language ID of a document.
//
// Since the languageID never changes, it does not block or return errors.
func (d *DocumentHandle) GetLanguageID() string {
	return d.doc.languageID
}

// getYamlDocuments returns the yaml documents found in the document.
//
// It must be ensured by the caller, that these are already compiled.
func (d *DocumentHandle) getYamlDocuments() ([]*yamlDoc, error) {
	d.doc.mu.RLock()
	defer d.doc.mu.RUnlock()

	select {
	case <-d.ctx.Done():
		return nil, d.ctx.Err()
	default:
		return d.doc.yamls, nil
	}
}

// GetDiagnostics returns the diagnostics created during the compilation of a document.
//
// It blocks until all compile tasks are finished.
func (d *DocumentHandle) GetDiagnostics() ([]protocol.Diagnostic, error) {
	d.doc.compilers.Wait()

	d.doc.mu.RLock()
	defer d.doc.mu.RUnlock()

	select {
	case <-d.ctx.Done():
		return nil, d.ctx.Err()
	default:
		return d.doc.diagnostics, nil
	}
}
