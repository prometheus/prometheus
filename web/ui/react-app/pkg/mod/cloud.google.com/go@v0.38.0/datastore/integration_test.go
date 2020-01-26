// Copyright 2014 Google LLC
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

package datastore

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/rpcreplay"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TODO(djd): Make test entity clean up more robust: some test entities may
// be left behind if tests are aborted, the transport fails, etc.

var timeNow = time.Now()

// suffix is a timestamp-based suffix which is appended to key names,
// particularly for the root keys of entity groups. This reduces flakiness
// when the tests are run in parallel.
var suffix string

const replayFilename = "datastore.replay"

type replayInfo struct {
	ProjectID string
	Time      time.Time
}

var (
	record = flag.Bool("record", false, "record RPCs")

	newTestClient = func(ctx context.Context, t *testing.T) *Client {
		return newClient(ctx, t, nil)
	}
)

func TestMain(m *testing.M) {
	os.Exit(testMain(m))
}

func testMain(m *testing.M) int {
	flag.Parse()
	if testing.Short() {
		if *record {
			log.Fatal("cannot combine -short and -record")
		}
		if testutil.CanReplay(replayFilename) {
			initReplay()
		}
	} else if *record {
		if testutil.ProjID() == "" {
			log.Fatal("must record with a project ID")
		}
		b, err := json.Marshal(replayInfo{
			ProjectID: testutil.ProjID(),
			Time:      timeNow,
		})
		if err != nil {
			log.Fatal(err)
		}
		rec, err := rpcreplay.NewRecorder(replayFilename, b)
		if err != nil {
			log.Fatal(err)
		}
		defer func() {
			if err := rec.Close(); err != nil {
				log.Fatalf("closing recorder: %v", err)
			}
		}()
		newTestClient = func(ctx context.Context, t *testing.T) *Client {
			return newClient(ctx, t, rec.DialOptions())
		}
		log.Printf("recording to %s", replayFilename)
	}
	suffix = fmt.Sprintf("-t%d", timeNow.UnixNano())
	return m.Run()
}

func initReplay() {
	rep, err := rpcreplay.NewReplayer(replayFilename)
	if err != nil {
		log.Fatal(err)
	}
	defer rep.Close()

	var ri replayInfo
	if err := json.Unmarshal(rep.Initial(), &ri); err != nil {
		log.Fatalf("unmarshaling initial replay info: %v", err)
	}
	timeNow = ri.Time.In(time.Local)

	conn, err := rep.Connection()
	if err != nil {
		log.Fatal(err)
	}
	newTestClient = func(ctx context.Context, t *testing.T) *Client {
		client, err := NewClient(ctx, ri.ProjectID, option.WithGRPCConn(conn))
		if err != nil {
			t.Fatalf("NewClient: %v", err)
		}
		return client
	}
	log.Printf("replaying from %s", replayFilename)
}

func newClient(ctx context.Context, t *testing.T, dialOpts []grpc.DialOption) *Client {
	if testing.Short() {
		t.Skip("Integration tests skipped in short mode")
	}
	ts := testutil.TokenSource(ctx, ScopeDatastore)
	if ts == nil {
		t.Skip("Integration tests skipped. See CONTRIBUTING.md for details")
	}
	opts := []option.ClientOption{option.WithTokenSource(ts)}
	for _, opt := range dialOpts {
		opts = append(opts, option.WithGRPCDialOption(opt))
	}
	client, err := NewClient(ctx, testutil.ProjID(), opts...)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return client
}

func TestBasics(t *testing.T) {
	ctx, _ := context.WithTimeout(context.Background(), time.Second*20)
	client := newTestClient(ctx, t)
	defer client.Close()

	type X struct {
		I int
		S string
		T time.Time
	}

	x0 := X{66, "99", timeNow.Truncate(time.Millisecond)}
	k, err := client.Put(ctx, IncompleteKey("BasicsX", nil), &x0)
	if err != nil {
		t.Fatalf("client.Put: %v", err)
	}
	x1 := X{}
	err = client.Get(ctx, k, &x1)
	if err != nil {
		t.Errorf("client.Get: %v", err)
	}
	err = client.Delete(ctx, k)
	if err != nil {
		t.Errorf("client.Delete: %v", err)
	}
	if !testutil.Equal(x0, x1) {
		t.Errorf("compare: x0=%v, x1=%v", x0, x1)
	}
}

