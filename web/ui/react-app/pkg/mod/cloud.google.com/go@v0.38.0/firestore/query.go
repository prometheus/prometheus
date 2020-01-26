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
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"time"

	"cloud.google.com/go/internal/btree"
	"github.com/golang/protobuf/ptypes/wrappers"
	"google.golang.org/api/iterator"
	pb "google.golang.org/genproto/googleapis/firestore/v1"
)

// Query represents a Firestore query.
//
// Query values are immutable. Each Query method creates
// a new Query; it does not modify the old.
type Query struct {
	c                      *Client
	path                   string // path to query (collection)
	parentPath             string // path of the collection's parent (document)
	collectionID           string
	selection              []FieldPath
	filters                []filter
	orders                 []order
	offset                 int32
	limit                  *wrappers.Int32Value
	startVals, endVals     []interface{}
	startDoc, endDoc       *DocumentSnapshot
	startBefore, endBefore bool
	err                    error
}

// DocumentID is the special field name representing the ID of a document
// in queries.
const DocumentID = "__name__"

// Select returns a new Query that specifies the paths
// to return from the result documents.
// Each path argument can be a single field or a dot-separated sequence of
// fields, and must not contain any of the runes "˜*/[]".
//
// An empty Select call will produce a query that returns only document IDs.
func (q Query) Select(paths ...string) Query {
	var fps []FieldPath
	for _, s := range paths {
		fp, err := parseDotSeparatedString(s)
		if err != nil {
			q.err = err
			return q
		}
		fps = append(fps, fp)
	}
	return q.SelectPaths(fps...)
}

// SelectPaths returns a new Query that specifies the field paths
// to return from the result documents.
//
// An empty SelectPaths call will produce a query that returns only document IDs.
func (q Query) SelectPaths(fieldPaths ...FieldPath) Query {
	if len(fieldPaths) == 0 {
		q.selection = []FieldPath{{DocumentID}}
	} else {
		q.selection = fieldPaths
	}
	return q
}

// Where returns a new Query that filters the set of results.
// A Query can have multiple filters.
// The path argument can be a single field or a dot-separated sequence of
// fields, and must not contain any of the runes "˜*/[]".
// The op argument must be one of "==", "<", "<=", ">" or ">=".
func (q Query) Where(path, op string, value interface{}) Query {
	fp, err := parseDotSeparatedString(path)
	if err != nil {
		q.err = err
		return q
	}
	q.filters = append(append([]filter(nil), q.filters...), filter{fp, op, value})
	return q
}

// WherePath returns a new Query that filters the set of results.
// A Query can have multiple filters.
// The op argument must be one of "==", "<", "<=", ">" or ">=".
func (q Query) WherePath(fp FieldPath, op string, value interface{}) Query {
	q.filters = append(append([]filter(nil), q.filters...), filter{fp, op, value})
	return q
}

// Direction is the sort direction for result ordering.
type Direction int32

const (
	// Asc sorts results from smallest to largest.
	Asc Direction = Direction(pb.StructuredQuery_ASCENDING)

	// Desc sorts results from largest to smallest.
	Desc Direction = Direction(pb.StructuredQuery_DESCENDING)
)

// OrderBy returns a new Query that specifies the order in which results are
// returned. A Query can have multiple OrderBy/OrderByPath specifications. OrderBy
// appends the specification to the list of existing ones.
//
// The path argument can be a single field or a dot-separated sequence of
// fields, and must not contain any of the runes "˜*/[]".
//
// To order by document name, use the special field path DocumentID.
func (q Query) OrderBy(path string, dir Direction) Query {
	fp, err := parseDotSeparatedString(path)
	if err != nil {
		q.err = err
		return q
	}
	q.orders = append(q.copyOrders(), order{fp, dir})
	return q
}

// OrderByPath returns a new Query that specifies the order in which results are
// returned. A Query can have multiple OrderBy/OrderByPath specifications.
// OrderByPath appends the specification to the list of existing ones.
func (q Query) OrderByPath(fp FieldPath, dir Direction) Query {
	q.orders = append(q.copyOrders(), order{fp, dir})
	return q
}

func (q *Query) copyOrders() []order {
	return append([]order(nil), q.orders...)
}

