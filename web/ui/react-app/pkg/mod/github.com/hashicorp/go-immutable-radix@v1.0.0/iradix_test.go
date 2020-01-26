package iradix

import (
	"fmt"
	"reflect"
	"sort"
	"testing"

	"github.com/hashicorp/go-uuid"
)

func CopyTree(t *Tree) *Tree {
	nt := &Tree{
		root: CopyNode(t.root),
		size: t.size,
	}
	return nt
}

func CopyNode(n *Node) *Node {
	nn := &Node{}
	if n.mutateCh != nil {
		nn.mutateCh = n.mutateCh
	}
	if n.prefix != nil {
		nn.prefix = make([]byte, len(n.prefix))
		copy(nn.prefix, n.prefix)
	}
	if n.leaf != nil {
		nn.leaf = CopyLeaf(n.leaf)
	}
	if len(n.edges) != 0 {
		nn.edges = make([]edge, len(n.edges))
		for idx, edge := range n.edges {
			nn.edges[idx].label = edge.label
			nn.edges[idx].node = CopyNode(edge.node)
		}
	}
	return nn
}

func CopyLeaf(l *leafNode) *leafNode {
	ll := &leafNode{
		mutateCh: l.mutateCh,
		key:      l.key,
		val:      l.val,
	}
	return ll
}

func TestRadix_HugeTxn(t *testing.T) {
	r := New()

	// Insert way more nodes than the cache can fit
	txn1 := r.Txn()
	var expect []string
	for i := 0; i < defaultModifiedCache*100; i++ {
		gen, err := uuid.GenerateUUID()
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		txn1.Insert([]byte(gen), i)
		expect = append(expect, gen)
	}
	r = txn1.Commit()
	sort.Strings(expect)

	// Collect the output, should be sorted
	var out []string
	fn := func(k []byte, v interface{}) bool {
		out = append(out, string(k))
		return false
	}
	r.Root().Walk(fn)

	// Verify the match
	if len(out) != len(expect) {
		t.Fatalf("length mis-match: %d vs %d", len(out), len(expect))
	}
	for i := 0; i < len(out); i++ {
		if out[i] != expect[i] {
			t.Fatalf("mis-match: %v %v", out[i], expect[i])
		}
	}
}

func TestRadix(t *testing.T) {
	var min, max string
	inp := make(map[string]interface{})
	for i := 0; i < 1000; i++ {
		gen, err := uuid.GenerateUUID()
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		inp[gen] = i
		if gen < min || i == 0 {
			min = gen
		}
		if gen > max || i == 0 {
			max = gen
		}
	}

	r := New()
	rCopy := CopyTree(r)
	for k, v := range inp {
		newR, _, _ := r.Insert([]byte(k), v)
		if !reflect.DeepEqual(r, rCopy) {
			t.Errorf("r: %#v rc: %#v", r, rCopy)
			t.Errorf("r: %#v rc: %#v", r.root, rCopy.root)
			t.Fatalf("structure modified %d", newR.Len())
		}
		r = newR
		rCopy = CopyTree(r)
	}

	if r.Len() != len(inp) {
		t.Fatalf("bad length: %v %v", r.Len(), len(inp))
	}

	for k, v := range inp {
		out, ok := r.Get([]byte(k))
		if !ok {
			t.Fatalf("missing key: %v", k)
		}
		if out != v {
			t.Fatalf("value mis-match: %v %v", out, v)
		}
	}

	// Check min and max
	outMin, _, _ := r.Root().Minimum()
	if string(outMin) != min {
		t.Fatalf("bad minimum: %v %v", outMin, min)
	}
	outMax, _, _ := r.Root().Maximum()
	if string(outMax) != max {
		t.Fatalf("bad maximum: %v %v", outMax, max)
	}

	// Copy the full tree before delete
	orig := r
	origCopy := CopyTree(r)

	for k, v := range inp {
		tree, out, ok := r.Delete([]byte(k))
		r = tree
		if !ok {
			t.Fatalf("missing key: %v", k)
		}
		if out != v {
			t.Fatalf("value mis-match: %v %v", out, v)
		}
	}
	if r.Len() != 0 {
		t.Fatalf("bad length: %v", r.Len())
	}

	if !reflect.DeepEqual(orig, origCopy) {
		t.Fatalf("structure modified")
	}
}

func TestRoot(t *testing.T) {
	r := New()
	r, _, ok := r.Delete(nil)
	if ok {
		t.Fatalf("bad")
	}
	r, _, ok = r.Insert(nil, true)
	if ok {
		t.Fatalf("bad")
	}
	val, ok := r.Get(nil)
	if !ok || val != true {
		t.Fatalf("bad: %#v", val)
	}
	r, val, ok = r.Delete(nil)
	if !ok || val != true {
		t.Fatalf("bad: %v", val)
	}
}