func TestTopLevelKeyLoaded(t *testing.T) {
	ctx, _ := context.WithTimeout(context.Background(), time.Second*20)
	client := newTestClient(ctx, t)
	defer client.Close()

	completeKey := NameKey("EntityWithKey", "myent", nil)

	type EntityWithKey struct {
		I int
		S string
		K *Key `datastore:"__key__"`
	}

	in := &EntityWithKey{
		I: 12,
		S: "abcd",
	}

	k, err := client.Put(ctx, completeKey, in)
	if err != nil {
		t.Fatalf("client.Put: %v", err)
	}

	var e EntityWithKey
	err = client.Get(ctx, k, &e)
	if err != nil {
		t.Fatalf("client.Get: %v", err)
	}

	// The two keys should be absolutely identical.
	if !testutil.Equal(e.K, k) {
		t.Fatalf("e.K not equal to k; got %#v, want %#v", e.K, k)
	}

}

func TestListValues(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(ctx, t)
	defer client.Close()

	p0 := PropertyList{
		{Name: "L", Value: []interface{}{int64(12), "string", true}},
	}
	k, err := client.Put(ctx, IncompleteKey("ListValue", nil), &p0)
	if err != nil {
		t.Fatalf("client.Put: %v", err)
	}
	var p1 PropertyList
	if err := client.Get(ctx, k, &p1); err != nil {
		t.Errorf("client.Get: %v", err)
	}
	if !testutil.Equal(p0, p1) {
		t.Errorf("compare:\np0=%v\np1=%#v", p0, p1)
	}
	if err = client.Delete(ctx, k); err != nil {
		t.Errorf("client.Delete: %v", err)
	}
}

func TestGetMulti(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(ctx, t)
	defer client.Close()

	type X struct {
		I int
	}
	p := NameKey("X", "x"+suffix, nil)

	cases := []struct {
		key *Key
		put bool
	}{
		{key: NameKey("X", "item1", p), put: true},
		{key: NameKey("X", "item2", p), put: false},
		{key: NameKey("X", "item3", p), put: false},
		{key: NameKey("X", "item3", p), put: false},
		{key: NameKey("X", "item4", p), put: true},
	}

	var src, dst []*X
	var srcKeys, dstKeys []*Key
	for _, c := range cases {
		dst = append(dst, &X{})
		dstKeys = append(dstKeys, c.key)
		if c.put {
			src = append(src, &X{})
			srcKeys = append(srcKeys, c.key)
		}
	}
	if _, err := client.PutMulti(ctx, srcKeys, src); err != nil {
		t.Error(err)
	}
	err := client.GetMulti(ctx, dstKeys, dst)
	if err == nil {
		t.Errorf("client.GetMulti got %v, expected error", err)
	}
	e, ok := err.(MultiError)
	if !ok {
		t.Errorf("client.GetMulti got %T, expected MultiError", err)
	}
	for i, err := range e {
		got, want := err, (error)(nil)
		if !cases[i].put {
			got, want = err, ErrNoSuchEntity
		}
		if got != want {
			t.Errorf("MultiError[%d] == %v, want %v", i, got, want)
		}
	}
}

type Z struct {
	S string
	T string `datastore:",noindex"`
	P []byte
	K []byte `datastore:",noindex"`
}

func (z Z) String() string {
	var lens []string
	v := reflect.ValueOf(z)
	for i := 0; i < v.NumField(); i++ {
		if l := v.Field(i).Len(); l > 0 {
			lens = append(lens, fmt.Sprintf("len(%s)=%d", v.Type().Field(i).Name, l))
		}
	}
	return fmt.Sprintf("Z{ %s }", strings.Join(lens, ","))
}

func TestUnindexableValues(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(ctx, t)
	defer client.Close()

	x1500 := strings.Repeat("x", 1500)
	x1501 := strings.Repeat("x", 1501)
	testCases := []struct {
		in      Z
		wantErr bool
	}{
		{in: Z{S: x1500}, wantErr: false},
		{in: Z{S: x1501}, wantErr: true},
		{in: Z{T: x1500}, wantErr: false},
		{in: Z{T: x1501}, wantErr: false},
		{in: Z{P: []byte(x1500)}, wantErr: false},
		{in: Z{P: []byte(x1501)}, wantErr: true},
		{in: Z{K: []byte(x1500)}, wantErr: false},
		{in: Z{K: []byte(x1501)}, wantErr: false},
	}
	for _, tt := range testCases {
		_, err := client.Put(ctx, IncompleteKey("BasicsZ", nil), &tt.in)
		if (err != nil) != tt.wantErr {
			t.Errorf("client.Put %s got err %v, want err %t", tt.in, err, tt.wantErr)
		}
	}
}

