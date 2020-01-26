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
	"reflect"
	"sort"

	vkit "cloud.google.com/go/firestore/apiv1"
	"cloud.google.com/go/internal/trace"
	"google.golang.org/api/iterator"
	pb "google.golang.org/genproto/googleapis/firestore/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var errNilDocRef = errors.New("firestore: nil DocumentRef")

// A DocumentRef is a reference to a Firestore document.
type DocumentRef struct {
	// The CollectionRef that this document is a part of. Never nil.
	Parent *CollectionRef

	// The full resource path of the document. A document "doc-1" in collection
	// "coll-1" would be: "projects/P/databases/D/documents/coll-1/doc-1".
	Path string

	// The shorter resource path of the document. A document "doc-1" in
	// collection "coll-1" would be: "coll-1/doc-1".
	shortPath string

	// The ID of the document: the last component of the resource path.
	ID string
}

func newDocRef(parent *CollectionRef, id string) *DocumentRef {
	return &DocumentRef{
		Parent:    parent,
		ID:        id,
		Path:      parent.Path + "/" + id,
		shortPath: parent.selfPath + "/" + id,
	}
}

// Collection returns a reference to sub-collection of this document.
func (d *DocumentRef) Collection(id string) *CollectionRef {
	return newCollRefWithParent(d.Parent.c, d, id)
}

// Get retrieves the document. If the document does not exist, Get return a NotFound error, which
// can be checked with
//    grpc.Code(err) == codes.NotFound
// In that case, Get returns a non-nil DocumentSnapshot whose Exists method return false and whose
// ReadTime is the time of the failed read operation.
func (d *DocumentRef) Get(ctx context.Context) (_ *DocumentSnapshot, err error) {
	ctx = trace.StartSpan(ctx, "cloud.google.com/go/firestore.DocumentRef.Get")
	defer func() { trace.EndSpan(ctx, err) }()

	if d == nil {
		return nil, errNilDocRef
	}
	docsnaps, err := d.Parent.c.getAll(ctx, []*DocumentRef{d}, nil)
	if err != nil {
		return nil, err
	}
	ds := docsnaps[0]
	if !ds.Exists() {
		return ds, status.Errorf(codes.NotFound, "%q not found", d.Path)
	}
	return ds, nil
}

// Create creates the document with the given data.
// It returns an error if a document with the same ID already exists.
//
// The data argument can be a map with string keys, a struct, or a pointer to a
// struct. The map keys or exported struct fields become the fields of the firestore
// document.
// The values of data are converted to Firestore values as follows:
//
//   - bool converts to Bool.
//   - string converts to String.
//   - int, int8, int16, int32 and int64 convert to Integer.
//   - uint8, uint16 and uint32 convert to Integer. uint, uint64 and uintptr are disallowed,
//     because they may be able to represent values that cannot be represented in an int64,
//     which is the underlying type of a Integer.
//   - float32 and float64 convert to Double.
//   - []byte converts to Bytes.
//   - time.Time and *ts.Timestamp convert to Timestamp. ts is the package
//     "github.com/golang/protobuf/ptypes/timestamp".
//   - *latlng.LatLng converts to GeoPoint. latlng is the package
//     "google.golang.org/genproto/googleapis/type/latlng". You should always use
//     a pointer to a LatLng.
//   - Slices convert to Array.
//   - *firestore.DocumentRef converts to Reference.
//   - Maps and structs convert to Map.
//   - nils of any type convert to Null.
//
// Pointers and interface{} are also permitted, and their elements processed
// recursively.
//
// Struct fields can have tags like those used by the encoding/json package. Tags
// begin with "firestore:" and are followed by "-", meaning "ignore this field," or
// an alternative name for the field. Following the name, these comma-separated
// options may be provided:
//
//   - omitempty: Do not encode this field if it is empty. A value is empty
//     if it is a zero value, or an array, slice or map of length zero.
//   - serverTimestamp: The field must be of type time.Time. When writing, if
//     the field has the zero value, the server will populate the stored document with
//     the time that the request is processed.
func (d *DocumentRef) Create(ctx context.Context, data interface{}) (_ *WriteResult, err error) {
	ctx = trace.StartSpan(ctx, "cloud.google.com/go/firestore.DocumentRef.Create")
	defer func() { trace.EndSpan(ctx, err) }()

	ws, err := d.newCreateWrites(data)
	if err != nil {
		return nil, err
	}
	return d.Parent.c.commit1(ctx, ws)
}

