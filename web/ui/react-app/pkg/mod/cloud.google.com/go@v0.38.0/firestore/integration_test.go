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
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
	"time"

	"cloud.google.com/go/internal/pretty"
	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/internal/uid"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/api/option"
	"google.golang.org/genproto/googleapis/type/latlng"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
)

func TestMain(m *testing.M) {
	initIntegrationTest()
	status := m.Run()
	cleanupIntegrationTest()
	os.Exit(status)
}

const (
	envProjID     = "GCLOUD_TESTS_GOLANG_FIRESTORE_PROJECT_ID"
	envPrivateKey = "GCLOUD_TESTS_GOLANG_FIRESTORE_KEY"
)

var (
	iClient       *Client
	iColl         *CollectionRef
	collectionIDs = uid.NewSpace("go-integration-test", nil)
)

func initIntegrationTest() {
	flag.Parse() // needed for testing.Short()
	if testing.Short() {
		return
	}
	ctx := context.Background()
	testProjectID := os.Getenv(envProjID)
	if testProjectID == "" {
		log.Println("Integration tests skipped. See CONTRIBUTING.md for details")
		return
	}
	ts := testutil.TokenSourceEnv(ctx, envPrivateKey,
		"https://www.googleapis.com/auth/cloud-platform",
		"https://www.googleapis.com/auth/datastore")
	if ts == nil {
		log.Fatal("The project key must be set. See CONTRIBUTING.md for details")
	}
	ti := &testInterceptor{dbPath: "projects/" + testProjectID + "/databases/(default)"}
	c, err := NewClient(ctx, testProjectID,
		option.WithTokenSource(ts),
		option.WithGRPCDialOption(grpc.WithUnaryInterceptor(ti.interceptUnary)),
		option.WithGRPCDialOption(grpc.WithStreamInterceptor(ti.interceptStream)),
	)
	if err != nil {
		log.Fatalf("NewClient: %v", err)
	}
	iClient = c
	iColl = c.Collection(collectionIDs.New())
	refDoc := iColl.NewDoc()
	integrationTestMap["ref"] = refDoc
	wantIntegrationTestMap["ref"] = refDoc
	integrationTestStruct.Ref = refDoc
}

type testInterceptor struct {
	dbPath string
}

func (ti *testInterceptor) interceptUnary(ctx context.Context, method string, req, res interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	ti.checkMetadata(ctx, method)
	return invoker(ctx, method, req, res, cc, opts...)
}

func (ti *testInterceptor) interceptStream(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	ti.checkMetadata(ctx, method)
	return streamer(ctx, desc, cc, method, opts...)
}

func (ti *testInterceptor) checkMetadata(ctx context.Context, method string) {
	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		log.Fatalf("method %s: bad metadata", method)
	}
	for _, h := range []string{"google-cloud-resource-prefix", "x-goog-api-client"} {
		v, ok := md[h]
		if !ok {
			log.Fatalf("method %s, header %s missing", method, h)
		}
		if len(v) != 1 {
			log.Fatalf("method %s, header %s: bad value %v", method, h, v)
		}
	}
	v := md["google-cloud-resource-prefix"][0]
	if v != ti.dbPath {
		log.Fatalf("method %s: bad resource prefix header:  %q", method, v)
	}
}

func cleanupIntegrationTest() {
	if iClient == nil {
		return
	}
	// TODO(jba): delete everything in integrationColl.
	iClient.Close()
}

// integrationClient should be called by integration tests to get a valid client. It will never
// return nil. If integrationClient returns, an integration test can proceed without
// further checks.
func integrationClient(t *testing.T) *Client {
	if testing.Short() {
		t.Skip("Integration tests skipped in short mode")
	}
	if iClient == nil {
		t.SkipNow() // log message printed in initIntegrationTest
	}
	return iClient
}

func integrationColl(t *testing.T) *CollectionRef {
	_ = integrationClient(t)
	return iColl
}

type integrationTestStructType struct {
	Int         int
	Str         string
	Bool        bool
	Float       float32
	Null        interface{}
	Bytes       []byte
	Time        time.Time
	Geo, NilGeo *latlng.LatLng
	Ref         *DocumentRef
}

var (
	integrationTime = time.Date(2017, 3, 20, 1, 2, 3, 456789, time.UTC)
	// Firestore times are accurate only to microseconds.
	wantIntegrationTime = time.Date(2017, 3, 20, 1, 2, 3, 456000, time.UTC)

	integrationGeo = &latlng.LatLng{Latitude: 30, Longitude: 70}

	// Use this when writing a doc.
	integrationTestMap = map[string]interface{}{
		"int":   1,
		"str":   "two",
		"bool":  true,
		"float": 3.14,
		"null":  nil,
		"bytes": []byte("bytes"),
		"*":     map[string]interface{}{"`": 4},
		"time":  integrationTime,
		"geo":   integrationGeo,
		"ref":   nil, // populated by initIntegrationTest
	}

	// The returned data is slightly different.
	wantIntegrationTestMap = map[string]interface{}{
		"int":   int64(1),
		"str":   "two",
		"bool":  true,
		"float": 3.14,
		"null":  nil,
		"bytes": []byte("bytes"),
		"*":     map[string]interface{}{"`": int64(4)},
		"time":  wantIntegrationTime,
		"geo":   integrationGeo,
		"ref":   nil, // populated by initIntegrationTest
	}

	integrationTestStruct = integrationTestStructType{
		Int:    1,
		Str:    "two",
		Bool:   true,
		Float:  3.14,
		Null:   nil,
		Bytes:  []byte("bytes"),
		Time:   integrationTime,
		Geo:    integrationGeo,
		NilGeo: nil,
		Ref:    nil, // populated by initIntegrationTest
	}
)