func TestNilKey(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(ctx, t)
	defer client.Close()

	testCases := []struct {
		in      K0
		wantErr bool
	}{
		{in: K0{K: testKey0}, wantErr: false},
		{in: K0{}, wantErr: false},
	}
	for _, tt := range testCases {
		_, err := client.Put(ctx, IncompleteKey("NilKey", nil), &tt.in)
		if (err != nil) != tt.wantErr {
			t.Errorf("client.Put %s got err %v, want err %t", tt.in, err, tt.wantErr)
		}
	}
}

type SQChild struct {
	I, J int
	T, U int64
}

type SQTestCase struct {
	desc      string
	q         *Query
	wantCount int
	wantSum   int
}

func testSmallQueries(ctx context.Context, t *testing.T, client *Client, parent *Key, children []*SQChild,
	testCases []SQTestCase, extraTests ...func()) {
	keys := make([]*Key, len(children))
	for i := range keys {
		keys[i] = IncompleteKey("SQChild", parent)
	}
	keys, err := client.PutMulti(ctx, keys, children)
	if err != nil {
		t.Fatalf("client.PutMulti: %v", err)
	}
	defer func() {
		err := client.DeleteMulti(ctx, keys)
		if err != nil {
			t.Errorf("client.DeleteMulti: %v", err)
		}
	}()

	for _, tc := range testCases {
		count, err := client.Count(ctx, tc.q)
		if err != nil {
			t.Errorf("Count %q: %v", tc.desc, err)
			continue
		}
		if count != tc.wantCount {
			t.Errorf("Count %q: got %d want %d", tc.desc, count, tc.wantCount)
			continue
		}
	}

	for _, tc := range testCases {
		var got []SQChild
		_, err := client.GetAll(ctx, tc.q, &got)
		if err != nil {
			t.Errorf("client.GetAll %q: %v", tc.desc, err)
			continue
		}
		sum := 0
		for _, c := range got {
			sum += c.I + c.J
		}
		if sum != tc.wantSum {
			t.Errorf("sum %q: got %d want %d", tc.desc, sum, tc.wantSum)
			continue
		}
	}
	for _, x := range extraTests {
		x()
	}
}

func TestFilters(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(ctx, t)
	defer client.Close()

	parent := NameKey("SQParent", "TestFilters"+suffix, nil)
	now := timeNow.Truncate(time.Millisecond).Unix()
	children := []*SQChild{
		{I: 0, T: now, U: now},
		{I: 1, T: now, U: now},
		{I: 2, T: now, U: now},
		{I: 3, T: now, U: now},
		{I: 4, T: now, U: now},
		{I: 5, T: now, U: now},
		{I: 6, T: now, U: now},
		{I: 7, T: now, U: now},
	}
	baseQuery := NewQuery("SQChild").Ancestor(parent).Filter("T=", now)
	testSmallQueries(ctx, t, client, parent, children, []SQTestCase{
		{
			"I>1",
			baseQuery.Filter("I>", 1),
			6,
			2 + 3 + 4 + 5 + 6 + 7,
		},
		{
			"I>2 AND I<=5",
			baseQuery.Filter("I>", 2).Filter("I<=", 5),
			3,
			3 + 4 + 5,
		},
		{
			"I>=3 AND I<3",
			baseQuery.Filter("I>=", 3).Filter("I<", 3),
			0,
			0,
		},
		{
			"I=4",
			baseQuery.Filter("I=", 4),
			1,
			4,
		},
	}, func() {
		got := []*SQChild{}
		want := []*SQChild{
			{I: 0, T: now, U: now},
			{I: 1, T: now, U: now},
			{I: 2, T: now, U: now},
			{I: 3, T: now, U: now},
			{I: 4, T: now, U: now},
			{I: 5, T: now, U: now},
			{I: 6, T: now, U: now},
			{I: 7, T: now, U: now},
		}
		_, err := client.GetAll(ctx, baseQuery.Order("I"), &got)
		if err != nil {
			t.Errorf("client.GetAll: %v", err)
		}
		if !testutil.Equal(got, want) {
			t.Errorf("compare: got=%v, want=%v", got, want)
		}
	}, func() {
		got := []*SQChild{}
		want := []*SQChild{
			{I: 7, T: now, U: now},
			{I: 6, T: now, U: now},
			{I: 5, T: now, U: now},
			{I: 4, T: now, U: now},
			{I: 3, T: now, U: now},
			{I: 2, T: now, U: now},
			{I: 1, T: now, U: now},
			{I: 0, T: now, U: now},
		}
		_, err := client.GetAll(ctx, baseQuery.Order("-I"), &got)
		if err != nil {
			t.Errorf("client.GetAll: %v", err)
		}
		if !testutil.Equal(got, want) {
			t.Errorf("compare: got=%v, want=%v", got, want)
		}
	})
}

type ckey struct{}