// Offset returns a new Query that specifies the number of initial results to skip.
// It must not be negative.
func (q Query) Offset(n int) Query {
	q.offset = trunc32(n)
	return q
}

// Limit returns a new Query that specifies the maximum number of results to return.
// It must not be negative.
func (q Query) Limit(n int) Query {
	q.limit = &wrappers.Int32Value{Value: trunc32(n)}
	return q
}

// StartAt returns a new Query that specifies that results should start at
// the document with the given field values.
//
// StartAt may be called with a single DocumentSnapshot, representing an
// existing document within the query. The document must be a direct child of
// the location being queried (not a parent document, or document in a
// different collection, or a grandchild document, for example).
//
// Otherwise, StartAt should be called with one field value for each OrderBy clause,
// in the order that they appear. For example, in
//   q.OrderBy("X", Asc).OrderBy("Y", Desc).StartAt(1, 2)
// results will begin at the first document where X = 1 and Y = 2.
//
// If an OrderBy call uses the special DocumentID field path, the corresponding value
// should be the document ID relative to the query's collection. For example, to
// start at the document "NewYork" in the "States" collection, write
//
//   client.Collection("States").OrderBy(DocumentID, firestore.Asc).StartAt("NewYork")
//
// Calling StartAt overrides a previous call to StartAt or StartAfter.
func (q Query) StartAt(docSnapshotOrFieldValues ...interface{}) Query {
	q.startBefore = true
	q.startVals, q.startDoc, q.err = q.processCursorArg("StartAt", docSnapshotOrFieldValues)
	return q
}

// StartAfter returns a new Query that specifies that results should start just after
// the document with the given field values. See Query.StartAt for more information.
//
// Calling StartAfter overrides a previous call to StartAt or StartAfter.
func (q Query) StartAfter(docSnapshotOrFieldValues ...interface{}) Query {
	q.startBefore = false
	q.startVals, q.startDoc, q.err = q.processCursorArg("StartAfter", docSnapshotOrFieldValues)
	return q
}

// EndAt returns a new Query that specifies that results should end at the
// document with the given field values. See Query.StartAt for more information.
//
// Calling EndAt overrides a previous call to EndAt or EndBefore.
func (q Query) EndAt(docSnapshotOrFieldValues ...interface{}) Query {
	q.endBefore = false
	q.endVals, q.endDoc, q.err = q.processCursorArg("EndAt", docSnapshotOrFieldValues)
	return q
}

// EndBefore returns a new Query that specifies that results should end just before
// the document with the given field values. See Query.StartAt for more information.
//
// Calling EndBefore overrides a previous call to EndAt or EndBefore.
func (q Query) EndBefore(docSnapshotOrFieldValues ...interface{}) Query {
	q.endBefore = true
	q.endVals, q.endDoc, q.err = q.processCursorArg("EndBefore", docSnapshotOrFieldValues)
	return q
}

func (q *Query) processCursorArg(name string, docSnapshotOrFieldValues []interface{}) ([]interface{}, *DocumentSnapshot, error) {
	for _, e := range docSnapshotOrFieldValues {
		if ds, ok := e.(*DocumentSnapshot); ok {
			if len(docSnapshotOrFieldValues) == 1 {
				return nil, ds, nil
			}
			return nil, nil, fmt.Errorf("firestore: a document snapshot must be the only argument to %s", name)
		}
	}
	return docSnapshotOrFieldValues, nil, nil
}

func (q Query) query() *Query { return &q }