func (d *DocumentRef) newCreateWrites(data interface{}) ([]*pb.Write, error) {
	if d == nil {
		return nil, errNilDocRef
	}
	doc, transforms, err := toProtoDocument(data)
	if err != nil {
		return nil, err
	}
	doc.Name = d.Path
	pc, err := exists(false).preconditionProto()
	if err != nil {
		return nil, err
	}
	return d.newUpdateWithTransform(doc, nil, pc, transforms, false), nil
}

// Set creates or overwrites the document with the given data. See DocumentRef.Create
// for the acceptable values of data. Without options, Set overwrites the document
// completely. Specify one of the Merge options to preserve an existing document's
// fields. To delete some fields, use a Merge option with firestore.Delete as the
// field value.
func (d *DocumentRef) Set(ctx context.Context, data interface{}, opts ...SetOption) (_ *WriteResult, err error) {
	ctx = trace.StartSpan(ctx, "cloud.google.com/go/firestore.DocumentRef.Set")
	defer func() { trace.EndSpan(ctx, err) }()

	ws, err := d.newSetWrites(data, opts)
	if err != nil {
		return nil, err
	}
	return d.Parent.c.commit1(ctx, ws)
}

func (d *DocumentRef) newSetWrites(data interface{}, opts []SetOption) ([]*pb.Write, error) {
	if d == nil {
		return nil, errNilDocRef
	}
	if data == nil {
		return nil, errors.New("firestore: nil document contents")
	}
	if len(opts) == 0 { // Set without merge
		doc, serverTimestampPaths, err := toProtoDocument(data)
		if err != nil {
			return nil, err
		}
		doc.Name = d.Path
		return d.newUpdateWithTransform(doc, nil, nil, serverTimestampPaths, true), nil
	}
	// Set with merge.
	// This is just like Update, except for the existence precondition.
	// So we turn data into a list of (FieldPath, interface{}) pairs (fpv's), as we do
	// for Update.
	fieldPaths, allPaths, err := processSetOptions(opts)
	if err != nil {
		return nil, err
	}
	var fpvs []fpv
	v := reflect.ValueOf(data)
	if allPaths {
		// Set with MergeAll. Collect all the leaves of the map.
		if v.Kind() != reflect.Map {
			return nil, errors.New("firestore: MergeAll can only be specified with map data")
		}
		if v.Len() == 0 {
			// Special case: MergeAll with an empty map.
			return d.newUpdateWithTransform(&pb.Document{Name: d.Path}, []FieldPath{}, nil, nil, true), nil
		}
		fpvsFromData(v, nil, &fpvs)
	} else {
		// Set with merge paths.  Collect only the values at the given paths.
		for _, fp := range fieldPaths {
			val, err := getAtPath(v, fp)
			if err != nil {
				return nil, err
			}
			fpvs = append(fpvs, fpv{fp, val})
		}
	}
	return d.fpvsToWrites(fpvs, nil)
}

// fpvsFromData converts v into a list of (FieldPath, value) pairs.
func fpvsFromData(v reflect.Value, prefix FieldPath, fpvs *[]fpv) {
	switch v.Kind() {
	case reflect.Map:
		for _, k := range v.MapKeys() {
			fpvsFromData(v.MapIndex(k), prefix.with(k.String()), fpvs)
		}
	case reflect.Interface:
		fpvsFromData(v.Elem(), prefix, fpvs)

	default:
		var val interface{}
		if v.IsValid() {
			val = v.Interface()
		}
		*fpvs = append(*fpvs, fpv{prefix, val})
	}
}

// Delete deletes the document. If the document doesn't exist, it does nothing
// and returns no error.
func (d *DocumentRef) Delete(ctx context.Context, preconds ...Precondition) (_ *WriteResult, err error) {
	ctx = trace.StartSpan(ctx, "cloud.google.com/go/firestore.DocumentRef.Delete")
	defer func() { trace.EndSpan(ctx, err) }()

	ws, err := d.newDeleteWrites(preconds)
	if err != nil {
		return nil, err
	}
	return d.Parent.c.commit1(ctx, ws)
}

func (d *DocumentRef) newDeleteWrites(preconds []Precondition) ([]*pb.Write, error) {
	if d == nil {
		return nil, errNilDocRef
	}
	pc, err := processPreconditionsForDelete(preconds)
	if err != nil {
		return nil, err
	}
	return []*pb.Write{{
		Operation:       &pb.Write_Delete{d.Path},
		CurrentDocument: pc,
	}}, nil
}

func (d *DocumentRef) newUpdatePathWrites(updates []Update, preconds []Precondition) ([]*pb.Write, error) {
	if len(updates) == 0 {
		return nil, errors.New("firestore: no paths to update")
	}
	var fpvs []fpv
	for _, u := range updates {
		v, err := u.process()
		if err != nil {
			return nil, err
		}
		fpvs = append(fpvs, v)
	}
	pc, err := processPreconditionsForUpdate(preconds)
	if err != nil {
		return nil, err
	}
	return d.fpvsToWrites(fpvs, pc)
}