func TestLargeQuery(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(ctx, t)
	defer client.Close()

	parent := NameKey("LQParent", "TestFilters"+suffix, nil)
	now := timeNow.Truncate(time.Millisecond).Unix()

	// Make a large number of children entities.
	const n = 800
	children := make([]*SQChild, 0, n)
	keys := make([]*Key, 0, n)
	for i := 0; i < n; i++ {
		children = append(children, &SQChild{I: i, T: now, U: now})
		keys = append(keys, IncompleteKey("SQChild", parent))
	}

	// Store using PutMulti in batches.
	const batchSize = 500
	for i := 0; i < n; i = i + 500 {
		j := i + batchSize
		if j > n {
			j = n
		}
		fullKeys, err := client.PutMulti(ctx, keys[i:j], children[i:j])
		if err != nil {
			t.Fatalf("PutMulti(%d, %d): %v", i, j, err)
		}
		defer func() {
			err := client.DeleteMulti(ctx, fullKeys)
			if err != nil {
				t.Errorf("client.DeleteMulti: %v", err)
			}
		}()
	}

	q := NewQuery("SQChild").Ancestor(parent).Filter("T=", now).Order("I")

	// Wait group to allow us to run query tests in parallel below.
	var wg sync.WaitGroup

	// Check we get the expected count and results for various limits/offsets.
	queryTests := []struct {
		limit, offset, want int
	}{
		// Just limit.
		{limit: 0, want: 0},
		{limit: 100, want: 100},
		{limit: 501, want: 501},
		{limit: n, want: n},
		{limit: n * 2, want: n},
		{limit: -1, want: n},
		// Just offset.
		{limit: -1, offset: 100, want: n - 100},
		{limit: -1, offset: 500, want: n - 500},
		{limit: -1, offset: n, want: 0},
		// Limit and offset.
		{limit: 100, offset: 100, want: 100},
		{limit: 1000, offset: 100, want: n - 100},
		{limit: 500, offset: 500, want: n - 500},
	}
	for _, tt := range queryTests {
		q := q.Limit(tt.limit).Offset(tt.offset)
		wg.Add(1)

		go func(limit, offset, want int) {
			defer wg.Done()
			// Check Count returns the expected number of results.
			count, err := client.Count(ctx, q)
			if err != nil {
				t.Errorf("client.Count(limit=%d offset=%d): %v", limit, offset, err)
				return
			}
			if count != want {
				t.Errorf("Count(limit=%d offset=%d) returned %d, want %d", limit, offset, count, want)
			}

			var got []SQChild
			_, err = client.GetAll(ctx, q, &got)
			if err != nil {
				t.Errorf("client.GetAll(limit=%d offset=%d): %v", limit, offset, err)
				return
			}
			if len(got) != want {
				t.Errorf("GetAll(limit=%d offset=%d) returned %d, want %d", limit, offset, len(got), want)
			}
			for i, child := range got {
				if got, want := child.I, i+offset; got != want {
					t.Errorf("GetAll(limit=%d offset=%d) got[%d].I == %d; want %d", limit, offset, i, got, want)
					break
				}
			}
		}(tt.limit, tt.offset, tt.want)
	}

	// Also check iterator cursor behaviour.
	cursorTests := []struct {
		limit, offset int // Query limit and offset.
		count         int // The number of times to call "next"
		want          int // The I value of the desired element, -1 for "Done".
	}{
		// No limits.
		{count: 0, limit: -1, want: 0},
		{count: 5, limit: -1, want: 5},
		{count: 500, limit: -1, want: 500},
		{count: 1000, limit: -1, want: -1}, // No more results.
		// Limits.
		{count: 5, limit: 5, want: 5},
		{count: 500, limit: 5, want: 5},
		{count: 1000, limit: 1000, want: -1}, // No more results.
		// Offsets.
		{count: 0, offset: 5, limit: -1, want: 5},
		{count: 5, offset: 5, limit: -1, want: 10},
		{count: 200, offset: 500, limit: -1, want: 700},
		{count: 200, offset: 1000, limit: -1, want: -1}, // No more results.
	}
	for _, tt := range cursorTests {
		wg.Add(1)

		go func(count, limit, offset, want int) {
			defer wg.Done()

			ctx := context.WithValue(ctx, ckey{}, fmt.Sprintf("c=%d,l=%d,o=%d", count, limit, offset))
			// Run iterator through count calls to Next.
			it := client.Run(ctx, q.Limit(limit).Offset(offset).KeysOnly())
			for i := 0; i < count; i++ {
				_, err := it.Next(nil)
				if err == iterator.Done {
					break
				}
				if err != nil {
					t.Errorf("count=%d, limit=%d, offset=%d: it.Next failed at i=%d", count, limit, offset, i)
					return
				}
			}

			// Grab the cursor.
			cursor, err := it.Cursor()
			if err != nil {
				t.Errorf("count=%d, limit=%d, offset=%d: it.Cursor: %v", count, limit, offset, err)
				return
			}

			// Make a request for the next element.
			it = client.Run(ctx, q.Limit(1).Start(cursor))
			var entity SQChild
			_, err = it.Next(&entity)
			switch {
			case want == -1:
				if err != iterator.Done {
					t.Errorf("count=%d, limit=%d, offset=%d: it.Next from cursor %v, want Done", count, limit, offset, err)
				}
			case err != nil:
				t.Errorf("count=%d, limit=%d, offset=%d: it.Next from cursor: %v, want nil", count, limit, offset, err)
			case entity.I != want:
				t.Errorf("count=%d, limit=%d, offset=%d: got.I = %d, want %d", count, limit, offset, entity.I, want)
			}
		}(tt.count, tt.limit, tt.offset, tt.want)
	}
	wg.Wait()
}