func (q Query) toProto() (*pb.StructuredQuery, error) {
	if q.err != nil {
		return nil, q.err
	}
	if q.collectionID == "" {
		return nil, errors.New("firestore: query created without CollectionRef")
	}
	if q.startBefore {
		if len(q.startVals) == 0 && q.startDoc == nil {
			return nil, errors.New("firestore: StartAt/StartAfter must be called with at least one value")
		}
	}
	if q.endBefore {
		if len(q.endVals) == 0 && q.endDoc == nil {
			return nil, errors.New("firestore: EndAt/EndBefore must be called with at least one value")
		}
	}
	p := &pb.StructuredQuery{
		From:   []*pb.StructuredQuery_CollectionSelector{{CollectionId: q.collectionID}},
		Offset: q.offset,
		Limit:  q.limit,
	}
	if len(q.selection) > 0 {
		p.Select = &pb.StructuredQuery_Projection{}
		for _, fp := range q.selection {
			if err := fp.validate(); err != nil {
				return nil, err
			}
			p.Select.Fields = append(p.Select.Fields, fref(fp))
		}
	}
	// If there is only filter, use it directly. Otherwise, construct
	// a CompositeFilter.
	if len(q.filters) == 1 {
		pf, err := q.filters[0].toProto()
		if err != nil {
			return nil, err
		}
		p.Where = pf
	} else if len(q.filters) > 1 {
		cf := &pb.StructuredQuery_CompositeFilter{
			Op: pb.StructuredQuery_CompositeFilter_AND,
		}
		p.Where = &pb.StructuredQuery_Filter{
			FilterType: &pb.StructuredQuery_Filter_CompositeFilter{cf},
		}
		for _, f := range q.filters {
			pf, err := f.toProto()
			if err != nil {
				return nil, err
			}
			cf.Filters = append(cf.Filters, pf)
		}
	}
	orders := q.orders
	if q.startDoc != nil || q.endDoc != nil {
		orders = q.adjustOrders()
	}
	for _, ord := range orders {
		po, err := ord.toProto()
		if err != nil {
			return nil, err
		}
		p.OrderBy = append(p.OrderBy, po)
	}

	cursor, err := q.toCursor(q.startVals, q.startDoc, q.startBefore, orders)
	if err != nil {
		return nil, err
	}
	p.StartAt = cursor
	cursor, err = q.toCursor(q.endVals, q.endDoc, q.endBefore, orders)
	if err != nil {
		return nil, err
	}
	p.EndAt = cursor
	return p, nil
}

// If there is a start/end that uses a Document Snapshot, we may need to adjust the OrderBy
// clauses that the user provided: we add OrderBy(__name__) if it isn't already present, and
// we make sure we don't invalidate the original query by adding an OrderBy for inequality filters.
func (q *Query) adjustOrders() []order {
	// If the user is already ordering by document ID, don't change anything.
	for _, ord := range q.orders {
		if ord.isDocumentID() {
			return q.orders
		}
	}
	// If there are OrderBy clauses, append an OrderBy(DocumentID), using the direction of the last OrderBy clause.
	if len(q.orders) > 0 {
		return append(q.copyOrders(), order{
			fieldPath: FieldPath{DocumentID},
			dir:       q.orders[len(q.orders)-1].dir,
		})
	}
	// If there are no OrderBy clauses but there is an inequality, add an OrderBy clause
	// for the field of the first inequality.
	var orders []order
	for _, f := range q.filters {
		if f.op != "==" {
			orders = []order{{fieldPath: f.fieldPath, dir: Asc}}
			break
		}
	}
	// Add an ascending OrderBy(DocumentID).
	return append(orders, order{fieldPath: FieldPath{DocumentID}, dir: Asc})
}

