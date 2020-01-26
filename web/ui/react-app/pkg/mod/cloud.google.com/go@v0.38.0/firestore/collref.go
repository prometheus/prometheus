// Copyright 2017 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package firestore

import (
	"context"
	"math/rand"
	"os"
	"sync"
	"time"
)

// A CollectionRef is a reference to Firestore collection.
type CollectionRef struct {
	c *Client

	// The full resource path of the collection's parent. Typically Parent.Path,
	// or c.path if Parent is nil. May be different if this CollectionRef was
	// created from a stored reference to a different project/DB. Always
	// includes /documents - that is, the parent is minimally considered to be
	// "<db>/documents".
	//
	// For example, "projects/P/databases/D/documents/coll-1/doc-1".
	parentPath string

	// The shorter resource path of the collection. A collection "coll-2" in
	// document "doc-1" in collection "coll-1" would be: "coll-1/doc-1/coll-2".
	selfPath string

	// Parent is the document of which this collection is a part. It is
	// nil for top-level collections.
	Parent *DocumentRef

	// The full resource path of the collection: "projects/P/databases/D/documents..."
	Path string

	// ID is the collection identifier.
	ID string

	// Use the methods of Query on a CollectionRef to create and run queries.
	Query
}

func newTopLevelCollRef(c *Client, dbPath, id string) *CollectionRef {
	return &CollectionRef{
		c:          c,
		ID:         id,
		parentPath: dbPath + "/documents",
		selfPath:   id,
		Path:       dbPath + "/documents/" + id,
		Query: Query{
			c:            c,
			collectionID: id,
			path:         dbPath + "/documents/" + id,
			parentPath:   dbPath + "/documents",
		},
	}
}

func newCollRefWithParent(c *Client, parent *DocumentRef, id string) *CollectionRef {
	selfPath := parent.shortPath + "/" + id
	return &CollectionRef{
		c:          c,
		Parent:     parent,
		ID:         id,
		parentPath: parent.Path,
		selfPath:   selfPath,
		Path:       parent.Path + "/" + id,
		Query: Query{
			c:            c,
			collectionID: id,
			path:         parent.Path + "/" + id,
			parentPath:   parent.Path,
		},
	}
}

// Doc returns a DocumentRef that refers to the document in the collection with the
// given identifier.
func (c *CollectionRef) Doc(id string) *DocumentRef {
	if c == nil {
		return nil
	}
	return newDocRef(c, id)
}

// NewDoc returns a DocumentRef with a uniquely generated ID.
func (c *CollectionRef) NewDoc() *DocumentRef {
	return c.Doc(uniqueID())
}

// Add generates a DocumentRef with a unique ID. It then creates the document
// with the given data, which can be a map[string]interface{}, a struct or a
// pointer to a struct.
//
// Add returns an error in the unlikely event that a document with the same ID
// already exists.
func (c *CollectionRef) Add(ctx context.Context, data interface{}) (*DocumentRef, *WriteResult, error) {
	d := c.NewDoc()
	wr, err := d.Create(ctx, data)
	if err != nil {
		return nil, nil, err
	}
	return d, wr, nil
}

// DocumentRefs returns references to all the documents in the collection, including
// missing documents. A missing document is a document that does not exist but has
// sub-documents.
func (c *CollectionRef) DocumentRefs(ctx context.Context) *DocumentRefIterator {
	return newDocumentRefIterator(ctx, c, nil)
}

const alphanum = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

var (
	rngMu sync.Mutex
	rng   = rand.New(rand.NewSource(time.Now().UnixNano() ^ int64(os.Getpid())))
)

func uniqueID() string {
	var b [20]byte
	rngMu.Lock()
	for i := 0; i < len(b); i++ {
		b[i] = alphanum[rng.Intn(len(alphanum))]
	}
	rngMu.Unlock()
	return string(b[:])
}