func TestIntegration_Create(t *testing.T) {
	ctx := context.Background()
	doc := integrationColl(t).NewDoc()
	start := time.Now()
	h := testHelper{t}
	wr := h.mustCreate(doc, integrationTestMap)
	end := time.Now()
	checkTimeBetween(t, wr.UpdateTime, start, end)
	_, err := doc.Create(ctx, integrationTestMap)
	codeEq(t, "Create on a present doc", codes.AlreadyExists, err)
	// OK to create an empty document.
	_, err = integrationColl(t).NewDoc().Create(ctx, map[string]interface{}{})
	codeEq(t, "Create empty doc", codes.OK, err)
}

func TestIntegration_Get(t *testing.T) {
	ctx := context.Background()
	doc := integrationColl(t).NewDoc()
	h := testHelper{t}
	h.mustCreate(doc, integrationTestMap)
	ds := h.mustGet(doc)
	if ds.CreateTime != ds.UpdateTime {
		t.Errorf("create time %s != update time %s", ds.CreateTime, ds.UpdateTime)
	}
	got := ds.Data()
	if want := wantIntegrationTestMap; !testEqual(got, want) {
		t.Errorf("got\n%v\nwant\n%v", pretty.Value(got), pretty.Value(want))
	}

	doc = integrationColl(t).NewDoc()
	empty := map[string]interface{}{}
	h.mustCreate(doc, empty)
	ds = h.mustGet(doc)
	if ds.CreateTime != ds.UpdateTime {
		t.Errorf("create time %s != update time %s", ds.CreateTime, ds.UpdateTime)
	}
	if got, want := ds.Data(), empty; !testEqual(got, want) {
		t.Errorf("got\n%v\nwant\n%v", pretty.Value(got), pretty.Value(want))
	}

	ds, err := integrationColl(t).NewDoc().Get(ctx)
	codeEq(t, "Get on a missing doc", codes.NotFound, err)
	if ds == nil || ds.Exists() {
		t.Fatal("got nil or existing doc snapshot, want !ds.Exists")
	}
	if ds.ReadTime.IsZero() {
		t.Error("got zero read time")
	}
}

func TestIntegration_GetAll(t *testing.T) {
	type getAll struct{ N int }

	h := testHelper{t}
	coll := integrationColl(t)
	ctx := context.Background()
	var docRefs []*DocumentRef
	for i := 0; i < 5; i++ {
		doc := coll.NewDoc()
		docRefs = append(docRefs, doc)
		if i != 3 {
			h.mustCreate(doc, getAll{N: i})
		}
	}
	docSnapshots, err := iClient.GetAll(ctx, docRefs)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(docSnapshots), len(docRefs); got != want {
		t.Fatalf("got %d snapshots, want %d", got, want)
	}
	for i, ds := range docSnapshots {
		if i == 3 {
			if ds == nil || ds.Exists() {
				t.Fatal("got nil or existing doc snapshot, want !ds.Exists")
			}
			err := ds.DataTo(nil)
			codeEq(t, "DataTo on a missing doc", codes.NotFound, err)
		} else {
			var got getAll
			if err := ds.DataTo(&got); err != nil {
				t.Fatal(err)
			}
			want := getAll{N: i}
			if got != want {
				t.Errorf("%d: got %+v, want %+v", i, got, want)
			}
		}
		if ds.ReadTime.IsZero() {
			t.Errorf("%d: got zero read time", i)
		}
	}
}

func TestIntegration_Add(t *testing.T) {
	start := time.Now()
	_, wr, err := integrationColl(t).Add(context.Background(), integrationTestMap)
	if err != nil {
		t.Fatal(err)
	}
	end := time.Now()
	checkTimeBetween(t, wr.UpdateTime, start, end)
}

