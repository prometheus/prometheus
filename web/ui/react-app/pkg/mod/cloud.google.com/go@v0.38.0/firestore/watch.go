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
	"errors"
	"fmt"
	"io"
	"log"
	"sort"
	"time"

	"cloud.google.com/go/internal/btree"
	"github.com/golang/protobuf/ptypes"
	gax "github.com/googleapis/gax-go/v2"
	pb "google.golang.org/genproto/googleapis/firestore/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// LogWatchStreams controls whether watch stream status changes are logged.
// This feature is EXPERIMENTAL and may disappear at any time.
var LogWatchStreams = false

// DocumentChangeKind describes the kind of change to a document between
// query snapshots.
type DocumentChangeKind int

const (
	// DocumentAdded indicates that the document was added for the first time.
	DocumentAdded DocumentChangeKind = iota
	// DocumentRemoved indicates that the document was removed.
	DocumentRemoved
	// DocumentModified indicates that the document was modified.
	DocumentModified
)

// A DocumentChange describes the change to a document from one query snapshot to the next.
type DocumentChange struct {
	Kind DocumentChangeKind
	Doc  *DocumentSnapshot
	// The zero-based index of the document in the sequence of query results prior to this change,
	// or -1 if the document was not present.
	OldIndex int
	// The zero-based index of the document in the sequence of query results after this change,
	// or -1 if the document is no longer present.
	NewIndex int
}

// Implementation of realtime updates (a.k.a. watch).
// This code is closely based on the Node.js implementation,
// https://github.com/googleapis/nodejs-firestore/blob/master/src/watch.js.

// The sole target ID for all streams from this client.
// Variable for testing.
var watchTargetID int32 = 'g' + 'o'

var defaultBackoff = gax.Backoff{
	// Values from https://github.com/googleapis/nodejs-firestore/blob/master/src/backoff.js.
	Initial:    1 * time.Second,
	Max:        60 * time.Second,
	Multiplier: 1.5,
}

// not goroutine-safe
type watchStream struct {
	ctx         context.Context
	c           *Client
	lc          pb.Firestore_ListenClient                 // the gRPC stream
	target      *pb.Target                                // document or query being watched
	backoff     gax.Backoff                               // for stream retries
	err         error                                     // sticky permanent error
	readTime    time.Time                                 // time of most recent snapshot
	current     bool                                      // saw CURRENT, but not RESET; precondition for a snapshot
	hasReturned bool                                      // have we returned a snapshot yet?
	compare     func(a, b *DocumentSnapshot) (int, error) // compare documents according to query

	// An ordered tree where DocumentSnapshots are the keys.
	docTree *btree.BTree
	// Map of document name to DocumentSnapshot for the last returned snapshot.
	docMap map[string]*DocumentSnapshot
	// Map of document name to DocumentSnapshot for accumulated changes for the current snapshot.
	// A nil value means the document was removed.
	changeMap map[string]*DocumentSnapshot
}

func newWatchStreamForDocument(ctx context.Context, dr *DocumentRef) *watchStream {
	// A single document is always equal to itself.
	compare := func(_, _ *DocumentSnapshot) (int, error) { return 0, nil }
	return newWatchStream(ctx, dr.Parent.c, compare, &pb.Target{
		TargetType: &pb.Target_Documents{
			Documents: &pb.Target_DocumentsTarget{Documents: []string{dr.Path}},
		},
		TargetId: watchTargetID,
	})
}

func newWatchStreamForQuery(ctx context.Context, q Query) (*watchStream, error) {
	qp, err := q.toProto()
	if err != nil {
		return nil, err
	}
	target := &pb.Target{
		TargetType: &pb.Target_Query{
			Query: &pb.Target_QueryTarget{
				Parent:    q.parentPath,
				QueryType: &pb.Target_QueryTarget_StructuredQuery{qp},
			},
		},
		TargetId: watchTargetID,
	}
	return newWatchStream(ctx, q.c, q.compareFunc(), target), nil
}

const btreeDegree = 4