func (q *Query) toCursor(fieldValues []interface{}, ds *DocumentSnapshot, before bool, orders []order) (*pb.Cursor, error) {
	var vals []*pb.Value
	var err error
	if ds != nil {
		vals, err = q.docSnapshotToCursorValues(ds, orders)
	} else if len(fieldValues) != 0 {
		vals, err = q.fieldValuesToCursorValues(fieldValues)
	} else {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &pb.Cursor{Values: vals, Before: before}, nil
}

// toPositionValues converts the field values to protos.
func (q *Query) fieldValuesToCursorValues(fieldValues []interface{}) ([]*pb.Value, error) {
	if len(fieldValues) != len(q.orders) {
		return nil, errors.New("firestore: number of field values in StartAt/StartAfter/EndAt/EndBefore does not match number of OrderBy fields")
	}
	vals := make([]*pb.Value, len(fieldValues))
	var err error
	for i, ord := range q.orders {
		fval := fieldValues[i]
		if ord.isDocumentID() {
			// TODO(jba): support DocumentRefs as well as strings.
			// TODO(jba): error if document ref does not belong to the right collection.
			docID, ok := fval.(string)
			if !ok {
				return nil, fmt.Errorf("firestore: expected doc ID for DocumentID field, got %T", fval)
			}
			vals[i] = &pb.Value{ValueType: &pb.Value_ReferenceValue{q.path + "/" + docID}}
		} else {
			var sawTransform bool
			vals[i], sawTransform, err = toProtoValue(reflect.ValueOf(fval))
			if err != nil {
				return nil, err
			}
			if sawTransform {
				return nil, errors.New("firestore: transforms disallowed in query value")
			}
		}
	}
	return vals, nil
}

func (q *Query) docSnapshotToCursorValues(ds *DocumentSnapshot, orders []order) ([]*pb.Value, error) {
	// TODO(jba): error if doc snap does not belong to the right collection.
	vals := make([]*pb.Value, len(orders))
	for i, ord := range orders {
		if ord.isDocumentID() {
			dp, qp := ds.Ref.Parent.Path, q.path
			if dp != qp {
				return nil, fmt.Errorf("firestore: document snapshot for %s passed to query on %s", dp, qp)
			}
			vals[i] = &pb.Value{ValueType: &pb.Value_ReferenceValue{ds.Ref.Path}}
		} else {
			val, err := valueAtPath(ord.fieldPath, ds.proto.Fields)
			if err != nil {
				return nil, err
			}
			vals[i] = val
		}
	}
	return vals, nil
}

// Returns a function that compares DocumentSnapshots according to q's ordering.
func (q Query) compareFunc() func(d1, d2 *DocumentSnapshot) (int, error) {
	// Add implicit sorting by name, using the last specified direction.
	lastDir := Asc
	if len(q.orders) > 0 {
		lastDir = q.orders[len(q.orders)-1].dir
	}
	orders := append(q.copyOrders(), order{[]string{DocumentID}, lastDir})
	return func(d1, d2 *DocumentSnapshot) (int, error) {
		for _, ord := range orders {
			var cmp int
			if len(ord.fieldPath) == 1 && ord.fieldPath[0] == DocumentID {
				cmp = compareReferences(d1.Ref.Path, d2.Ref.Path)
			} else {
				v1, err := valueAtPath(ord.fieldPath, d1.proto.Fields)
				if err != nil {
					return 0, err
				}
				v2, err := valueAtPath(ord.fieldPath, d2.proto.Fields)
				if err != nil {
					return 0, err
				}
				cmp = compareValues(v1, v2)
			}
			if cmp != 0 {
				if ord.dir == Desc {
					cmp = -cmp
				}
				return cmp, nil
			}
		}
		return 0, nil
	}
}

type filter struct {
	fieldPath FieldPath
	op        string
	value     interface{}
}

func (f filter) toProto() (*pb.StructuredQuery_Filter, error) {
	if err := f.fieldPath.validate(); err != nil {
		return nil, err
	}
	if uop, ok := unaryOpFor(f.value); ok {
		if f.op != "==" {
			return nil, fmt.Errorf("firestore: must use '==' when comparing %v", f.value)
		}
		return &pb.StructuredQuery_Filter{
			FilterType: &pb.StructuredQuery_Filter_UnaryFilter{
				UnaryFilter: &pb.StructuredQuery_UnaryFilter{
					OperandType: &pb.StructuredQuery_UnaryFilter_Field{
						Field: fref(f.fieldPath),
					},
					Op: uop,
				},
			},
		}, nil
	}
	var op pb.StructuredQuery_FieldFilter_Operator
	switch f.op {
	case "<":
		op = pb.StructuredQuery_FieldFilter_LESS_THAN
	case "<=":
		op = pb.StructuredQuery_FieldFilter_LESS_THAN_OR_EQUAL
	case ">":
		op = pb.StructuredQuery_FieldFilter_GREATER_THAN
	case ">=":
		op = pb.StructuredQuery_FieldFilter_GREATER_THAN_OR_EQUAL
	case "==":
		op = pb.StructuredQuery_FieldFilter_EQUAL
	case "array-contains":
		op = pb.StructuredQuery_FieldFilter_ARRAY_CONTAINS
	default:
		return nil, fmt.Errorf("firestore: invalid operator %q", f.op)
	}
	val, sawTransform, err := toProtoValue(reflect.ValueOf(f.value))
	if err != nil {
		return nil, err
	}
	if sawTransform {
		return nil, errors.New("firestore: transforms disallowed in query value")
	}
	return &pb.StructuredQuery_Filter{
		FilterType: &pb.StructuredQuery_Filter_FieldFilter{
			FieldFilter: &pb.StructuredQuery_FieldFilter{
				Field: fref(f.fieldPath),
				Op:    op,
				Value: val,
			},
		},
	}, nil
}

func unaryOpFor(value interface{}) (pb.StructuredQuery_UnaryFilter_Operator, bool) {
	switch {
	case value == nil:
		return pb.StructuredQuery_UnaryFilter_IS_NULL, true
	case isNaN(value):
		return pb.StructuredQuery_UnaryFilter_IS_NAN, true
	default:
		return pb.StructuredQuery_UnaryFilter_OPERATOR_UNSPECIFIED, false
	}
}

func isNaN(x interface{}) bool {
	switch x := x.(type) {
	case float32:
		return math.IsNaN(float64(x))
	case float64:
		return math.IsNaN(x)
	default:
		return false
	}
}

type order struct {
	fieldPath FieldPath
	dir       Direction
}

func (r order) isDocumentID() bool {
	return len(r.fieldPath) == 1 && r.fieldPath[0] == DocumentID
}

func (r order) toProto() (*pb.StructuredQuery_Order, error) {
	if err := r.fieldPath.validate(); err != nil {
		return nil, err
	}
	return &pb.StructuredQuery_Order{
		Field:     fref(r.fieldPath),
		Direction: pb.StructuredQuery_Direction(r.dir),
	}, nil
}

func fref(fp FieldPath) *pb.StructuredQuery_FieldReference {
	return &pb.StructuredQuery_FieldReference{FieldPath: fp.toServiceFieldPath()}
}

func trunc32(i int) int32 {
	if i > math.MaxInt32 {
		i = math.MaxInt32
	}
	return int32(i)
}

// Documents returns an iterator over the query's resulting documents.
func (q Query) Documents(ctx context.Context) *DocumentIterator {
	return &DocumentIterator{
		iter: newQueryDocumentIterator(withResourceHeader(ctx, q.c.path()), &q, nil),
	}
}

// DocumentIterator is an iterator over documents returned by a query.
type DocumentIterator struct {
	iter docIterator
	err  error
}

// Unexported interface so we can have two different kinds of DocumentIterator: one
// for straight queries, and one for query snapshots. We do it this way instead of
// making DocumentIterator an interface because in the client libraries, iterators are
// always concrete types, and the fact that this one has two different implementations
// is an internal detail.
type docIterator interface {
	next() (*DocumentSnapshot, error)
	stop()
}

// Next returns the next result. Its second return value is iterator.Done if there
// are no more results. Once Next returns Done, all subsequent calls will return
// Done.
func (it *DocumentIterator) Next() (*DocumentSnapshot, error) {
	if it.err != nil {
		return nil, it.err
	}
	ds, err := it.iter.next()
	if err != nil {
		it.err = err
	}
	return ds, err
}

// Stop stops the iterator, freeing its resources.
// Always call Stop when you are done with a DocumentIterator.
// It is not safe to call Stop concurrently with Next.
func (it *DocumentIterator) Stop() {
	if it.iter != nil { // possible in error cases
		it.iter.stop()
	}
	if it.err == nil {
		it.err = iterator.Done
	}
}

// GetAll returns all the documents remaining from the iterator.
// It is not necessary to call Stop on the iterator after calling GetAll.
func (it *DocumentIterator) GetAll() ([]*DocumentSnapshot, error) {
	defer it.Stop()
	var docs []*DocumentSnapshot
	for {
		doc, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		docs = append(docs, doc)
	}
	return docs, nil
}

type queryDocumentIterator struct {
	ctx          context.Context
	cancel       func()
	q            *Query
	tid          []byte // transaction ID, if any
	streamClient pb.Firestore_RunQueryClient
}

func newQueryDocumentIterator(ctx context.Context, q *Query, tid []byte) *queryDocumentIterator {
	ctx, cancel := context.WithCancel(ctx)
	return &queryDocumentIterator{
		ctx:    ctx,
		cancel: cancel,
		q:      q,
		tid:    tid,
	}
}

func (it *queryDocumentIterator) next() (*DocumentSnapshot, error) {
	client := it.q.c
	if it.streamClient == nil {
		sq, err := it.q.toProto()
		if err != nil {
			return nil, err
		}
		req := &pb.RunQueryRequest{
			Parent:    it.q.parentPath,
			QueryType: &pb.RunQueryRequest_StructuredQuery{sq},
		}
		if it.tid != nil {
			req.ConsistencySelector = &pb.RunQueryRequest_Transaction{it.tid}
		}
		it.streamClient, err = client.c.RunQuery(it.ctx, req)
		if err != nil {
			return nil, err
		}
	}
	var res *pb.RunQueryResponse
	var err error
	for {
		res, err = it.streamClient.Recv()
		if err == io.EOF {
			return nil, iterator.Done
		}
		if err != nil {
			return nil, err
		}
		if res.Document != nil {
			break
		}
		// No document => partial progress; keep receiving.
	}
	docRef, err := pathToDoc(res.Document.Name, client)
	if err != nil {
		return nil, err
	}
	doc, err := newDocumentSnapshot(docRef, res.Document, client, res.ReadTime)
	if err != nil {
		return nil, err
	}
	return doc, nil
}

func (it *queryDocumentIterator) stop() {
	it.cancel()
}

// Snapshots returns an iterator over snapshots of the query. Each time the query
// results change, a new snapshot will be generated.
func (q Query) Snapshots(ctx context.Context) *QuerySnapshotIterator {
	ws, err := newWatchStreamForQuery(ctx, q)
	if err != nil {
		return &QuerySnapshotIterator{err: err}
	}
	return &QuerySnapshotIterator{
		Query: q,
		ws:    ws,
	}
}

// QuerySnapshotIterator is an iterator over snapshots of a query.
// Call Next on the iterator to get a snapshot of the query's results each time they change.
// Call Stop on the iterator when done.
//
// For an example, see Query.Snapshots.
type QuerySnapshotIterator struct {
	// The Query used to construct this iterator.
	Query Query

	ws  *watchStream
	err error
}

// Next blocks until the query's results change, then returns a QuerySnapshot for
// the current results.
//
// Next never returns iterator.Done unless it is called after Stop.
func (it *QuerySnapshotIterator) Next() (*QuerySnapshot, error) {
	if it.err != nil {
		return nil, it.err
	}
	btree, changes, readTime, err := it.ws.nextSnapshot()
	if err != nil {
		if err == io.EOF {
			err = iterator.Done
		}
		it.err = err
		return nil, it.err
	}
	return &QuerySnapshot{
		Documents: &DocumentIterator{
			iter: (*btreeDocumentIterator)(btree.BeforeIndex(0)),
		},
		Size:     btree.Len(),
		Changes:  changes,
		ReadTime: readTime,
	}, nil
}

// Stop stops receiving snapshots. You should always call Stop when you are done with
// a QuerySnapshotIterator, to free up resources. It is not safe to call Stop
// concurrently with Next.
func (it *QuerySnapshotIterator) Stop() {
	if it.ws != nil {
		it.ws.stop()
	}
}

// A QuerySnapshot is a snapshot of query results. It is returned by
// QuerySnapshotIterator.Next whenever the results of a query change.
type QuerySnapshot struct {
	// An iterator over the query results.
	// It is not necessary to call Stop on this iterator.
	Documents *DocumentIterator

	// The number of results in this snapshot.
	Size int

	// The changes since the previous snapshot.
	Changes []DocumentChange

	// The time at which this snapshot was obtained from Firestore.
	ReadTime time.Time
}

type btreeDocumentIterator btree.Iterator

func (it *btreeDocumentIterator) next() (*DocumentSnapshot, error) {
	if !(*btree.Iterator)(it).Next() {
		return nil, iterator.Done
	}
	return it.Key.(*DocumentSnapshot), nil
}

func (*btreeDocumentIterator) stop() {}