func TestIntegration_Set(t *testing.T) {
	coll := integrationColl(t)
	h := testHelper{t}
	ctx := context.Background()

	// Set Should be able to create a new doc.
	doc := coll.NewDoc()
	wr1 := h.mustSet(doc, integrationTestMap)
	// Calling Set on the doc completely replaces the contents.
	// The update time should increase.
	newData := map[string]interface{}{
		"str": "change",
		"x":   "1",
	}
	wr2 := h.mustSet(doc, newData)
	if !wr1.UpdateTime.Before(wr2.UpdateTime) {
		t.Errorf("update time did not increase: old=%s, new=%s", wr1.UpdateTime, wr2.UpdateTime)
	}
	ds := h.mustGet(doc)
	if got := ds.Data(); !testEqual(got, newData) {
		t.Errorf("got %v, want %v", got, newData)
	}

	newData = map[string]interface{}{
		"str": "1",
		"x":   "2",
		"y":   "3",
	}
	// SetOptions:
	// Only fields mentioned in the Merge option will be changed.
	// In this case, "str" will not be changed to "1".
	wr3, err := doc.Set(ctx, newData, Merge([]string{"x"}, []string{"y"}))
	if err != nil {
		t.Fatal(err)
	}
	ds = h.mustGet(doc)
	want := map[string]interface{}{
		"str": "change",
		"x":   "2",
		"y":   "3",
	}
	if got := ds.Data(); !testEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if !wr2.UpdateTime.Before(wr3.UpdateTime) {
		t.Errorf("update time did not increase: old=%s, new=%s", wr2.UpdateTime, wr3.UpdateTime)
	}

	// Another way to change only x and y is to pass a map with only
	// those keys, and use MergeAll.
	wr4, err := doc.Set(ctx, map[string]interface{}{"x": "4", "y": "5"}, MergeAll)
	if err != nil {
		t.Fatal(err)
	}
	ds = h.mustGet(doc)
	want = map[string]interface{}{
		"str": "change",
		"x":   "4",
		"y":   "5",
	}
	if got := ds.Data(); !testEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if !wr3.UpdateTime.Before(wr4.UpdateTime) {
		t.Errorf("update time did not increase: old=%s, new=%s", wr3.UpdateTime, wr4.UpdateTime)
	}

	// use firestore.Delete to delete a field.
	// TODO(deklerk) We should be able to use mustSet, but then we get a test error. We should investigate this.
	_, err = doc.Set(ctx, map[string]interface{}{"str": Delete}, MergeAll)
	if err != nil {
		t.Fatal(err)
	}
	ds = h.mustGet(doc)
	want = map[string]interface{}{
		"x": "4",
		"y": "5",
	}
	if got := ds.Data(); !testEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}

	// Writing an empty doc with MergeAll should create the doc.
	doc2 := coll.NewDoc()
	want = map[string]interface{}{}
	h.mustSet(doc2, want, MergeAll)
	ds = h.mustGet(doc2)
	if got := ds.Data(); !testEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestIntegration_Delete(t *testing.T) {
	ctx := context.Background()
	doc := integrationColl(t).NewDoc()
	h := testHelper{t}
	h.mustCreate(doc, integrationTestMap)
	h.mustDelete(doc)
	// Confirm that doc doesn't exist.
	if _, err := doc.Get(ctx); grpc.Code(err) != codes.NotFound {
		t.Fatalf("got error <%v>, want NotFound", err)
	}

	er := func(_ *WriteResult, err error) error { return err }

	codeEq(t, "Delete on a missing doc", codes.OK,
		er(doc.Delete(ctx)))
	// TODO(jba): confirm that the server should return InvalidArgument instead of
	// FailedPrecondition.
	wr := h.mustCreate(doc, integrationTestMap)
	codeEq(t, "Delete with wrong LastUpdateTime", codes.FailedPrecondition,
		er(doc.Delete(ctx, LastUpdateTime(wr.UpdateTime.Add(-time.Millisecond)))))
	codeEq(t, "Delete with right LastUpdateTime", codes.OK,
		er(doc.Delete(ctx, LastUpdateTime(wr.UpdateTime))))
}

func TestIntegration_Update(t *testing.T) {
	ctx := context.Background()
	doc := integrationColl(t).NewDoc()
	h := testHelper{t}

	h.mustCreate(doc, integrationTestMap)
	fpus := []Update{
		{Path: "bool", Value: false},
		{Path: "time", Value: 17},
		{FieldPath: []string{"*", "`"}, Value: 18},
		{Path: "null", Value: Delete},
		{Path: "noSuchField", Value: Delete}, // deleting a non-existent field is a no-op
	}
	wr := h.mustUpdate(doc, fpus)
	ds := h.mustGet(doc)
	got := ds.Data()
	want := copyMap(wantIntegrationTestMap)
	want["bool"] = false
	want["time"] = int64(17)
	want["*"] = map[string]interface{}{"`": int64(18)}
	delete(want, "null")
	if !testEqual(got, want) {
		t.Errorf("got\n%#v\nwant\n%#v", got, want)
	}

	er := func(_ *WriteResult, err error) error { return err }

	codeEq(t, "Update on missing doc", codes.NotFound,
		er(integrationColl(t).NewDoc().Update(ctx, fpus)))
	codeEq(t, "Update with wrong LastUpdateTime", codes.FailedPrecondition,
		er(doc.Update(ctx, fpus, LastUpdateTime(wr.UpdateTime.Add(-time.Millisecond)))))
	codeEq(t, "Update with right LastUpdateTime", codes.OK,
		er(doc.Update(ctx, fpus, LastUpdateTime(wr.UpdateTime))))
}

func TestIntegration_Collections(t *testing.T) {
	ctx := context.Background()
	c := integrationClient(t)
	h := testHelper{t}
	got, err := c.Collections(ctx).GetAll()
	if err != nil {
		t.Fatal(err)
	}
	// There should be at least one collection.
	if len(got) == 0 {
		t.Error("got 0 top-level collections, want at least one")
	}

	doc := integrationColl(t).NewDoc()
	got, err = doc.Collections(ctx).GetAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("got %d collections, want 0", len(got))
	}
	var want []*CollectionRef
	for i := 0; i < 3; i++ {
		id := collectionIDs.New()
		cr := doc.Collection(id)
		want = append(want, cr)
		h.mustCreate(cr.NewDoc(), integrationTestMap)
	}
	got, err = doc.Collections(ctx).GetAll()
	if err != nil {
		t.Fatal(err)
	}
	if !testEqual(got, want) {
		t.Errorf("got\n%#v\nwant\n%#v", got, want)
	}
}

