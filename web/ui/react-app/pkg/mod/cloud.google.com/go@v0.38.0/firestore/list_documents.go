// Copyright 2018 Google LLC
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

	vkit "cloud.google.com/go/firestore/apiv1"
	"google.golang.org/api/iterator"
	pb "google.golang.org/genproto/googleapis/firestore/v1"
)

// DocumentRefIterator is an interator over DocumentRefs.
type DocumentRefIterator struct {
	client   *Client
	it       *vkit.DocumentIterator
	pageInfo *iterator.PageInfo
	nextFunc func() error
	items    []*DocumentRef
	err      error
}

func newDocumentRefIterator(ctx context.Context, cr *CollectionRef, tid []byte) *DocumentRefIterator {
	client := cr.c
	req := &pb.ListDocumentsRequest{
		Parent:       cr.parentPath,
		CollectionId: cr.ID,
		ShowMissing:  true,
		Mask:         &pb.DocumentMask{}, // empty mask: we want only the ref
	}
	if tid != nil {
		req.ConsistencySelector = &pb.ListDocumentsRequest_Transaction{tid}
	}
	it := &DocumentRefIterator{
		client: client,
		it:     client.c.ListDocuments(withResourceHeader(ctx, client.path()), req),
	}
	it.pageInfo, it.nextFunc = iterator.NewPageInfo(
		it.fetch,
		func() int { return len(it.items) },
		func() interface{} { b := it.items; it.items = nil; return b })
	return it
}

// PageInfo supports pagination. See the google.golang.org/api/iterator package for details.
func (it *DocumentRefIterator) PageInfo() *iterator.PageInfo { return it.pageInfo }

// Next returns the next result. Its second return value is iterator.Done if there
// are no more results. Once Next returns Done, all subsequent calls will return
// Done.
func (it *DocumentRefIterator) Next() (*DocumentRef, error) {
	if err := it.nextFunc(); err != nil {
		return nil, err
	}
	item := it.items[0]
	it.items = it.items[1:]
	return item, nil
}

func (it *DocumentRefIterator) fetch(pageSize int, pageToken string) (string, error) {
	if it.err != nil {
		return "", it.err
	}
	return iterFetch(pageSize, pageToken, it.it.PageInfo(), func() error {
		docProto, err := it.it.Next()
		if err != nil {
			return err
		}
		docRef, err := pathToDoc(docProto.Name, it.client)
		if err != nil {
			return err
		}
		it.items = append(it.items, docRef)
		return nil
	})
}

// GetAll returns all the DocumentRefs remaining from the iterator.
func (it *DocumentRefIterator) GetAll() ([]*DocumentRef, error) {
	var drs []*DocumentRef
	for {
		dr, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		drs = append(drs, dr)
	}
	return drs, nil
}