func (d *DocumentRef) fpvsToWrites(fpvs []fpv, pc *pb.Precondition) ([]*pb.Write, error) {
	// Make sure there are no duplications or prefixes among the field paths.
	var fps []FieldPath
	for _, fpv := range fpvs {
		fps = append(fps, fpv.fieldPath)
	}
	if err := checkNoDupOrPrefix(fps); err != nil {
		return nil, err
	}

	// Process each fpv.
	var updatePaths []FieldPath
	var transforms []*pb.DocumentTransform_FieldTransform
	doc := &pb.Document{
		Name:   d.Path,
		Fields: map[string]*pb.Value{},
	}
	for _, fpv := range fpvs {
		switch fpv.value.(type) {
		case arrayUnion:
			au := fpv.value.(arrayUnion)
			t, err := arrayUnionTransform(au, fpv.fieldPath)
			if err != nil {
				return nil, err
			}
			transforms = append(transforms, t)
		case arrayRemove:
			ar := fpv.value.(arrayRemove)
			t, err := arrayRemoveTransform(ar, fpv.fieldPath)
			if err != nil {
				return nil, err
			}
			transforms = append(transforms, t)
		default:
			switch fpv.value {
			case Delete:
				// Send the field path without a corresponding value.
				updatePaths = append(updatePaths, fpv.fieldPath)

			case ServerTimestamp:
				// Use the path in a transform operation.
				transforms = append(transforms, serverTimestamp(fpv.fieldPath.toServiceFieldPath()))

			default:
				updatePaths = append(updatePaths, fpv.fieldPath)
				// Convert the value to a proto and put it into the document.
				v := reflect.ValueOf(fpv.value)

				pv, _, err := toProtoValue(v)
				if err != nil {
					return nil, err
				}
				setAtPath(doc.Fields, fpv.fieldPath, pv)
				// Also accumulate any transforms within the value.
				ts, err := extractTransforms(v, fpv.fieldPath)
				if err != nil {
					return nil, err
				}
				transforms = append(transforms, ts...)
			}
		}
	}
	return d.newUpdateWithTransform(doc, updatePaths, pc, transforms, false), nil
}

// newUpdateWithTransform constructs operations for a commit. Most generally, it
// returns an update operation followed by a transform.
//
// If there are no serverTimestampPaths, the transform is omitted.
//
// If doc.Fields is empty, there are no updatePaths, and there is no precondition,
// the update is omitted, unless updateOnEmpty is true.
func (d *DocumentRef) newUpdateWithTransform(doc *pb.Document, updatePaths []FieldPath, pc *pb.Precondition, transforms []*pb.DocumentTransform_FieldTransform, updateOnEmpty bool) []*pb.Write {
	var ws []*pb.Write
	if updateOnEmpty || len(doc.Fields) > 0 ||
		len(updatePaths) > 0 || (pc != nil && len(transforms) == 0) {
		var mask *pb.DocumentMask
		if updatePaths != nil {
			sfps := toServiceFieldPaths(updatePaths)
			sort.Strings(sfps) // TODO(jba): make tests pass without this
			mask = &pb.DocumentMask{FieldPaths: sfps}
		}
		w := &pb.Write{
			Operation:       &pb.Write_Update{doc},
			UpdateMask:      mask,
			CurrentDocument: pc,
		}
		ws = append(ws, w)
		pc = nil // If the precondition is in the write, we don't need it in the transform.
	}
	if len(transforms) > 0 || pc != nil {
		ws = append(ws, &pb.Write{
			Operation: &pb.Write_Transform{
				Transform: &pb.DocumentTransform{
					Document:        d.Path,
					FieldTransforms: transforms,
				},
			},
			CurrentDocument: pc,
		})
	}
	return ws
}

// arrayUnion is a special type in firestore. It instructs the server to add its
// elements to whatever array already exists, or to create an array if no value
// exists.
type arrayUnion struct {
	elems []interface{}
}

// ArrayUnion specifies elements to be added to whatever array already exists in
// the server, or to create an array if no value exists.
//
// If a value exists and it's an array, values are appended to it. Any duplicate
// value is ignored.
// If a value exists and it's not an array, the value is replaced by an array of
// the values in the ArrayUnion.
// If a value does not exist, an array of the values in the ArrayUnion is created.
//
// ArrayUnion must be the value of a field directly; it cannot appear in
// array or struct values, or in any value that is itself inside an array or
// struct.
func ArrayUnion(elems ...interface{}) arrayUnion {
	return arrayUnion{elems: elems}
}