func TestIntegration_ServerTimestamp(t *testing.T) {
	type S struct {
		A int
		B time.Time
		C time.Time `firestore:"C.C,serverTimestamp"`
		D map[string]interface{}
		E time.Time `firestore:",omitempty,serverTimestamp"`
	}
	data := S{
		A: 1,
		B: aTime,
		// C is unset, so will get the server timestamp.
		D: map[string]interface{}{"x": ServerTimestamp},
		// E is unset, so will get the server timestamp.
	}
	h := testHelper{t}
	doc := integrationColl(t).NewDoc()
	// Bound times of the RPC, with some slack for clock skew.
	start := time.Now()
	h.mustCreate(doc, data)
	end := time.Now()
	ds := h.mustGet(doc)
	var got S
	if err := ds.DataTo(&got); err != nil {
		t.Fatal(err)
	}
	if !testEqual(got.B, aTime) {
		t.Errorf("B: got %s, want %s", got.B, aTime)
	}
	checkTimeBetween(t, got.C, start, end)
	if g, w := got.D["x"], got.C; !testEqual(g, w) {
		t.Errorf(`D["x"] = %s, want equal to C (%s)`, g, w)
	}
	if g, w := got.E, got.C; !testEqual(g, w) {
		t.Errorf(`E = %s, want equal to C (%s)`, g, w)
	}
}

func TestIntegration_MergeServerTimestamp(t *testing.T) {
	doc := integrationColl(t).NewDoc()
	h := testHelper{t}

	// Create a doc with an ordinary field "a" and a ServerTimestamp field "b".
	h.mustSet(doc, map[string]interface{}{"a": 1, "b": ServerTimestamp})
	docSnap := h.mustGet(doc)
	data1 := docSnap.Data()
	// Merge with a document with a different value of "a". However,
	// specify only "b" in the list of merge fields.
	h.mustSet(doc, map[string]interface{}{"a": 2, "b": ServerTimestamp}, Merge([]string{"b"}))
	// The result should leave "a" unchanged, while "b" is updated.
	docSnap = h.mustGet(doc)
	data2 := docSnap.Data()
	if got, want := data2["a"], data1["a"]; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	t1 := data1["b"].(time.Time)
	t2 := data2["b"].(time.Time)
	if !t1.Before(t2) {
		t.Errorf("got t1=%s, t2=%s; want t1 before t2", t1, t2)
	}
}

func TestIntegration_MergeNestedServerTimestamp(t *testing.T) {
	doc := integrationColl(t).NewDoc()
	h := testHelper{t}

	// Create a doc with an ordinary field "a" a ServerTimestamp field "b",
	// and a second ServerTimestamp field "c.d".
	h.mustSet(doc, map[string]interface{}{
		"a": 1,
		"b": ServerTimestamp,
		"c": map[string]interface{}{"d": ServerTimestamp},
	})
	data1 := h.mustGet(doc).Data()
	// Merge with a document with a different value of "a". However,
	// specify only "c.d" in the list of merge fields.
	h.mustSet(doc, map[string]interface{}{
		"a": 2,
		"b": ServerTimestamp,
		"c": map[string]interface{}{"d": ServerTimestamp},
	}, Merge([]string{"c", "d"}))
	// The result should leave "a" and "b" unchanged, while "c.d" is updated.
	data2 := h.mustGet(doc).Data()
	if got, want := data2["a"], data1["a"]; got != want {
		t.Errorf("a: got %v, want %v", got, want)
	}
	want := data1["b"].(time.Time)
	got := data2["b"].(time.Time)
	if !got.Equal(want) {
		t.Errorf("b: got %s, want %s", got, want)
	}
	t1 := data1["c"].(map[string]interface{})["d"].(time.Time)
	t2 := data2["c"].(map[string]interface{})["d"].(time.Time)
	if !t1.Before(t2) {
		t.Errorf("got t1=%s, t2=%s; want t1 before t2", t1, t2)
	}
}