func newWatchStream(ctx context.Context, c *Client, compare func(_, _ *DocumentSnapshot) (int, error), target *pb.Target) *watchStream {
	w := &watchStream{
		ctx:       ctx,
		c:         c,
		compare:   compare,
		target:    target,
		backoff:   defaultBackoff,
		docMap:    map[string]*DocumentSnapshot{},
		changeMap: map[string]*DocumentSnapshot{},
	}
	w.docTree = btree.New(btreeDegree, func(a, b interface{}) bool {
		return w.less(a.(*DocumentSnapshot), b.(*DocumentSnapshot))
	})
	return w
}

func (s *watchStream) less(a, b *DocumentSnapshot) bool {
	c, err := s.compare(a, b)
	if err != nil {
		s.err = err
		return false
	}
	return c < 0
}

// Once nextSnapshot returns an error, it will always return the same error.
func (s *watchStream) nextSnapshot() (*btree.BTree, []DocumentChange, time.Time, error) {
	if s.err != nil {
		return nil, nil, time.Time{}, s.err
	}
	var changes []DocumentChange
	for {
		// Process messages until we are in a consistent state.
		for !s.handleNextMessage() {
		}
		if s.err != nil {
			_ = s.close() // ignore error
			return nil, nil, time.Time{}, s.err
		}
		var newDocTree *btree.BTree
		newDocTree, changes = s.computeSnapshot(s.docTree, s.docMap, s.changeMap, s.readTime)
		if s.err != nil {
			return nil, nil, time.Time{}, s.err
		}
		// Only return a snapshot if something has changed, or this is the first snapshot.
		if !s.hasReturned || newDocTree != s.docTree {
			s.docTree = newDocTree
			break
		}
	}
	s.changeMap = map[string]*DocumentSnapshot{}
	s.hasReturned = true
	return s.docTree, changes, s.readTime, nil
}

// Read a message from the stream and handle it. Return true when
// we're in a consistent state, or there is a permanent error.
func (s *watchStream) handleNextMessage() bool {
	res, err := s.recv()
	if err != nil {
		s.err = err
		// Errors returned by recv are permanent.
		return true
	}
	switch r := res.ResponseType.(type) {
	case *pb.ListenResponse_TargetChange:
		return s.handleTargetChange(r.TargetChange)

	case *pb.ListenResponse_DocumentChange:
		name := r.DocumentChange.Document.Name
		s.logf("DocumentChange %q", name)
		if hasWatchTargetID(r.DocumentChange.TargetIds) { // document changed
			ref, err := pathToDoc(name, s.c)
			if err == nil {
				s.changeMap[name], err = newDocumentSnapshot(ref, r.DocumentChange.Document, s.c, nil)
			}
			if err != nil {
				s.err = err
				return true
			}
		} else if hasWatchTargetID(r.DocumentChange.RemovedTargetIds) { // document removed
			s.changeMap[name] = nil
		}

	case *pb.ListenResponse_DocumentDelete:
		s.logf("Delete %q", r.DocumentDelete.Document)
		s.changeMap[r.DocumentDelete.Document] = nil

	case *pb.ListenResponse_DocumentRemove:
		s.logf("Remove %q", r.DocumentRemove.Document)
		s.changeMap[r.DocumentRemove.Document] = nil

	case *pb.ListenResponse_Filter:
		s.logf("Filter %d", r.Filter.Count)
		if int(r.Filter.Count) != s.currentSize() {
			s.resetDocs() // Remove all the current results.
			// The filter didn't match; close the stream so it will be re-opened on the next
			// call to nextSnapshot.
			_ = s.close() // ignore error
			s.lc = nil
		}

	default:
		s.err = fmt.Errorf("unknown response type %T", r)
		return true
	}
	return false
}