func TestEventualConsistency(t *testing.T) {
	// TODO(jba): either make this actually test eventual consistency, or
	// delete it. Currently it behaves the same with or without the
	// EventualConsistency call.
	ctx := context.Background()
	client := newTestClient(ctx, t)
	defer client.Close()

	parent := NameKey("SQParent", "TestEventualConsistency"+suffix, nil)
	now := timeNow.Truncate(time.Millisecond).Unix()
	children := []*SQChild{
		{I: 0, T: now, U: now},
		{I: 1, T: now, U: now},
		{I: 2, T: now, U: now},
	}
	query := NewQuery("SQChild").Ancestor(parent).Filter("T =", now).EventualConsistency()
	testSmallQueries(ctx, t, client, parent, children, nil, func() {
		got, err := client.Count(ctx, query)
		if err != nil {
			t.Fatalf("Count: %v", err)
		}
		if got < 0 || 3 < got {
			t.Errorf("Count: got %d, want [0,3]", got)
		}
	})
}

func TestProjection(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(ctx, t)
	defer client.Close()

	parent := NameKey("SQParent", "TestProjection"+suffix, nil)
	now := timeNow.Truncate(time.Millisecond).Unix()
	children := []*SQChild{
		{I: 1 << 0, J: 100, T: now, U: now},
		{I: 1 << 1, J: 100, T: now, U: now},
		{I: 1 << 2, J: 200, T: now, U: now},
		{I: 1 << 3, J: 300, T: now, U: now},
		{I: 1 << 4, J: 300, T: now, U: now},
	}
	baseQuery := NewQuery("SQChild").Ancestor(parent).Filter("T=", now).Filter("J>", 150)
	testSmallQueries(ctx, t, client, parent, children, []SQTestCase{
		{
			"project",
			baseQuery.Project("J"),
			3,
			200 + 300 + 300,
		},
		{
			"distinct",
			baseQuery.Project("J").Distinct(),
			2,
			200 + 300,
		},
		{
			"distinct on",
			baseQuery.Project("J").DistinctOn("J"),
			2,
			200 + 300,
		},
		{
			"project on meaningful (GD_WHEN) field",
			baseQuery.Project("U"),
			3,
			0,
		},
	})
}

func TestAllocateIDs(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(ctx, t)
	defer client.Close()

	keys := make([]*Key, 5)
	for i := range keys {
		keys[i] = IncompleteKey("AllocID", nil)
	}
	keys, err := client.AllocateIDs(ctx, keys)
	if err != nil {
		t.Errorf("AllocID #0 failed: %v", err)
	}
	if want := len(keys); want != 5 {
		t.Errorf("Expected to allocate 5 keys, %d keys are found", want)
	}
	for _, k := range keys {
		if k.Incomplete() {
			t.Errorf("Unexpeceted incomplete key found: %v", k)
		}
	}
}