func TestIntegration_WriteBatch(t *testing.T) {
	ctx := context.Background()
	b := integrationClient(t).Batch()
	h := testHelper{t}
	doc1 := iColl.NewDoc()
	doc2 := iColl.NewDoc()
	b.Create(doc1, integrationTestMap)
	b.Set(doc2, integrationTestMap)
	b.Update(doc1, []Update{{Path: "bool", Value: false}})
	b.Update(doc1, []Update{{Path: "str", Value: Delete}})

	wrs, err := b.Commit(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(wrs), 4; got != want {
		t.Fatalf("got %d WriteResults, want %d", got, want)
	}
	got1 := h.mustGet(doc1).Data()
	want := copyMap(wantIntegrationTestMap)
	want["bool"] = false
	delete(want, "str")
	if !testEqual(got1, want) {
		t.Errorf("got\n%#v\nwant\n%#v", got1, want)
	}
	got2 := h.mustGet(doc2).Data()
	if !testEqual(got2, wantIntegrationTestMap) {
		t.Errorf("got\n%#v\nwant\n%#v", got2, wantIntegrationTestMap)
	}
	// TODO(jba): test two updates to the same document when it is supported.
	// TODO(jba): test verify when it is supported.
}

func TestIntegration_Query(t *testing.T) {
	ctx := context.Background()
	coll := integrationColl(t)
	h := testHelper{t}
	var wants []map[string]interface{}
	for i := 0; i < 3; i++ {
		doc := coll.NewDoc()
		// To support running this test in parallel with the others, use a field name
		// that we don't use anywhere else.
		h.mustCreate(doc, map[string]interface{}{"q": i, "x": 1})
		wants = append(wants, map[string]interface{}{"q": int64(i)})
	}
	q := coll.Select("q").OrderBy("q", Asc)
	for i, test := range []struct {
		q    Query
		want []map[string]interface{}
	}{
		{q, wants},
		{q.Where("q", ">", 1), wants[2:]},
		{q.WherePath([]string{"q"}, ">", 1), wants[2:]},
		{q.Offset(1).Limit(1), wants[1:2]},
		{q.StartAt(1), wants[1:]},
		{q.StartAfter(1), wants[2:]},
		{q.EndAt(1), wants[:2]},
		{q.EndBefore(1), wants[:1]},
	} {
		gotDocs, err := test.q.Documents(ctx).GetAll()
		if err != nil {
			t.Errorf("#%d: %+v: %v", i, test.q, err)
			continue
		}
		if len(gotDocs) != len(test.want) {
			t.Errorf("#%d: %+v: got %d docs, want %d", i, test.q, len(gotDocs), len(test.want))
			continue
		}
		for j, g := range gotDocs {
			if got, want := g.Data(), test.want[j]; !testEqual(got, want) {
				t.Errorf("#%d: %+v, #%d: got\n%+v\nwant\n%+v", i, test.q, j, got, want)
			}
		}
	}
	_, err := coll.Select("q").Where("x", "==", 1).OrderBy("q", Asc).Documents(ctx).GetAll()
	codeEq(t, "Where and OrderBy on different fields without an index", codes.FailedPrecondition, err)

	// Using the collection itself as the query should return the full documents.
	allDocs, err := coll.Documents(ctx).GetAll()
	if err != nil {
		t.Fatal(err)
	}
	seen := map[int64]bool{} // "q" values we see
	for _, d := range allDocs {
		data := d.Data()
		q, ok := data["q"]
		if !ok {
			// A document from another test.
			continue
		}
		if seen[q.(int64)] {
			t.Errorf("%v: duplicate doc", data)
		}
		seen[q.(int64)] = true
		if data["x"] != int64(1) {
			t.Errorf("%v: wrong or missing 'x'", data)
		}
		if len(data) != 2 {
			t.Errorf("%v: want two keys", data)
		}
	}
	if got, want := len(seen), len(wants); got != want {
		t.Errorf("got %d docs with 'q', want %d", len(seen), len(wants))
	}
}

// Test unary filters.
func TestIntegration_QueryUnary(t *testing.T) {
	ctx := context.Background()
	coll := integrationColl(t)
	h := testHelper{t}
	h.mustCreate(coll.NewDoc(), map[string]interface{}{"x": 2, "q": "a"})
	h.mustCreate(coll.NewDoc(), map[string]interface{}{"x": 2, "q": nil})
	h.mustCreate(coll.NewDoc(), map[string]interface{}{"x": 2, "q": math.NaN()})
	wantNull := map[string]interface{}{"q": nil}
	wantNaN := map[string]interface{}{"q": math.NaN()}

	base := coll.Select("q").Where("x", "==", 2)
	for _, test := range []struct {
		q    Query
		want map[string]interface{}
	}{
		{base.Where("q", "==", nil), wantNull},
		{base.Where("q", "==", math.NaN()), wantNaN},
	} {
		got, err := test.q.Documents(ctx).GetAll()
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 {
			t.Errorf("got %d responses, want 1", len(got))
			continue
		}
		if g, w := got[0].Data(), test.want; !testEqual(g, w) {
			t.Errorf("%v: got %v, want %v", test.q, g, w)
		}
	}
}

// Test the special DocumentID field in queries.
func TestIntegration_QueryName(t *testing.T) {
	ctx := context.Background()
	h := testHelper{t}

	checkIDs := func(q Query, wantIDs []string) {
		gots, err := q.Documents(ctx).GetAll()
		if err != nil {
			t.Fatal(err)
		}
		if len(gots) != len(wantIDs) {
			t.Fatalf("got %d, want %d", len(gots), len(wantIDs))
		}
		for i, g := range gots {
			if got, want := g.Ref.ID, wantIDs[i]; got != want {
				t.Errorf("#%d: got %s, want %s", i, got, want)
			}
		}
	}

	coll := integrationColl(t)
	var wantIDs []string
	for i := 0; i < 3; i++ {
		doc := coll.NewDoc()
		h.mustCreate(doc, map[string]interface{}{"nm": 1})
		wantIDs = append(wantIDs, doc.ID)
	}
	sort.Strings(wantIDs)
	q := coll.Where("nm", "==", 1).OrderBy(DocumentID, Asc)
	checkIDs(q, wantIDs)

	// Empty Select.
	q = coll.Select().Where("nm", "==", 1).OrderBy(DocumentID, Asc)
	checkIDs(q, wantIDs)

	// Test cursors with __name__.
	checkIDs(q.StartAt(wantIDs[1]), wantIDs[1:])
	checkIDs(q.EndAt(wantIDs[1]), wantIDs[:2])
}

func TestIntegration_QueryNested(t *testing.T) {
	ctx := context.Background()
	h := testHelper{t}
	coll1 := integrationColl(t)
	doc1 := coll1.NewDoc()
	coll2 := doc1.Collection(collectionIDs.New())
	doc2 := coll2.NewDoc()
	wantData := map[string]interface{}{"x": int64(1)}
	h.mustCreate(doc2, wantData)
	q := coll2.Select("x")
	got, err := q.Documents(ctx).GetAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d docs, want 1", len(got))
	}
	if gotData := got[0].Data(); !testEqual(gotData, wantData) {
		t.Errorf("got\n%+v\nwant\n%+v", gotData, wantData)
	}
}

