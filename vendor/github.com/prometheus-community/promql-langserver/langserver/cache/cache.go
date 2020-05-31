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
	"context"
	"errors"
	"go/token"
	"sync"

	"github.com/prometheus-community/promql-langserver/internal/vendored/go-tools/jsonrpc2"
	"github.com/prometheus-community/promql-langserver/internal/vendored/go-tools/lsp/protocol"
)

// We need this so we can reserve a certain position range in the FileSet
// for each Document.
const maxDocumentSize = 1000000000

// DocumentCache caches the documents and compile Results associated with one server-client connection or one REST API instance.
//
// Before a cache instance can be used, Init must be called.
type DocumentCache struct {
	documents map[protocol.DocumentURI]*document
	mu        sync.RWMutex
}

// Init initializes a Document cache.
func (c *DocumentCache) Init() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.documents = make(map[protocol.DocumentURI]*document)
}

// AddDocument adds a document to the cache.
//
// This triggers async parsing of the document.
func (c *DocumentCache) AddDocument(serverLifetime context.Context, doc *protocol.TextDocumentItem) (*DocumentHandle, error) {
	if _, ok := c.documents[doc.URI]; ok {
		return nil, errors.New("document already exists")
	}

	fset := token.NewFileSet()

	file := fset.AddFile(string(doc.URI), -1, maxDocumentSize)

	if r := recover(); r != nil {
		if err, ok := r.(error); !ok {
			return nil, jsonrpc2.NewErrorf(jsonrpc2.CodeInternalError, "cache/addDocument: %v", err)
		}
	}

	file.SetLinesForContent([]byte(doc.Text))

	d := &document{
		posData:    file,
		uri:        doc.URI,
		languageID: doc.LanguageID,
	}

	d.compilers.initialize()

	err := (&DocumentHandle{d, context.Background()}).SetContent(serverLifetime, doc.Text, doc.Version, true)

	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.documents[doc.URI] = d

	return &DocumentHandle{d, d.versionCtx}, nil
}

// GetDocument retrieve a document from the cache.
func (c *DocumentCache) GetDocument(uri protocol.DocumentURI) (*DocumentHandle, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	ret, ok := c.documents[uri]

	if !ok {
		return nil, jsonrpc2.NewErrorf(jsonrpc2.CodeInternalError, "cache/getDocument: Document not found: %v", uri)
	}

	ret.mu.RLock()
	defer ret.mu.RUnlock()

	return &DocumentHandle{ret, ret.versionCtx}, nil
}

// RemoveDocument removes a document from the cache.
func (c *DocumentCache) RemoveDocument(uri protocol.DocumentURI) error {
	d, err := c.GetDocument(uri)
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	d.doc.obsoleteVersion()

	delete(c.documents, uri)

	return nil
}