// Return true iff in a consistent state, or there is a permanent error.
func (s *watchStream) handleTargetChange(tc *pb.TargetChange) bool {
	switch tc.TargetChangeType {
	case pb.TargetChange_NO_CHANGE:
		s.logf("TargetNoChange %d %v", len(tc.TargetIds), tc.ReadTime)
		if len(tc.TargetIds) == 0 && tc.ReadTime != nil && s.current {
			// Everything is up-to-date, so we are ready to return a snapshot.
			rt, err := ptypes.Timestamp(tc.ReadTime)
			if err != nil {
				s.err = err
				return true
			}
			s.readTime = rt
			s.target.ResumeType = &pb.Target_ResumeToken{tc.ResumeToken}
			return true
		}

	case pb.TargetChange_ADD:
		s.logf("TargetAdd")
		if tc.TargetIds[0] != watchTargetID {
			s.err = errors.New("unexpected target ID sent by server")
			return true
		}

	case pb.TargetChange_REMOVE:
		s.logf("TargetRemove")
		// We should never see a remove.
		if tc.Cause != nil {
			s.err = status.Error(codes.Code(tc.Cause.Code), tc.Cause.Message)
		} else {
			s.err = status.Error(codes.Internal, "firestore: client saw REMOVE")
		}
		return true

	// The targets reflect all changes committed before the targets were added
	// to the stream.
	case pb.TargetChange_CURRENT:
		s.logf("TargetCurrent")
		s.current = true

	// The targets have been reset, and a new initial state for the targets will be
	// returned in subsequent changes. Whatever changes have happened so far no
	// longer matter.
	case pb.TargetChange_RESET:
		s.logf("TargetReset")
		s.resetDocs()

	default:
		s.err = fmt.Errorf("firestore: unknown TargetChange type %s", tc.TargetChangeType)
		return true
	}
	// If we see a resume token and our watch ID is affected, we assume the stream
	// is now healthy, so we reset our backoff time to the minimum.
	if tc.ResumeToken != nil && (len(tc.TargetIds) == 0 || hasWatchTargetID(tc.TargetIds)) {
		s.backoff = defaultBackoff
	}
	return false // not in a consistent state, keep receiving
}

func (s *watchStream) resetDocs() {
	s.target.ResumeType = nil // clear resume token
	s.current = false
	s.changeMap = map[string]*DocumentSnapshot{}
	// Mark each document as deleted. If documents are not deleted, they
	// will be send again by the server.
	it := s.docTree.BeforeIndex(0)
	for it.Next() {
		s.changeMap[it.Key.(*DocumentSnapshot).Ref.Path] = nil
	}
}

func (s *watchStream) currentSize() int {
	_, adds, deletes := extractChanges(s.docMap, s.changeMap)
	return len(s.docMap) + len(adds) - len(deletes)
}

// Return the changes that have occurred since the last snapshot.
func extractChanges(docMap, changeMap map[string]*DocumentSnapshot) (updates, adds []*DocumentSnapshot, deletes []string) {
	for name, doc := range changeMap {
		switch {
		case doc == nil:
			if _, ok := docMap[name]; ok {
				deletes = append(deletes, name)
			}
		case docMap[name] != nil:
			updates = append(updates, doc)
		default:
			adds = append(adds, doc)
		}
	}
	return updates, adds, deletes
}

// For development only.
// TODO(jba): remove.
func assert(b bool) {
	if !b {
		panic("assertion failed")
	}
}

// Applies the mutations in changeMap to both the document tree and the
// document lookup map. Modifies docMap in place and returns a new docTree.
// If there were no changes, returns docTree unmodified.
func (s *watchStream) computeSnapshot(docTree *btree.BTree, docMap, changeMap map[string]*DocumentSnapshot, readTime time.Time) (*btree.BTree, []DocumentChange) {
	var changes []DocumentChange
	updatedTree := docTree
	assert(docTree.Len() == len(docMap))
	updates, adds, deletes := extractChanges(docMap, changeMap)
	if len(adds) > 0 || len(deletes) > 0 {
		updatedTree = docTree.Clone()
	}
	// Process the sorted changes in the order that is expected by our clients
	// (removals, additions, and then modifications). We also need to sort the
	// individual changes to assure that oldIndex/newIndex keep incrementing.
	deldocs := make([]*DocumentSnapshot, len(deletes))
	for i, d := range deletes {
		deldocs[i] = docMap[d]
	}
	sort.Sort(byLess{deldocs, s.less})
	for _, oldDoc := range deldocs {
		assert(oldDoc != nil)
		delete(docMap, oldDoc.Ref.Path)
		_, oldi := updatedTree.GetWithIndex(oldDoc)
		// TODO(jba): have btree.Delete return old index
		_, found := updatedTree.Delete(oldDoc)
		assert(found)
		changes = append(changes, DocumentChange{
			Kind:     DocumentRemoved,
			Doc:      oldDoc,
			OldIndex: oldi,
			NewIndex: -1,
		})
	}
	sort.Sort(byLess{adds, s.less})
	for _, newDoc := range adds {
		name := newDoc.Ref.Path
		assert(docMap[name] == nil)
		newDoc.ReadTime = readTime
		docMap[name] = newDoc
		updatedTree.Set(newDoc, nil)
		// TODO(jba): change btree so Set returns index as second value.
		_, newi := updatedTree.GetWithIndex(newDoc)
		changes = append(changes, DocumentChange{
			Kind:     DocumentAdded,
			Doc:      newDoc,
			OldIndex: -1,
			NewIndex: newi,
		})
	}
	sort.Sort(byLess{updates, s.less})
	for _, newDoc := range updates {
		name := newDoc.Ref.Path
		oldDoc := docMap[name]
		assert(oldDoc != nil)
		if newDoc.UpdateTime.Equal(oldDoc.UpdateTime) {
			continue
		}
		if updatedTree == docTree {
			updatedTree = docTree.Clone()
		}
		newDoc.ReadTime = readTime
		docMap[name] = newDoc
		_, oldi := updatedTree.GetWithIndex(oldDoc)
		updatedTree.Delete(oldDoc)
		updatedTree.Set(newDoc, nil)
		_, newi := updatedTree.GetWithIndex(newDoc)
		changes = append(changes, DocumentChange{
			Kind:     DocumentModified,
			Doc:      newDoc,
			OldIndex: oldi,
			NewIndex: newi,
		})
	}
	assert(updatedTree.Len() == len(docMap))
	return updatedTree, changes
}