func TestGetAllWithFieldMismatch(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(ctx, t)
	defer client.Close()

	type Fat struct {
		X, Y int
	}
	type Thin struct {
		X int
	}

	// Ancestor queries (those within an entity group) are strongly consistent
	// by default, which prevents a test from being flaky.
	// See https://cloud.google.com/appengine/docs/go/datastore/queries#Go_Data_consistency
	// for more information.
	parent := NameKey("SQParent", "TestGetAllWithFieldMismatch"+suffix, nil)
	putKeys := make([]*Key, 3)
	for i := range putKeys {
		putKeys[i] = IDKey("GetAllThing", int64(10+i), parent)
		_, err := client.Put(ctx, putKeys[i], &Fat{X: 20 + i, Y: 30 + i})
		if err != nil {
			t.Fatalf("client.Put: %v", err)
		}
	}

	var got []Thin
	want := []Thin{
		{X: 20},
		{X: 21},
		{X: 22},
	}
	getKeys, err := client.GetAll(ctx, NewQuery("GetAllThing").Ancestor(parent), &got)
	if len(getKeys) != 3 && !testutil.Equal(getKeys, putKeys) {
		t.Errorf("client.GetAll: keys differ\ngetKeys=%v\nputKeys=%v", getKeys, putKeys)
	}
	if !testutil.Equal(got, want) {
		t.Errorf("client.GetAll: entities differ\ngot =%v\nwant=%v", got, want)
	}
	if _, ok := err.(*ErrFieldMismatch); !ok {
		t.Errorf("client.GetAll: got err=%v, want ErrFieldMismatch", err)
	}
}

func TestKindlessQueries(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(ctx, t)
	defer client.Close()

	type Dee struct {
		I   int
		Why string
	}
	type Dum struct {
		I     int
		Pling string
	}

	parent := NameKey("Tweedle", "tweedle"+suffix, nil)

	keys := []*Key{
		NameKey("Dee", "dee0", parent),
		NameKey("Dum", "dum1", parent),
		NameKey("Dum", "dum2", parent),
		NameKey("Dum", "dum3", parent),
	}
	src := []interface{}{
		&Dee{1, "binary0001"},
		&Dum{2, "binary0010"},
		&Dum{4, "binary0100"},
		&Dum{8, "binary1000"},
	}
	keys, err := client.PutMulti(ctx, keys, src)
	if err != nil {
		t.Fatalf("put: %v", err)
	}

	testCases := []struct {
		desc    string
		query   *Query
		want    []int
		wantErr string
	}{
		{
			desc:  "Dee",
			query: NewQuery("Dee"),
			want:  []int{1},
		},
		{
			desc:  "Doh",
			query: NewQuery("Doh"),
			want:  nil},
		{
			desc:  "Dum",
			query: NewQuery("Dum"),
			want:  []int{2, 4, 8},
		},
		{
			desc:  "",
			query: NewQuery(""),
			want:  []int{1, 2, 4, 8},
		},
		{
			desc:  "Kindless filter",
			query: NewQuery("").Filter("__key__ =", keys[2]),
			want:  []int{4},
		},
		{
			desc:  "Kindless order",
			query: NewQuery("").Order("__key__"),
			want:  []int{1, 2, 4, 8},
		},
		{
			desc:    "Kindless bad filter",
			query:   NewQuery("").Filter("I =", 4),
			wantErr: "kind is required",
		},
		{
			desc:    "Kindless bad order",
			query:   NewQuery("").Order("-__key__"),
			wantErr: "kind is required for all orders except __key__ ascending",
		},
	}
loop:
	for _, tc := range testCases {
		q := tc.query.Ancestor(parent)
		gotCount, err := client.Count(ctx, q)
		if err != nil {
			if tc.wantErr == "" || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("count %q: err %v, want err %q", tc.desc, err, tc.wantErr)
			}
			continue
		}
		if tc.wantErr != "" {
			t.Errorf("count %q: want err %q", tc.desc, tc.wantErr)
			continue
		}
		if gotCount != len(tc.want) {
			t.Errorf("count %q: got %d want %d", tc.desc, gotCount, len(tc.want))
			continue
		}
		var got []int
		for iter := client.Run(ctx, q); ; {
			var dst struct {
				I          int
				Why, Pling string
			}
			_, err := iter.Next(&dst)
			if err == iterator.Done {
				break
			}
			if err != nil {
				t.Errorf("iter.Next %q: %v", tc.desc, err)
				continue loop
			}
			got = append(got, dst.I)
		}
		sort.Ints(got)
		if !testutil.Equal(got, tc.want) {
			t.Errorf("elems %q: got %+v want %+v", tc.desc, got, tc.want)
			continue
		}
	}
}