// This helper converts an arrayUnion into a proto object.
func arrayUnionTransform(au arrayUnion, fp FieldPath) (*pb.DocumentTransform_FieldTransform, error) {
	var elems []*pb.Value
	for _, v := range au.elems {
		pv, _, err := toProtoValue(reflect.ValueOf(v))
		if err != nil {
			return nil, err
		}
		elems = append(elems, pv)
	}
	return &pb.DocumentTransform_FieldTransform{
		FieldPath: fp.toServiceFieldPath(),
		TransformType: &pb.DocumentTransform_FieldTransform_AppendMissingElements{
			AppendMissingElements: &pb.ArrayValue{Values: elems},
		},
	}, nil
}

// arrayRemove is a special type in firestore. It instructs the server to remove
// the specified values.
type arrayRemove struct {
	elems []interface{}
}

// ArrayRemove specifies elements to be removed from whatever array already
// exists in the server.
//
// If a value exists and it's an array, values are removed from it. All
// duplicate values are removed.
// If a value exists and it's not an array, the value is replaced by an empty
// array.
// If a value does not exist, an empty array is created.
//
// ArrayRemove must be the value of a field directly; it cannot appear in
// array or struct values, or in any value that is itself inside an array or
// struct.
func ArrayRemove(elems ...interface{}) arrayRemove {
	return arrayRemove{elems: elems}
}

// This helper converts an arrayRemove into a proto object.
func arrayRemoveTransform(ar arrayRemove, fp FieldPath) (*pb.DocumentTransform_FieldTransform, error) {
	var elems []*pb.Value
	for _, v := range ar.elems {
		// ServerTimestamp cannot occur in an array, so we ignore transformations here.
		pv, _, err := toProtoValue(reflect.ValueOf(v))
		if err != nil {
			return nil, err
		}
		elems = append(elems, pv)
	}
	return &pb.DocumentTransform_FieldTransform{
		FieldPath: fp.toServiceFieldPath(),
		TransformType: &pb.DocumentTransform_FieldTransform_RemoveAllFromArray{
			RemoveAllFromArray: &pb.ArrayValue{Values: elems},
		},
	}, nil
}

type sentinel int

const (
	// Delete is used as a value in a call to Update or Set with merge to indicate
	// that the corresponding key should be deleted.
	Delete sentinel = iota

	// ServerTimestamp is used as a value in a call to Update to indicate that the
	// key's value should be set to the time at which the server processed
	// the request.
	//
	// ServerTimestamp must be the value of a field directly; it cannot appear in
	// array or struct values, or in any value that is itself inside an array or
	// struct.
	ServerTimestamp
)

func (s sentinel) String() string {
	switch s {
	case Delete:
		return "Delete"
	case ServerTimestamp:
		return "ServerTimestamp"
	default:
		return "<?sentinel?>"
	}
}

// An Update describes an update to a value referred to by a path.
// An Update should have either a non-empty Path or a non-empty FieldPath,
// but not both.
//
// See DocumentRef.Create for acceptable values.
// To delete a field, specify firestore.Delete as the value.
type Update struct {
	Path      string // Will be split on dots, and must not contain any of "˜*/[]".
	FieldPath FieldPath
	Value     interface{}
}

// An fpv is a pair of validated FieldPath and value.
type fpv struct {
	fieldPath FieldPath
	value     interface{}
}

func (u *Update) process() (fpv, error) {
	if (u.Path != "") == (u.FieldPath != nil) {
		return fpv{}, fmt.Errorf("firestore: update %+v should have exactly one of Path or FieldPath", u)
	}
	fp := u.FieldPath
	var err error
	if fp == nil {
		fp, err = parseDotSeparatedString(u.Path)
		if err != nil {
			return fpv{}, err
		}
	}
	if err := fp.validate(); err != nil {
		return fpv{}, err
	}
	return fpv{fp, u.Value}, nil
}

// Update updates the document. The values at the given
// field paths are replaced, but other fields of the stored document are untouched.
func (d *DocumentRef) Update(ctx context.Context, updates []Update, preconds ...Precondition) (_ *WriteResult, err error) {
	ctx = trace.StartSpan(ctx, "cloud.google.com/go/firestore.DocumentRef.Update")
	defer func() { trace.EndSpan(ctx, err) }()

	ws, err := d.newUpdatePathWrites(updates, preconds)
	if err != nil {
		return nil, err
	}
	return d.Parent.c.commit1(ctx, ws)
}