type byLess struct {
	s    []*DocumentSnapshot
	less func(a, b *DocumentSnapshot) bool
}

func (b byLess) Len() int           { return len(b.s) }
func (b byLess) Swap(i, j int)      { b.s[i], b.s[j] = b.s[j], b.s[i] }
func (b byLess) Less(i, j int) bool { return b.less(b.s[i], b.s[j]) }

func hasWatchTargetID(ids []int32) bool {
	for _, id := range ids {
		if id == watchTargetID {
			return true
		}
	}
	return false
}

func (s *watchStream) logf(format string, args ...interface{}) {
	if LogWatchStreams {
		log.Printf(format, args...)
	}
}

// Close the stream. From this point on, calls to nextSnapshot will return
// io.EOF, or the error from CloseSend.
func (s *watchStream) stop() {
	err := s.close()
	if s.err != nil { // don't change existing error
		return
	}
	if err != nil {
		s.err = err
	}
	s.err = io.EOF // normal shutdown
}

func (s *watchStream) close() error {
	if s.lc == nil {
		return nil
	}
	return s.lc.CloseSend()
}

// recv receives the next message from the stream. It also handles opening the stream
// initially, and reopening it on non-permanent errors.
// recv doesn't have to be goroutine-safe.
func (s *watchStream) recv() (*pb.ListenResponse, error) {
	var err error
	for {
		if s.lc == nil {
			s.lc, err = s.open()
			if err != nil {
				// Do not retry if open fails.
				return nil, err
			}
		}
		res, err := s.lc.Recv()
		if err == nil || isPermanentWatchError(err) {
			return res, err
		}
		// Non-permanent error. Sleep and retry.
		s.changeMap = map[string]*DocumentSnapshot{} // clear changeMap
		dur := s.backoff.Pause()
		// If we're out of quota, wait a long time before retrying.
		if status.Code(err) == codes.ResourceExhausted {
			dur = s.backoff.Max
		}
		if err := sleep(s.ctx, dur); err != nil {
			return nil, err
		}
		s.lc = nil
	}
}

func (s *watchStream) open() (pb.Firestore_ListenClient, error) {
	dbPath := s.c.path()
	lc, err := s.c.c.Listen(withResourceHeader(s.ctx, dbPath))
	if err == nil {
		err = lc.Send(&pb.ListenRequest{
			Database:     dbPath,
			TargetChange: &pb.ListenRequest_AddTarget{AddTarget: s.target},
		})
	}
	if err != nil {
		return nil, err
	}
	return lc, nil
}

func isPermanentWatchError(err error) bool {
	if err == io.EOF {
		// Retry on normal end-of-stream.
		return false
	}
	switch status.Code(err) {
	case codes.Unknown, codes.DeadlineExceeded, codes.ResourceExhausted,
		codes.Internal, codes.Unavailable, codes.Unauthenticated:
		return false
	default:
		return true
	}
}