func TestTransaction(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(ctx, t)
	defer client.Close()

	type Counter struct {
		N int
		T time.Time
	}

	bangErr := errors.New("bang")
	tests := []struct {
		desc          string
		causeConflict []bool
		retErr        []error
		want          int
		wantErr       error
	}{
		{
			desc:          "3 attempts, no conflicts",
			causeConflict: []bool{false},
			retErr:        []error{nil},
			want:          11,
		},
		{
			desc:          "1 attempt, user error",
			causeConflict: []bool{false},
			retErr:        []error{bangErr},
			wantErr:       bangErr,
		},
		{
			desc:          "2 attempts, 1 conflict",
			causeConflict: []bool{true, false},
			retErr:        []error{nil, nil},
			want:          13, // Each conflict increments by 2.
		},
		{
			desc:          "3 attempts, 3 conflicts",
			causeConflict: []bool{true, true, true},
			retErr:        []error{nil, nil, nil},
			wantErr:       ErrConcurrentTransaction,
		},
	}

	for i, tt := range tests {
		// Put a new counter.
		c := &Counter{N: 10, T: timeNow}
		key, err := client.Put(ctx, IncompleteKey("TransCounter", nil), c)
		if err != nil {
			t.Errorf("%s: client.Put: %v", tt.desc, err)
			continue
		}
		defer client.Delete(ctx, key)

		// Increment the counter in a transaction.
		// The test case can manually cause a conflict or return an
		// error at each attempt.
		var attempts int
		_, err = client.RunInTransaction(ctx, func(tx *Transaction) error {
			attempts++
			if attempts > len(tt.causeConflict) {
				return fmt.Errorf("too many attempts. Got %d, max %d", attempts, len(tt.causeConflict))
			}

			var c Counter
			if err := tx.Get(key, &c); err != nil {
				return err
			}
			c.N++
			if _, err := tx.Put(key, &c); err != nil {
				return err
			}

			if tt.causeConflict[attempts-1] {
				c.N++
				if _, err := client.Put(ctx, key, &c); err != nil {
					return err
				}
			}

			return tt.retErr[attempts-1]
		}, MaxAttempts(i))

		// Check the error returned by RunInTransaction.
		if err != tt.wantErr {
			t.Errorf("%s: got err %v, want %v", tt.desc, err, tt.wantErr)
			continue
		}
		if err != nil {
			continue
		}

		// Check the final value of the counter.
		if err := client.Get(ctx, key, c); err != nil {
			t.Errorf("%s: client.Get: %v", tt.desc, err)
			continue
		}
		if c.N != tt.want {
			t.Errorf("%s: counter N=%d, want N=%d", tt.desc, c.N, tt.want)
		}
	}
}

func TestReadOnlyTransaction(t *testing.T) {
	if testing.Short() {
		t.Skip("Integration tests skipped in short mode")
	}
	ctx := context.Background()
	client := newClient(ctx, t, nil)
	defer client.Close()

	type value struct{ N int }

	// Put a value.
	const n = 5
	v := &value{N: n}
	key, err := client.Put(ctx, IncompleteKey("roTxn", nil), v)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Delete(ctx, key)

	// Read it from a read-only transaction.
	_, err = client.RunInTransaction(ctx, func(tx *Transaction) error {
		if err := tx.Get(key, v); err != nil {
			return err
		}
		return nil
	}, ReadOnly)
	if err != nil {
		t.Fatal(err)
	}
	if v.N != n {
		t.Fatalf("got %d, want %d", v.N, n)
	}

	// Attempting to write from a read-only transaction is an error.
	_, err = client.RunInTransaction(ctx, func(tx *Transaction) error {
		if _, err := tx.Put(key, v); err != nil {
			return err
		}
		return nil
	}, ReadOnly)
	if err == nil {
		t.Fatal("got nil, want error")
	}
}

func TestNilPointers(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(ctx, t)
	defer client.Close()

	type X struct {
		S string
	}

	src := []*X{{"zero"}, {"one"}}
	keys := []*Key{IncompleteKey("NilX", nil), IncompleteKey("NilX", nil)}
	keys, err := client.PutMulti(ctx, keys, src)
	if err != nil {
		t.Fatalf("PutMulti: %v", err)
	}

	// It's okay to store into a slice of nil *X.
	xs := make([]*X, 2)
	if err := client.GetMulti(ctx, keys, xs); err != nil {
		t.Errorf("GetMulti: %v", err)
	} else if !testutil.Equal(xs, src) {
		t.Errorf("GetMulti fetched %v, want %v", xs, src)
	}

	// It isn't okay to store into a single nil *X.
	var x0 *X
	if err, want := client.Get(ctx, keys[0], x0), ErrInvalidEntityType; err != want {
		t.Errorf("Get: err %v; want %v", err, want)
	}

	// Test that deleting with duplicate keys work.
	keys = append(keys, keys...)
	if err := client.DeleteMulti(ctx, keys); err != nil {
		t.Errorf("Delete: %v", err)
	}
}