func TestIntegration_RunTransaction(t *testing.T) {
	ctx := context.Background()
	h := testHelper{t}

	type Player struct {
		Name  string
		Score int
		Star  bool `firestore:"*"`
	}

	pat := Player{Name: "Pat", Score: 3, Star: false}
	client := integrationClient(t)
	patDoc := iColl.Doc("pat")
	var anError error
	incPat := func(_ context.Context, tx *Transaction) error {
		doc, err := tx.Get(patDoc)
		if err != nil {
			return err
		}
		score, err := doc.DataAt("Score")
		if err != nil {
			return err
		}
		// Since the Star field is called "*", we must use DataAtPath to get it.
		star, err := doc.DataAtPath([]string{"*"})
		if err != nil {
			return err
		}
		err = tx.Update(patDoc, []Update{{Path: "Score", Value: int(score.(int64) + 7)}})
		if err != nil {
			return err
		}
		// Since the Star field is called "*", we must use Update to change it.
		err = tx.Update(patDoc,
			[]Update{{FieldPath: []string{"*"}, Value: !star.(bool)}})
		if err != nil {
			return err
		}
		return anError
	}

	h.mustCreate(patDoc, pat)
	err := client.RunTransaction(ctx, incPat)
	if err != nil {
		t.Fatal(err)
	}
	ds := h.mustGet(patDoc)
	var got Player
	if err := ds.DataTo(&got); err != nil {
		t.Fatal(err)
	}
	want := Player{Name: "Pat", Score: 10, Star: true}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}

	// Function returns error, so transaction is rolled back and no writes happen.
	anError = errors.New("bad")
	err = client.RunTransaction(ctx, incPat)
	if err != anError {
		t.Fatalf("got %v, want %v", err, anError)
	}
	if err := ds.DataTo(&got); err != nil {
		t.Fatal(err)
	}
	// want is same as before.
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestIntegration_TransactionGetAll(t *testing.T) {
	ctx := context.Background()
	h := testHelper{t}
	type Player struct {
		Name  string
		Score int
	}
	lee := Player{Name: "Lee", Score: 3}
	sam := Player{Name: "Sam", Score: 1}
	client := integrationClient(t)
	leeDoc := iColl.Doc("lee")
	samDoc := iColl.Doc("sam")
	h.mustCreate(leeDoc, lee)
	h.mustCreate(samDoc, sam)

	err := client.RunTransaction(ctx, func(_ context.Context, tx *Transaction) error {
		docs, err := tx.GetAll([]*DocumentRef{samDoc, leeDoc})
		if err != nil {
			return err
		}
		for i, want := range []Player{sam, lee} {
			var got Player
			if err := docs[i].DataTo(&got); err != nil {
				return err
			}
			if !testutil.Equal(got, want) {
				return fmt.Errorf("got %+v, want %+v", got, want)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestIntegration_WatchDocument(t *testing.T) {
	coll := integrationColl(t)
	ctx := context.Background()
	h := testHelper{t}
	doc := coll.NewDoc()
	it := doc.Snapshots(ctx)
	defer it.Stop()

	next := func() *DocumentSnapshot {
		snap, err := it.Next()
		if err != nil {
			t.Fatal(err)
		}
		return snap
	}

	snap := next()
	if snap.Exists() {
		t.Fatal("snapshot exists; it should not")
	}
	want := map[string]interface{}{"a": int64(1), "b": "two"}
	h.mustCreate(doc, want)
	snap = next()
	if got := snap.Data(); !testutil.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}

	h.mustUpdate(doc, []Update{{Path: "a", Value: int64(2)}})
	want["a"] = int64(2)
	snap = next()
	if got := snap.Data(); !testutil.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}

	h.mustDelete(doc)
	snap = next()
	if snap.Exists() {
		t.Fatal("snapshot exists; it should not")
	}

	h.mustCreate(doc, want)
	snap = next()
	if got := snap.Data(); !testutil.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestIntegration_ArrayUnion_Create(t *testing.T) {
	path := "somepath"
	data := map[string]interface{}{
		path: ArrayUnion("a", "b"),
	}

	doc := integrationColl(t).NewDoc()
	h := testHelper{t}
	h.mustCreate(doc, data)
	ds := h.mustGet(doc)
	var gotMap map[string][]string
	if err := ds.DataTo(&gotMap); err != nil {
		t.Fatal(err)
	}
	if _, ok := gotMap[path]; !ok {
		t.Fatalf("expected a %v key in data, got %v", path, gotMap)
	}

	want := []string{"a", "b"}
	for i, v := range gotMap[path] {
		if v != want[i] {
			t.Fatalf("got\n%#v\nwant\n%#v", gotMap[path], want)
		}
	}
}

func TestIntegration_ArrayUnion_Update(t *testing.T) {
	doc := integrationColl(t).NewDoc()
	h := testHelper{t}
	path := "somepath"

	h.mustCreate(doc, map[string]interface{}{
		path: []string{"a", "b"},
	})
	fpus := []Update{
		{
			Path:  path,
			Value: ArrayUnion("this should be added"),
		},
	}
	h.mustUpdate(doc, fpus)
	ds := h.mustGet(doc)
	var gotMap map[string][]string
	err := ds.DataTo(&gotMap)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := gotMap[path]; !ok {
		t.Fatalf("expected a %v key in data, got %v", path, gotMap)
	}

	want := []string{"a", "b", "this should be added"}
	for i, v := range gotMap[path] {
		if v != want[i] {
			t.Fatalf("got\n%#v\nwant\n%#v", gotMap[path], want)
		}
	}
}

func TestIntegration_ArrayUnion_Set(t *testing.T) {
	coll := integrationColl(t)
	h := testHelper{t}
	path := "somepath"

	doc := coll.NewDoc()
	newData := map[string]interface{}{
		path: ArrayUnion("a", "b"),
	}
	h.mustSet(doc, newData)
	ds := h.mustGet(doc)
	var gotMap map[string][]string
	if err := ds.DataTo(&gotMap); err != nil {
		t.Fatal(err)
	}
	if _, ok := gotMap[path]; !ok {
		t.Fatalf("expected a %v key in data, got %v", path, gotMap)
	}

	want := []string{"a", "b"}
	for i, v := range gotMap[path] {
		if v != want[i] {
			t.Fatalf("got\n%#v\nwant\n%#v", gotMap[path], want)
		}
	}
}

func TestIntegration_ArrayRemove_Create(t *testing.T) {
	doc := integrationColl(t).NewDoc()
	h := testHelper{t}
	path := "somepath"

	h.mustCreate(doc, map[string]interface{}{
		path: ArrayRemove("a", "b"),
	})

	ds := h.mustGet(doc)
	var gotMap map[string][]string
	err := ds.DataTo(&gotMap)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := gotMap[path]; !ok {
		t.Fatalf("expected a %v key in data, got %v", path, gotMap)
	}

	// A create with arrayRemove results in an empty array.
	want := []string(nil)
	if !testEqual(gotMap[path], want) {
		t.Fatalf("got\n%#v\nwant\n%#v", gotMap[path], want)
	}
}

func TestIntegration_ArrayRemove_Update(t *testing.T) {
	doc := integrationColl(t).NewDoc()
	h := testHelper{t}
	path := "somepath"

	h.mustCreate(doc, map[string]interface{}{
		path: []string{"a", "this should be removed", "c"},
	})
	fpus := []Update{
		{
			Path:  path,
			Value: ArrayRemove("this should be removed"),
		},
	}
	h.mustUpdate(doc, fpus)
	ds := h.mustGet(doc)
	var gotMap map[string][]string
	err := ds.DataTo(&gotMap)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := gotMap[path]; !ok {
		t.Fatalf("expected a %v key in data, got %v", path, gotMap)
	}

	want := []string{"a", "c"}
	for i, v := range gotMap[path] {
		if v != want[i] {
			t.Fatalf("got\n%#v\nwant\n%#v", gotMap[path], want)
		}
	}
}

func TestIntegration_ArrayRemove_Set(t *testing.T) {
	coll := integrationColl(t)
	h := testHelper{t}
	path := "somepath"

	doc := coll.NewDoc()
	newData := map[string]interface{}{
		path: ArrayRemove("a", "b"),
	}
	h.mustSet(doc, newData)
	ds := h.mustGet(doc)
	var gotMap map[string][]string
	if err := ds.DataTo(&gotMap); err != nil {
		t.Fatal(err)
	}
	if _, ok := gotMap[path]; !ok {
		t.Fatalf("expected a %v key in data, got %v", path, gotMap)
	}

	want := []string(nil)
	if !testEqual(gotMap[path], want) {
		t.Fatalf("got\n%#v\nwant\n%#v", gotMap[path], want)
	}
}

type imap map[string]interface{}

func TestIntegration_WatchQuery(t *testing.T) {
	ctx := context.Background()
	coll := integrationColl(t)
	h := testHelper{t}

	q := coll.Where("e", ">", 1).OrderBy("e", Asc)
	it := q.Snapshots(ctx)
	defer it.Stop()

	next := func() ([]*DocumentSnapshot, []DocumentChange) {
		qsnap, err := it.Next()
		if err != nil {
			t.Fatal(err)
		}
		if qsnap.ReadTime.IsZero() {
			t.Fatal("zero time")
		}
		ds, err := qsnap.Documents.GetAll()
		if err != nil {
			t.Fatal(err)
		}
		if qsnap.Size != len(ds) {
			t.Fatalf("Size=%d but we have %d docs", qsnap.Size, len(ds))
		}
		return ds, qsnap.Changes
	}

	copts := append([]cmp.Option{cmpopts.IgnoreFields(DocumentSnapshot{}, "ReadTime")}, cmpOpts...)
	check := func(msg string, wantd []*DocumentSnapshot, wantc []DocumentChange) {
		gotd, gotc := next()
		if diff := testutil.Diff(gotd, wantd, copts...); diff != "" {
			t.Errorf("%s: %s", msg, diff)
		}
		if diff := testutil.Diff(gotc, wantc, copts...); diff != "" {
			t.Errorf("%s: %s", msg, diff)
		}
	}

	check("initial", nil, nil)
	doc1 := coll.NewDoc()
	h.mustCreate(doc1, imap{"e": int64(2), "b": "two"})
	wds := h.mustGet(doc1)
	check("one",
		[]*DocumentSnapshot{wds},
		[]DocumentChange{{Kind: DocumentAdded, Doc: wds, OldIndex: -1, NewIndex: 0}})

	// Add a doc that does not match. We won't see a snapshot  for this.
	doc2 := coll.NewDoc()
	h.mustCreate(doc2, imap{"e": int64(1)})

	// Update the first doc. We should see the change. We won't see doc2.
	h.mustUpdate(doc1, []Update{{Path: "e", Value: int64(3)}})
	wds = h.mustGet(doc1)
	check("update",
		[]*DocumentSnapshot{wds},
		[]DocumentChange{{Kind: DocumentModified, Doc: wds, OldIndex: 0, NewIndex: 0}})

	// Now update doc so that it is not in the query. We should see a snapshot with no docs.
	h.mustUpdate(doc1, []Update{{Path: "e", Value: int64(0)}})
	check("update2", nil, []DocumentChange{{Kind: DocumentRemoved, Doc: wds, OldIndex: 0, NewIndex: -1}})

	// Add two docs out of order. We should see them in order.
	doc3 := coll.NewDoc()
	doc4 := coll.NewDoc()
	want3 := imap{"e": int64(5)}
	want4 := imap{"e": int64(4)}
	h.mustCreate(doc3, want3)
	h.mustCreate(doc4, want4)
	wds4 := h.mustGet(doc4)
	wds3 := h.mustGet(doc3)
	check("two#1",
		[]*DocumentSnapshot{wds3},
		[]DocumentChange{{Kind: DocumentAdded, Doc: wds3, OldIndex: -1, NewIndex: 0}})
	check("two#2",
		[]*DocumentSnapshot{wds4, wds3},
		[]DocumentChange{{Kind: DocumentAdded, Doc: wds4, OldIndex: -1, NewIndex: 0}})
	// Delete a doc.
	h.mustDelete(doc4)
	check("after del", []*DocumentSnapshot{wds3}, []DocumentChange{{Kind: DocumentRemoved, Doc: wds4, OldIndex: 0, NewIndex: -1}})
}

func TestIntegration_WatchQueryCancel(t *testing.T) {
	ctx := context.Background()
	coll := integrationColl(t)

	q := coll.Where("e", ">", 1).OrderBy("e", Asc)
	ctx, cancel := context.WithCancel(ctx)
	it := q.Snapshots(ctx)
	defer it.Stop()

	// First call opens the stream.
	_, err := it.Next()
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	_, err = it.Next()
	codeEq(t, "after cancel", codes.Canceled, err)
}

func TestIntegration_MissingDocs(t *testing.T) {
	ctx := context.Background()
	h := testHelper{t}
	client := integrationClient(t)
	coll := client.Collection(collectionIDs.New())
	dr1 := coll.NewDoc()
	dr2 := coll.NewDoc()
	dr3 := dr2.Collection("sub").NewDoc()
	h.mustCreate(dr1, integrationTestMap)
	defer h.mustDelete(dr1)
	h.mustCreate(dr3, integrationTestMap)
	defer h.mustDelete(dr3)

	// dr1 is a document in coll. dr2 was never created, but there are documents in
	// its sub-collections. It is "missing".
	// The Collection.DocumentRefs method includes missing document refs.
	want := []string{dr1.Path, dr2.Path}
	drs, err := coll.DocumentRefs(ctx).GetAll()
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, dr := range drs {
		got = append(got, dr.Path)
	}
	sort.Strings(want)
	sort.Strings(got)
	if !testutil.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func codeEq(t *testing.T, msg string, code codes.Code, err error) {
	if grpc.Code(err) != code {
		t.Fatalf("%s:\ngot <%v>\nwant code %s", msg, err, code)
	}
}

func loc() string {
	_, file, line, ok := runtime.Caller(2)
	if !ok {
		return "???"
	}
	return fmt.Sprintf("%s:%d", filepath.Base(file), line)
}

func copyMap(m map[string]interface{}) map[string]interface{} {
	c := map[string]interface{}{}
	for k, v := range m {
		c[k] = v
	}
	return c
}

func checkTimeBetween(t *testing.T, got, low, high time.Time) {
	// Allow slack for clock skew.
	const slack = 4 * time.Second
	low = low.Add(-slack)
	high = high.Add(slack)
	if got.Before(low) || got.After(high) {
		t.Fatalf("got %s, not in [%s, %s]", got, low, high)
	}
}

type testHelper struct {
	t *testing.T
}

func (h testHelper) mustCreate(doc *DocumentRef, data interface{}) *WriteResult {
	wr, err := doc.Create(context.Background(), data)
	if err != nil {
		h.t.Fatalf("%s: creating: %v", loc(), err)
	}
	return wr
}

func (h testHelper) mustUpdate(doc *DocumentRef, updates []Update) *WriteResult {
	wr, err := doc.Update(context.Background(), updates)
	if err != nil {
		h.t.Fatalf("%s: updating: %v", loc(), err)
	}
	return wr
}

func (h testHelper) mustGet(doc *DocumentRef) *DocumentSnapshot {
	d, err := doc.Get(context.Background())
	if err != nil {
		h.t.Fatalf("%s: getting: %v", loc(), err)
	}
	return d
}

func (h testHelper) mustDelete(doc *DocumentRef) *WriteResult {
	wr, err := doc.Delete(context.Background())
	if err != nil {
		h.t.Fatalf("%s: updating: %v", loc(), err)
	}
	return wr
}

func (h testHelper) mustSet(doc *DocumentRef, data interface{}, opts ...SetOption) *WriteResult {
	wr, err := doc.Set(context.Background(), data, opts...)
	if err != nil {
		h.t.Fatalf("%s: updating: %v", loc(), err)
	}
	return wr
}

func TestDetectProjectID(t *testing.T) {
	if testing.Short() {
		t.Skip("Integration tests skipped in short mode")
	}
	ctx := context.Background()

	creds := testutil.Credentials(ctx)
	if creds == nil {
		t.Skip("Integration tests skipped. See CONTRIBUTING.md for details")
	}

	// Use creds with project ID.
	if _, err := NewClient(ctx, DetectProjectID, option.WithCredentials(creds)); err != nil {
		t.Errorf("NewClient: %v", err)
	}

	ts := testutil.ErroringTokenSource{}
	// Try to use creds without project ID.
	_, err := NewClient(ctx, DetectProjectID, option.WithTokenSource(ts))
	if err == nil || err.Error() != "firestore: see the docs on DetectProjectID" {
		t.Errorf("expected an error while using TokenSource that does not have a project ID")
	}
}