// Collections returns an iterator over the immediate sub-collections of the document.
func (d *DocumentRef) Collections(ctx context.Context) *CollectionIterator {
	client := d.Parent.c
	it := &CollectionIterator{
		client: client,
		parent: d,
		it: client.c.ListCollectionIds(
			withResourceHeader(ctx, client.path()),
			&pb.ListCollectionIdsRequest{Parent: d.Path}),
	}
	it.pageInfo, it.nextFunc = iterator.NewPageInfo(
		it.fetch,
		func() int { return len(it.items) },
		func() interface{} { b := it.items; it.items = nil; return b })
	return it
}

// CollectionIterator is an iterator over sub-collections of a document.
type CollectionIterator struct {
	client   *Client
	parent   *DocumentRef
	it       *vkit.StringIterator
	pageInfo *iterator.PageInfo
	nextFunc func() error
	items    []*CollectionRef
	err      error
}

// PageInfo supports pagination. See the google.golang.org/api/iterator package for details.
func (it *CollectionIterator) PageInfo() *iterator.PageInfo { return it.pageInfo }

// Next returns the next result. Its second return value is iterator.Done if there
// are no more results. Once Next returns Done, all subsequent calls will return
// Done.
func (it *CollectionIterator) Next() (*CollectionRef, error) {
	if err := it.nextFunc(); err != nil {
		return nil, err
	}
	item := it.items[0]
	it.items = it.items[1:]
	return item, nil
}

func (it *CollectionIterator) fetch(pageSize int, pageToken string) (string, error) {
	if it.err != nil {
		return "", it.err
	}
	return iterFetch(pageSize, pageToken, it.it.PageInfo(), func() error {
		id, err := it.it.Next()
		if err != nil {
			return err
		}
		var cr *CollectionRef
		if it.parent == nil {
			cr = newTopLevelCollRef(it.client, it.client.path(), id)
		} else {
			cr = newCollRefWithParent(it.client, it.parent, id)
		}
		it.items = append(it.items, cr)
		return nil
	})
}

// GetAll returns all the collections remaining from the iterator.
func (it *CollectionIterator) GetAll() ([]*CollectionRef, error) {
	var crs []*CollectionRef
	for {
		cr, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		crs = append(crs, cr)
	}
	return crs, nil
}

// Common fetch code for iterators that are backed by vkit iterators.
// TODO(jba): dedup with same function in logging/logadmin.
func iterFetch(pageSize int, pageToken string, pi *iterator.PageInfo, next func() error) (string, error) {
	pi.MaxSize = pageSize
	pi.Token = pageToken
	// Get one item, which will fill the buffer.
	if err := next(); err != nil {
		return "", err
	}
	// Collect the rest of the buffer.
	for pi.Remaining() > 0 {
		if err := next(); err != nil {
			return "", err
		}
	}
	return pi.Token, nil
}

// Snapshots returns an iterator over snapshots of the document. Each time the document
// changes or is added or deleted, a new snapshot will be generated.
func (d *DocumentRef) Snapshots(ctx context.Context) *DocumentSnapshotIterator {
	return &DocumentSnapshotIterator{
		docref: d,
		ws:     newWatchStreamForDocument(ctx, d),
	}
}

// DocumentSnapshotIterator is an iterator over snapshots of a document.
// Call Next on the iterator to get a snapshot of the document each time it changes.
// Call Stop on the iterator when done.
//
// For an example, see DocumentRef.Snapshots.
type DocumentSnapshotIterator struct {
	docref *DocumentRef
	ws     *watchStream
}

// Next blocks until the document changes, then returns the DocumentSnapshot for
// the current state of the document. If the document has been deleted, Next
// returns a DocumentSnapshot whose Exists method returns false.
//
// Next never returns iterator.Done unless it is called after Stop.
func (it *DocumentSnapshotIterator) Next() (*DocumentSnapshot, error) {
	btree, _, readTime, err := it.ws.nextSnapshot()
	if err != nil {
		if err == io.EOF {
			err = iterator.Done
		}
		// watchStream's error is sticky, so SnapshotIterator does not need to remember it.
		return nil, err
	}
	if btree.Len() == 0 { // document deleted
		return &DocumentSnapshot{Ref: it.docref, ReadTime: readTime}, nil
	}
	snap, _ := btree.At(0)
	return snap.(*DocumentSnapshot), nil
}

// Stop stops receiving snapshots. You should always call Stop when you are done with
// a DocumentSnapshotIterator, to free up resources. It is not safe to call Stop
// concurrently with Next.
func (it *DocumentSnapshotIterator) Stop() {
	it.ws.stop()
}