func TestInsert_UpdateFeedback(t *testing.T) {
	r := New()
	txn1 := r.Txn()

	for i := 0; i < 10; i++ {
		var old interface{}
		var didUpdate bool
		old, didUpdate = txn1.Insert([]byte("helloworld"), i)
		if i == 0 {
			if old != nil || didUpdate {
				t.Fatalf("bad: %d %v %v", i, old, didUpdate)
			}
		} else {
			if old == nil || old.(int) != i-1 || !didUpdate {
				t.Fatalf("bad: %d %v %v", i, old, didUpdate)
			}
		}
	}
}

func TestDelete(t *testing.T) {
	r := New()
	s := []string{"", "A", "AB"}

	for _, ss := range s {
		r, _, _ = r.Insert([]byte(ss), true)
	}
	var ok bool
	for _, ss := range s {
		r, _, ok = r.Delete([]byte(ss))
		if !ok {
			t.Fatalf("bad %q", ss)
		}
	}
}

func TestDeletePrefix(t *testing.T) {

	type exp struct {
		desc        string
		treeNodes   []string
		prefix      string
		expectedOut []string
	}

	//various test cases where DeletePrefix should succeed
	cases := []exp{
		{
			"prefix not a node in tree",
			[]string{
				"",
				"test/test1",
				"test/test2",
				"test/test3",
				"R",
				"RA"},
			"test",
			[]string{
				"",
				"R",
				"RA",
			},
		},
		{
			"prefix matches a node in tree",
			[]string{
				"",
				"test",
				"test/test1",
				"test/test2",
				"test/test3",
				"test/testAAA",
				"R",
				"RA",
			},
			"test",
			[]string{
				"",
				"R",
				"RA",
			},
		},
		{
			"longer prefix, but prefix is not a node in tree",
			[]string{
				"",
				"test/test1",
				"test/test2",
				"test/test3",
				"test/testAAA",
				"R",
				"RA",
			},
			"test/test",
			[]string{
				"",
				"R",
				"RA",
			},
		},
		{
			"prefix only matches one node",
			[]string{
				"",
				"AB",
				"ABC",
				"AR",
				"R",
				"RA",
			},
			"AR",
			[]string{
				"",
				"AB",
				"ABC",
				"R",
				"RA",
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.desc, func(t *testing.T) {
			r := New()
			for _, ss := range testCase.treeNodes {
				r, _, _ = r.Insert([]byte(ss), true)
			}
			if got, want := r.Len(), len(testCase.treeNodes); got != want {
				t.Fatalf("Unexpected tree length after insert, got %d want %d ", got, want)
			}
			r, ok := r.DeletePrefix([]byte(testCase.prefix))
			if !ok {
				t.Fatalf("DeletePrefix should have returned true for tree %v, deleting prefix %v", testCase.treeNodes, testCase.prefix)
			}
			if got, want := r.Len(), len(testCase.expectedOut); got != want {
				t.Fatalf("Bad tree length, got %d want %d tree %v, deleting prefix %v ", got, want, testCase.treeNodes, testCase.prefix)
			}

			verifyTree(t, testCase.expectedOut, r)
			//Delete a non-existant node
			r, ok = r.DeletePrefix([]byte("CCCCC"))
			if ok {
				t.Fatalf("Expected DeletePrefix to return false ")
			}
			verifyTree(t, testCase.expectedOut, r)
		})
	}
}

func TestTrackMutate_DeletePrefix(t *testing.T) {

	r := New()

	keys := []string{
		"foo",
		"foo/bar/baz",
		"foo/baz/bar",
		"foo/zip/zap",
		"bazbaz",
		"zipzap",
	}
	for _, k := range keys {
		r, _, _ = r.Insert([]byte(k), nil)
	}
	if r.Len() != len(keys) {
		t.Fatalf("bad len: %v %v", r.Len(), len(keys))
	}

	rootWatch, _, _ := r.Root().GetWatch(nil)
	if rootWatch == nil {
		t.Fatalf("Should have returned a watch")
	}

	nodeWatch1, _, _ := r.Root().GetWatch([]byte("foo/bar/baz"))
	if nodeWatch1 == nil {
		t.Fatalf("Should have returned a watch")
	}

	nodeWatch2, _, _ := r.Root().GetWatch([]byte("foo/baz/bar"))
	if nodeWatch2 == nil {
		t.Fatalf("Should have returned a watch")
	}

	nodeWatch3, _, _ := r.Root().GetWatch([]byte("foo/zip/zap"))
	if nodeWatch3 == nil {
		t.Fatalf("Should have returned a watch")
	}

	unknownNodeWatch, _, _ := r.Root().GetWatch([]byte("bazbaz"))
	if unknownNodeWatch == nil {
		t.Fatalf("Should have returned a watch")
	}

	// Verify that deleting prefixes triggers the right set of watches
	txn := r.Txn()
	txn.TrackMutate(true)
	ok := txn.DeletePrefix([]byte("foo"))
	if !ok {
		t.Fatalf("Expected delete prefix to return true")
	}
	if hasAnyClosedMutateCh(r) {
		t.Fatalf("Transaction was not committed, no channel should have been closed")
	}

	txn.Commit()

	// Verify that all the leaf nodes we set up watches for above get triggered from the delete prefix call
	select {
	case <-rootWatch:
	default:
		t.Fatalf("root watch was not triggered")
	}
	select {
	case <-nodeWatch1:
	default:
		t.Fatalf("node watch was not triggered")
	}
	select {
	case <-nodeWatch2:
	default:
		t.Fatalf("node watch was not triggered")
	}
	select {
	case <-nodeWatch3:
	default:
		t.Fatalf("node watch was not triggered")
	}
	select {
	case <-unknownNodeWatch:
		t.Fatalf("Unrelated node watch was triggered during a prefix delete")
	default:
	}

}

func verifyTree(t *testing.T, expected []string, r *Tree) {
	root := r.Root()
	out := []string{}
	fn := func(k []byte, v interface{}) bool {
		out = append(out, string(k))
		return false
	}
	root.Walk(fn)

	if !reflect.DeepEqual(expected, out) {
		t.Fatalf("Unexpected contents of tree after delete prefix: expected %v, but got %v", expected, out)
	}
}

func TestLongestPrefix(t *testing.T) {
	r := New()

	keys := []string{
		"",
		"foo",
		"foobar",
		"foobarbaz",
		"foobarbazzip",
		"foozip",
	}
	for _, k := range keys {
		r, _, _ = r.Insert([]byte(k), nil)
	}
	if r.Len() != len(keys) {
		t.Fatalf("bad len: %v %v", r.Len(), len(keys))
	}

	type exp struct {
		inp string
		out string
	}
	cases := []exp{
		{"a", ""},
		{"abc", ""},
		{"fo", ""},
		{"foo", "foo"},
		{"foob", "foo"},
		{"foobar", "foobar"},
		{"foobarba", "foobar"},
		{"foobarbaz", "foobarbaz"},
		{"foobarbazzi", "foobarbaz"},
		{"foobarbazzip", "foobarbazzip"},
		{"foozi", "foo"},
		{"foozip", "foozip"},
		{"foozipzap", "foozip"},
	}
	root := r.Root()
	for _, test := range cases {
		m, _, ok := root.LongestPrefix([]byte(test.inp))
		if !ok {
			t.Fatalf("no match: %v", test)
		}
		if string(m) != test.out {
			t.Fatalf("mis-match: %v %v", m, test)
		}
	}
}

func TestWalkPrefix(t *testing.T) {
	r := New()

	keys := []string{
		"foobar",
		"foo/bar/baz",
		"foo/baz/bar",
		"foo/zip/zap",
		"zipzap",
	}
	for _, k := range keys {
		r, _, _ = r.Insert([]byte(k), nil)
	}
	if r.Len() != len(keys) {
		t.Fatalf("bad len: %v %v", r.Len(), len(keys))
	}

	type exp struct {
		inp string
		out []string
	}
	cases := []exp{
		exp{
			"f",
			[]string{"foobar", "foo/bar/baz", "foo/baz/bar", "foo/zip/zap"},
		},
		exp{
			"foo",
			[]string{"foobar", "foo/bar/baz", "foo/baz/bar", "foo/zip/zap"},
		},
		exp{
			"foob",
			[]string{"foobar"},
		},
		exp{
			"foo/",
			[]string{"foo/bar/baz", "foo/baz/bar", "foo/zip/zap"},
		},
		exp{
			"foo/b",
			[]string{"foo/bar/baz", "foo/baz/bar"},
		},
		exp{
			"foo/ba",
			[]string{"foo/bar/baz", "foo/baz/bar"},
		},
		exp{
			"foo/bar",
			[]string{"foo/bar/baz"},
		},
		exp{
			"foo/bar/baz",
			[]string{"foo/bar/baz"},
		},
		exp{
			"foo/bar/bazoo",
			[]string{},
		},
		exp{
			"z",
			[]string{"zipzap"},
		},
	}

	root := r.Root()
	for _, test := range cases {
		out := []string{}
		fn := func(k []byte, v interface{}) bool {
			out = append(out, string(k))
			return false
		}
		root.WalkPrefix([]byte(test.inp), fn)
		sort.Strings(out)
		sort.Strings(test.out)
		if !reflect.DeepEqual(out, test.out) {
			t.Fatalf("mis-match: %v %v", out, test.out)
		}
	}
}

func TestWalkPath(t *testing.T) {
	r := New()

	keys := []string{
		"foo",
		"foo/bar",
		"foo/bar/baz",
		"foo/baz/bar",
		"foo/zip/zap",
		"zipzap",
	}
	for _, k := range keys {
		r, _, _ = r.Insert([]byte(k), nil)
	}
	if r.Len() != len(keys) {
		t.Fatalf("bad len: %v %v", r.Len(), len(keys))
	}

	type exp struct {
		inp string
		out []string
	}
	cases := []exp{
		exp{
			"f",
			[]string{},
		},
		exp{
			"foo",
			[]string{"foo"},
		},
		exp{
			"foo/",
			[]string{"foo"},
		},
		exp{
			"foo/ba",
			[]string{"foo"},
		},
		exp{
			"foo/bar",
			[]string{"foo", "foo/bar"},
		},
		exp{
			"foo/bar/baz",
			[]string{"foo", "foo/bar", "foo/bar/baz"},
		},
		exp{
			"foo/bar/bazoo",
			[]string{"foo", "foo/bar", "foo/bar/baz"},
		},
		exp{
			"z",
			[]string{},
		},
	}

	root := r.Root()
	for _, test := range cases {
		out := []string{}
		fn := func(k []byte, v interface{}) bool {
			out = append(out, string(k))
			return false
		}
		root.WalkPath([]byte(test.inp), fn)
		sort.Strings(out)
		sort.Strings(test.out)
		if !reflect.DeepEqual(out, test.out) {
			t.Fatalf("mis-match: %v %v", out, test.out)
		}
	}
}

func TestIteratePrefix(t *testing.T) {
	r := New()

	keys := []string{
		"foo/bar/baz",
		"foo/baz/bar",
		"foo/zip/zap",
		"foobar",
		"zipzap",
	}
	for _, k := range keys {
		r, _, _ = r.Insert([]byte(k), nil)
	}
	if r.Len() != len(keys) {
		t.Fatalf("bad len: %v %v", r.Len(), len(keys))
	}

	type exp struct {
		inp string
		out []string
	}
	cases := []exp{
		exp{
			"",
			keys,
		},
		exp{
			"f",
			[]string{
				"foo/bar/baz",
				"foo/baz/bar",
				"foo/zip/zap",
				"foobar",
			},
		},
		exp{
			"foo",
			[]string{
				"foo/bar/baz",
				"foo/baz/bar",
				"foo/zip/zap",
				"foobar",
			},
		},
		exp{
			"foob",
			[]string{"foobar"},
		},
		exp{
			"foo/",
			[]string{"foo/bar/baz", "foo/baz/bar", "foo/zip/zap"},
		},
		exp{
			"foo/b",
			[]string{"foo/bar/baz", "foo/baz/bar"},
		},
		exp{
			"foo/ba",
			[]string{"foo/bar/baz", "foo/baz/bar"},
		},
		exp{
			"foo/bar",
			[]string{"foo/bar/baz"},
		},
		exp{
			"foo/bar/baz",
			[]string{"foo/bar/baz"},
		},
		exp{
			"foo/bar/bazoo",
			[]string{},
		},
		exp{
			"z",
			[]string{"zipzap"},
		},
	}

	root := r.Root()
	for idx, test := range cases {
		iter := root.Iterator()
		if test.inp != "" {
			iter.SeekPrefix([]byte(test.inp))
		}

		// Consume all the keys
		out := []string{}
		for {
			key, _, ok := iter.Next()
			if !ok {
				break
			}
			out = append(out, string(key))
		}
		if !reflect.DeepEqual(out, test.out) {
			t.Fatalf("mis-match: %d %v %v", idx, out, test.out)
		}
	}
}

func TestMergeChildNilEdges(t *testing.T) {
	r := New()
	r, _, _ = r.Insert([]byte("foobar"), 42)
	r, _, _ = r.Insert([]byte("foozip"), 43)
	r, _, _ = r.Delete([]byte("foobar"))

	root := r.Root()
	out := []string{}
	fn := func(k []byte, v interface{}) bool {
		out = append(out, string(k))
		return false
	}
	root.Walk(fn)

	expect := []string{"foozip"}
	sort.Strings(out)
	sort.Strings(expect)
	if !reflect.DeepEqual(out, expect) {
		t.Fatalf("mis-match: %v %v", out, expect)
	}
}

func TestMergeChildVisibility(t *testing.T) {
	r := New()
	r, _, _ = r.Insert([]byte("foobar"), 42)
	r, _, _ = r.Insert([]byte("foobaz"), 43)
	r, _, _ = r.Insert([]byte("foozip"), 10)

	txn1 := r.Txn()
	txn2 := r.Txn()

	// Ensure we get the expected value foobar and foobaz
	if val, ok := txn1.Get([]byte("foobar")); !ok || val != 42 {
		t.Fatalf("bad: %v", val)
	}
	if val, ok := txn1.Get([]byte("foobaz")); !ok || val != 43 {
		t.Fatalf("bad: %v", val)
	}
	if val, ok := txn2.Get([]byte("foobar")); !ok || val != 42 {
		t.Fatalf("bad: %v", val)
	}
	if val, ok := txn2.Get([]byte("foobaz")); !ok || val != 43 {
		t.Fatalf("bad: %v", val)
	}

	// Delete of foozip will cause a merge child between the
	// "foo" and "ba" nodes.
	if val, ok := txn2.Delete([]byte("foozip")); !ok || val != 10 {
		t.Fatalf("bad: %v", val)
	}

	// Insert of "foobaz" will update the slice of the "fooba" node
	// in-place to point to the new "foobaz" node. This in-place update
	// will cause the visibility of the update to leak into txn1 (prior
	// to the fix).
	if val, ok := txn2.Insert([]byte("foobaz"), 44); !ok || val != 43 {
		t.Fatalf("bad: %v", val)
	}

	// Ensure we get the expected value foobar and foobaz
	if val, ok := txn1.Get([]byte("foobar")); !ok || val != 42 {
		t.Fatalf("bad: %v", val)
	}
	if val, ok := txn1.Get([]byte("foobaz")); !ok || val != 43 {
		t.Fatalf("bad: %v", val)
	}
	if val, ok := txn2.Get([]byte("foobar")); !ok || val != 42 {
		t.Fatalf("bad: %v", val)
	}
	if val, ok := txn2.Get([]byte("foobaz")); !ok || val != 44 {
		t.Fatalf("bad: %v", val)
	}

	// Commit txn2
	r = txn2.Commit()

	// Ensure we get the expected value foobar and foobaz
	if val, ok := txn1.Get([]byte("foobar")); !ok || val != 42 {
		t.Fatalf("bad: %v", val)
	}
	if val, ok := txn1.Get([]byte("foobaz")); !ok || val != 43 {
		t.Fatalf("bad: %v", val)
	}
	if val, ok := r.Get([]byte("foobar")); !ok || val != 42 {
		t.Fatalf("bad: %v", val)
	}
	if val, ok := r.Get([]byte("foobaz")); !ok || val != 44 {
		t.Fatalf("bad: %v", val)
	}
}

// isClosed returns true if the given channel is closed.
func isClosed(ch chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

// hasAnyClosedMutateCh scans the given tree and returns true if there are any
// closed mutate channels on any nodes or leaves.
func hasAnyClosedMutateCh(r *Tree) bool {
	for iter := r.root.rawIterator(); iter.Front() != nil; iter.Next() {
		n := iter.Front()
		if isClosed(n.mutateCh) {
			return true
		}
		if n.isLeaf() && isClosed(n.leaf.mutateCh) {
			return true
		}
	}
	return false
}

func TestTrackMutate_SeekPrefixWatch(t *testing.T) {
	for i := 0; i < 3; i++ {
		r := New()

		keys := []string{
			"foo/bar/baz",
			"foo/baz/bar",
			"foo/zip/zap",
			"foobar",
			"zipzap",
		}
		for _, k := range keys {
			r, _, _ = r.Insert([]byte(k), nil)
		}
		if r.Len() != len(keys) {
			t.Fatalf("bad len: %v %v", r.Len(), len(keys))
		}

		iter := r.Root().Iterator()
		rootWatch := iter.SeekPrefixWatch([]byte("nope"))

		iter = r.Root().Iterator()
		parentWatch := iter.SeekPrefixWatch([]byte("foo"))

		iter = r.Root().Iterator()
		leafWatch := iter.SeekPrefixWatch([]byte("foobar"))

		iter = r.Root().Iterator()
		missingWatch := iter.SeekPrefixWatch([]byte("foobarbaz"))

		iter = r.Root().Iterator()
		otherWatch := iter.SeekPrefixWatch([]byte("foo/b"))

		// Write to a sub-child should trigger the leaf!
		txn := r.Txn()
		txn.TrackMutate(true)
		txn.Insert([]byte("foobarbaz"), nil)
		switch i {
		case 0:
			r = txn.Commit()
		case 1:
			r = txn.CommitOnly()
			txn.Notify()
		default:
			r = txn.CommitOnly()
			txn.slowNotify()
		}
		if hasAnyClosedMutateCh(r) {
			t.Fatalf("bad")
		}

		// Verify root and parent triggered, and leaf affected
		select {
		case <-rootWatch:
		default:
			t.Fatalf("bad")
		}
		select {
		case <-parentWatch:
		default:
			t.Fatalf("bad")
		}
		select {
		case <-leafWatch:
		default:
			t.Fatalf("bad")
		}
		select {
		case <-missingWatch:
		default:
			t.Fatalf("bad")
		}
		select {
		case <-otherWatch:
			t.Fatalf("bad")
		default:
		}

		iter = r.Root().Iterator()
		rootWatch = iter.SeekPrefixWatch([]byte("nope"))

		iter = r.Root().Iterator()
		parentWatch = iter.SeekPrefixWatch([]byte("foo"))

		iter = r.Root().Iterator()
		leafWatch = iter.SeekPrefixWatch([]byte("foobar"))

		iter = r.Root().Iterator()
		missingWatch = iter.SeekPrefixWatch([]byte("foobarbaz"))

		// Delete to a sub-child should trigger the leaf!
		txn = r.Txn()
		txn.TrackMutate(true)
		txn.Delete([]byte("foobarbaz"))
		switch i {
		case 0:
			r = txn.Commit()
		case 1:
			r = txn.CommitOnly()
			txn.Notify()
		default:
			r = txn.CommitOnly()
			txn.slowNotify()
		}
		if hasAnyClosedMutateCh(r) {
			t.Fatalf("bad")
		}

		// Verify root and parent triggered, and leaf affected
		select {
		case <-rootWatch:
		default:
			t.Fatalf("bad")
		}
		select {
		case <-parentWatch:
		default:
			t.Fatalf("bad")
		}
		select {
		case <-leafWatch:
		default:
			t.Fatalf("bad")
		}
		select {
		case <-missingWatch:
		default:
			t.Fatalf("bad")
		}
		select {
		case <-otherWatch:
			t.Fatalf("bad")
		default:
		}
	}
}

func TestTrackMutate_GetWatch(t *testing.T) {
	for i := 0; i < 3; i++ {
		r := New()

		keys := []string{
			"foo/bar/baz",
			"foo/baz/bar",
			"foo/zip/zap",
			"foobar",
			"zipzap",
		}
		for _, k := range keys {
			r, _, _ = r.Insert([]byte(k), nil)
		}
		if r.Len() != len(keys) {
			t.Fatalf("bad len: %v %v", r.Len(), len(keys))
		}

		rootWatch, _, ok := r.Root().GetWatch(nil)
		if rootWatch == nil {
			t.Fatalf("bad")
		}

		parentWatch, _, ok := r.Root().GetWatch([]byte("foo"))
		if parentWatch == nil {
			t.Fatalf("bad")
		}

		leafWatch, _, ok := r.Root().GetWatch([]byte("foobar"))
		if !ok {
			t.Fatalf("should be found")
		}
		if leafWatch == nil {
			t.Fatalf("bad")
		}

		otherWatch, _, ok := r.Root().GetWatch([]byte("foo/b"))
		if otherWatch == nil {
			t.Fatalf("bad")
		}

		// Write to a sub-child should not trigger the leaf!
		txn := r.Txn()
		txn.TrackMutate(true)
		txn.Insert([]byte("foobarbaz"), nil)
		switch i {
		case 0:
			r = txn.Commit()
		case 1:
			r = txn.CommitOnly()
			txn.Notify()
		default:
			r = txn.CommitOnly()
			txn.slowNotify()
		}
		if hasAnyClosedMutateCh(r) {
			t.Fatalf("bad")
		}

		// Verify root and parent triggered, not leaf affected
		select {
		case <-rootWatch:
		default:
			t.Fatalf("bad")
		}
		select {
		case <-parentWatch:
		default:
			t.Fatalf("bad")
		}
		select {
		case <-leafWatch:
			t.Fatalf("bad")
		default:
		}
		select {
		case <-otherWatch:
			t.Fatalf("bad")
		default:
		}

		// Setup new watchers
		rootWatch, _, ok = r.Root().GetWatch(nil)
		if rootWatch == nil {
			t.Fatalf("bad")
		}

		parentWatch, _, ok = r.Root().GetWatch([]byte("foo"))
		if parentWatch == nil {
			t.Fatalf("bad")
		}

		// Write to a exactly leaf should trigger the leaf!
		txn = r.Txn()
		txn.TrackMutate(true)
		txn.Insert([]byte("foobar"), nil)
		switch i {
		case 0:
			r = txn.Commit()
		case 1:
			r = txn.CommitOnly()
			txn.Notify()
		default:
			r = txn.CommitOnly()
			txn.slowNotify()
		}
		if hasAnyClosedMutateCh(r) {
			t.Fatalf("bad")
		}

		select {
		case <-rootWatch:
		default:
			t.Fatalf("bad")
		}
		select {
		case <-parentWatch:
		default:
			t.Fatalf("bad")
		}
		select {
		case <-leafWatch:
		default:
			t.Fatalf("bad")
		}
		select {
		case <-otherWatch:
			t.Fatalf("bad")
		default:
		}

		// Setup all the watchers again
		rootWatch, _, ok = r.Root().GetWatch(nil)
		if rootWatch == nil {
			t.Fatalf("bad")
		}

		parentWatch, _, ok = r.Root().GetWatch([]byte("foo"))
		if parentWatch == nil {
			t.Fatalf("bad")
		}

		leafWatch, _, ok = r.Root().GetWatch([]byte("foobar"))
		if !ok {
			t.Fatalf("should be found")
		}
		if leafWatch == nil {
			t.Fatalf("bad")
		}

		// Delete to a sub-child should not trigger the leaf!
		txn = r.Txn()
		txn.TrackMutate(true)
		txn.Delete([]byte("foobarbaz"))
		switch i {
		case 0:
			r = txn.Commit()
		case 1:
			r = txn.CommitOnly()
			txn.Notify()
		default:
			r = txn.CommitOnly()
			txn.slowNotify()
		}
		if hasAnyClosedMutateCh(r) {
			t.Fatalf("bad")
		}

		// Verify root and parent triggered, not leaf affected
		select {
		case <-rootWatch:
		default:
			t.Fatalf("bad")
		}
		select {
		case <-parentWatch:
		default:
			t.Fatalf("bad")
		}
		select {
		case <-leafWatch:
			t.Fatalf("bad")
		default:
		}
		select {
		case <-otherWatch:
			t.Fatalf("bad")
		default:
		}

		// Setup new watchers
		rootWatch, _, ok = r.Root().GetWatch(nil)
		if rootWatch == nil {
			t.Fatalf("bad")
		}

		parentWatch, _, ok = r.Root().GetWatch([]byte("foo"))
		if parentWatch == nil {
			t.Fatalf("bad")
		}

		// Write to a exactly leaf should trigger the leaf!
		txn = r.Txn()
		txn.TrackMutate(true)
		txn.Delete([]byte("foobar"))
		switch i {
		case 0:
			r = txn.Commit()
		case 1:
			r = txn.CommitOnly()
			txn.Notify()
		default:
			r = txn.CommitOnly()
			txn.slowNotify()
		}
		if hasAnyClosedMutateCh(r) {
			t.Fatalf("bad")
		}

		select {
		case <-rootWatch:
		default:
			t.Fatalf("bad")
		}
		select {
		case <-parentWatch:
		default:
			t.Fatalf("bad")
		}
		select {
		case <-leafWatch:
		default:
			t.Fatalf("bad")
		}
		select {
		case <-otherWatch:
			t.Fatalf("bad")
		default:
		}
	}
}

func TestTrackMutate_HugeTxn(t *testing.T) {
	r := New()

	keys := []string{
		"foo/bar/baz",
		"foo/baz/bar",
		"foo/zip/zap",
		"foobar",
		"nochange",
	}
	for i := 0; i < defaultModifiedCache; i++ {
		key := fmt.Sprintf("aaa%d", i)
		r, _, _ = r.Insert([]byte(key), nil)
	}
	for _, k := range keys {
		r, _, _ = r.Insert([]byte(k), nil)
	}
	for i := 0; i < defaultModifiedCache; i++ {
		key := fmt.Sprintf("zzz%d", i)
		r, _, _ = r.Insert([]byte(key), nil)
	}
	if r.Len() != len(keys)+2*defaultModifiedCache {
		t.Fatalf("bad len: %v %v", r.Len(), len(keys))
	}

	rootWatch, _, ok := r.Root().GetWatch(nil)
	if rootWatch == nil {
		t.Fatalf("bad")
	}

	parentWatch, _, ok := r.Root().GetWatch([]byte("foo"))
	if parentWatch == nil {
		t.Fatalf("bad")
	}

	leafWatch, _, ok := r.Root().GetWatch([]byte("foobar"))
	if !ok {
		t.Fatalf("should be found")
	}
	if leafWatch == nil {
		t.Fatalf("bad")
	}

	nopeWatch, _, ok := r.Root().GetWatch([]byte("nochange"))
	if !ok {
		t.Fatalf("should be found")
	}
	if nopeWatch == nil {
		t.Fatalf("bad")
	}

	beforeWatch, _, ok := r.Root().GetWatch([]byte("aaa123"))
	if beforeWatch == nil {
		t.Fatalf("bad")
	}

	afterWatch, _, ok := r.Root().GetWatch([]byte("zzz123"))
	if afterWatch == nil {
		t.Fatalf("bad")
	}

	// Start the transaction.
	txn := r.Txn()
	txn.TrackMutate(true)

	// Add new nodes on both sides of the tree and delete enough nodes to
	// overflow the tracking.
	txn.Insert([]byte("aaa"), nil)
	for i := 0; i < defaultModifiedCache; i++ {
		key := fmt.Sprintf("aaa%d", i)
		txn.Delete([]byte(key))
	}
	for i := 0; i < defaultModifiedCache; i++ {
		key := fmt.Sprintf("zzz%d", i)
		txn.Delete([]byte(key))
	}
	txn.Insert([]byte("zzz"), nil)

	// Hit the leaf, and add a child so we make multiple mutations to the
	// same node.
	txn.Insert([]byte("foobar"), nil)
	txn.Insert([]byte("foobarbaz"), nil)

	// Commit and make sure we overflowed but didn't take on extra stuff.
	r = txn.CommitOnly()
	if !txn.trackOverflow || txn.trackChannels != nil {
		t.Fatalf("bad")
	}

	// Now do the trigger.
	txn.Notify()

	// Make sure no closed channels escaped the transaction.
	if hasAnyClosedMutateCh(r) {
		t.Fatalf("bad")
	}

	// Verify the watches fired as expected.
	select {
	case <-rootWatch:
	default:
		t.Fatalf("bad")
	}
	select {
	case <-parentWatch:
	default:
		t.Fatalf("bad")
	}
	select {
	case <-leafWatch:
	default:
		t.Fatalf("bad")
	}
	select {
	case <-nopeWatch:
		t.Fatalf("bad")
	default:
	}
	select {
	case <-beforeWatch:
	default:
		t.Fatalf("bad")
	}
	select {
	case <-afterWatch:
	default:
		t.Fatalf("bad")
	}
}

func TestTrackMutate_mergeChild(t *testing.T) {
	// This case does a delete of the "acb" leaf, which causes the "aca"
	// leaf to get merged with the old "ac" node:
	//
	//    [root]                [root]
	//      |a                    |a
	//    [node]                [node]
	//   b/    \c              b/    \c
	//  (ab)  [node]          (ab)  (aca)
	//       a/    \b
	//     (aca)  (acb)
	//
	for i := 0; i < 3; i++ {
		r := New()
		r, _, _ = r.Insert([]byte("ab"), nil)
		r, _, _ = r.Insert([]byte("aca"), nil)
		r, _, _ = r.Insert([]byte("acb"), nil)
		snapIter := r.root.rawIterator()

		// Run through all notification methods as there were bugs in
		// both that affected these operations. The slowNotify path
		// would detect copied but otherwise identical leaves as changed
		// and wrongly close channels. The normal path would fail to
		// notify on a child node that had been merged.
		txn := r.Txn()
		txn.TrackMutate(true)
		txn.Delete([]byte("acb"))
		switch i {
		case 0:
			r = txn.Commit()
		case 1:
			r = txn.CommitOnly()
			txn.Notify()
		default:
			r = txn.CommitOnly()
			txn.slowNotify()
		}
		if hasAnyClosedMutateCh(r) {
			t.Fatalf("bad")
		}

		// Run through the old tree and make sure the exact channels we
		// expected were closed.
		for ; snapIter.Front() != nil; snapIter.Next() {
			n := snapIter.Front()
			path := snapIter.Path()
			switch path {
			case "", "a", "ac": // parent nodes all change
				if !isClosed(n.mutateCh) || n.leaf != nil {
					t.Fatalf("bad")
				}
			case "ab": // unrelated node / leaf sees no change
				if isClosed(n.mutateCh) || isClosed(n.leaf.mutateCh) {
					t.Fatalf("bad")
				}
			case "aca": // this node gets merged, but the leaf doesn't change
				if !isClosed(n.mutateCh) || isClosed(n.leaf.mutateCh) {
					t.Fatalf("bad")
				}
			case "acb": // this node / leaf gets deleted
				if !isClosed(n.mutateCh) || !isClosed(n.leaf.mutateCh) {
					t.Fatalf("bad")
				}
			default:
				t.Fatalf("bad: %s", path)
			}
		}
	}
}

func TestTrackMutate_cachedNodeChange(t *testing.T) {
	// This case does a delete of the "acb" leaf, which causes the "aca"
	// leaf to get merged with the old "ac" node:
	//
	//    [root]                [root]
	//      |a                    |a
	//    [node]                [node]
	//   b/    \c              b/    \c
	//  (ab)  [node]          (ab)  (aca*) <- this leaf gets modified
	//       a/    \b                         post-merge
	//     (aca)  (acb)
	//
	// Then it makes a modification to the "aca" leaf on a node that will
	// be in the cache, so this makes sure that the leaf watch fires.
	for i := 0; i < 3; i++ {
		r := New()
		r, _, _ = r.Insert([]byte("ab"), nil)
		r, _, _ = r.Insert([]byte("aca"), nil)
		r, _, _ = r.Insert([]byte("acb"), nil)
		snapIter := r.root.rawIterator()

		txn := r.Txn()
		txn.TrackMutate(true)
		txn.Delete([]byte("acb"))
		txn.Insert([]byte("aca"), nil)
		switch i {
		case 0:
			r = txn.Commit()
		case 1:
			r = txn.CommitOnly()
			txn.Notify()
		default:
			r = txn.CommitOnly()
			txn.slowNotify()
		}
		if hasAnyClosedMutateCh(r) {
			t.Fatalf("bad")
		}

		// Run through the old tree and make sure the exact channels we
		// expected were closed.
		for ; snapIter.Front() != nil; snapIter.Next() {
			n := snapIter.Front()
			path := snapIter.Path()
			switch path {
			case "", "a", "ac": // parent nodes all change
				if !isClosed(n.mutateCh) || n.leaf != nil {
					t.Fatalf("bad")
				}
			case "ab": // unrelated node / leaf sees no change
				if isClosed(n.mutateCh) || isClosed(n.leaf.mutateCh) {
					t.Fatalf("bad")
				}
			case "aca": // merge changes the node, then we update the leaf
				if !isClosed(n.mutateCh) || !isClosed(n.leaf.mutateCh) {
					t.Fatalf("bad")
				}
			case "acb": // this node / leaf gets deleted
				if !isClosed(n.mutateCh) || !isClosed(n.leaf.mutateCh) {
					t.Fatalf("bad")
				}
			default:
				t.Fatalf("bad: %s", path)
			}
		}
	}
}

func TestLenTxn(t *testing.T) {
	r := New()

	if r.Len() != 0 {
		t.Fatalf("not starting with empty tree")
	}

	txn := r.Txn()
	keys := []string{
		"foo/bar/baz",
		"foo/baz/bar",
		"foo/zip/zap",
		"foobar",
		"nochange",
	}
	for _, k := range keys {
		txn.Insert([]byte(k), nil)
	}
	r = txn.Commit()

	if r.Len() != len(keys) {
		t.Fatalf("bad: expected %d, got %d", len(keys), r.Len())
	}

	txn = r.Txn()
	for _, k := range keys {
		txn.Delete([]byte(k))
	}
	r = txn.Commit()

	if r.Len() != 0 {
		t.Fatalf("tree len should be zero, got %d", r.Len())
	}
}