func TestNestedRepeatedElementNoIndex(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(ctx, t)
	defer client.Close()

	type Inner struct {
		Name  string
		Value string `datastore:",noindex"`
	}
	type Outer struct {
		Config []Inner
	}
	m := &Outer{
		Config: []Inner{
			{Name: "short", Value: "a"},
			{Name: "long", Value: strings.Repeat("a", 2000)},
		},
	}

	key := NameKey("Nested", "Nested"+suffix, nil)
	if _, err := client.Put(ctx, key, m); err != nil {
		t.Fatalf("client.Put: %v", err)
	}
	if err := client.Delete(ctx, key); err != nil {
		t.Fatalf("client.Delete: %v", err)
	}
}

func TestPointerFields(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(ctx, t)
	defer client.Close()

	want := populatedPointers()
	key, err := client.Put(ctx, IncompleteKey("pointers", nil), want)
	if err != nil {
		t.Fatal(err)
	}
	var got Pointers
	if err := client.Get(ctx, key, &got); err != nil {
		t.Fatal(err)
	}
	if got.Pi == nil || *got.Pi != *want.Pi {
		t.Errorf("Pi: got %v, want %v", got.Pi, *want.Pi)
	}
	if got.Ps == nil || *got.Ps != *want.Ps {
		t.Errorf("Ps: got %v, want %v", got.Ps, *want.Ps)
	}
	if got.Pb == nil || *got.Pb != *want.Pb {
		t.Errorf("Pb: got %v, want %v", got.Pb, *want.Pb)
	}
	if got.Pf == nil || *got.Pf != *want.Pf {
		t.Errorf("Pf: got %v, want %v", got.Pf, *want.Pf)
	}
	if got.Pg == nil || *got.Pg != *want.Pg {
		t.Errorf("Pg: got %v, want %v", got.Pg, *want.Pg)
	}
	if got.Pt == nil || !got.Pt.Equal(*want.Pt) {
		t.Errorf("Pt: got %v, want %v", got.Pt, *want.Pt)
	}
}

func TestMutate(t *testing.T) {
	// test Client.Mutate
	testMutate(t, func(ctx context.Context, client *Client, muts ...*Mutation) ([]*Key, error) {
		return client.Mutate(ctx, muts...)
	})
	// test Transaction.Mutate
	testMutate(t, func(ctx context.Context, client *Client, muts ...*Mutation) ([]*Key, error) {
		var pkeys []*PendingKey
		commit, err := client.RunInTransaction(ctx, func(tx *Transaction) error {
			var err error
			pkeys, err = tx.Mutate(muts...)
			return err
		})
		if err != nil {
			return nil, err
		}
		var keys []*Key
		for _, pk := range pkeys {
			keys = append(keys, commit.Key(pk))
		}
		return keys, nil
	})
}

func testMutate(t *testing.T, mutate func(ctx context.Context, client *Client, muts ...*Mutation) ([]*Key, error)) {
	ctx := context.Background()
	client := newTestClient(ctx, t)
	defer client.Close()

	type T struct{ I int }

	check := func(k *Key, want interface{}) {
		var x T
		err := client.Get(ctx, k, &x)
		switch want := want.(type) {
		case error:
			if err != want {
				t.Errorf("key %s: got error %v, want %v", k, err, want)
			}
		case int:
			if err != nil {
				t.Fatalf("key %s: %v", k, err)
			}
			if x.I != want {
				t.Errorf("key %s: got %d, want %d", k, x.I, want)
			}
		default:
			panic("check: bad arg")
		}
	}

	keys, err := mutate(ctx, client,
		NewInsert(IncompleteKey("t", nil), &T{1}),
		NewUpsert(IncompleteKey("t", nil), &T{2}),
	)
	if err != nil {
		t.Fatal(err)
	}
	check(keys[0], 1)
	check(keys[1], 2)

	_, err = mutate(ctx, client,
		NewUpdate(keys[0], &T{3}),
		NewDelete(keys[1]),
	)
	if err != nil {
		t.Fatal(err)
	}
	check(keys[0], 3)
	check(keys[1], ErrNoSuchEntity)

	_, err = mutate(ctx, client, NewInsert(keys[0], &T{4}))
	if got, want := status.Code(err), codes.AlreadyExists; got != want {
		t.Errorf("Insert existing key: got %s, want %s", got, want)
	}

	_, err = mutate(ctx, client, NewUpdate(keys[1], &T{4}))
	if got, want := status.Code(err), codes.NotFound; got != want {
		t.Errorf("Update non-existing key: got %s, want %s", got, want)
	}
}

func TestDetectProjectID(t *testing.T) {
	if testing.Short() {
		t.Skip("Integration tests skipped in short mode")
	}
	ctx := context.Background()

	creds := testutil.Credentials(ctx, ScopeDatastore)
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
	if err == nil || err.Error() != "datastore: see the docs on DetectProjectID" {
		t.Errorf("expected an error while using TokenSource that does not have a project ID")
	}
}
