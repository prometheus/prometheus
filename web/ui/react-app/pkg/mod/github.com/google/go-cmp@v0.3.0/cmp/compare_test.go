// Copyright 2017, The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE.md file.

package cmp_test

import (
	"bytes"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/go-cmp/cmp/internal/flags"

	pb "github.com/google/go-cmp/cmp/internal/testprotos"
	ts "github.com/google/go-cmp/cmp/internal/teststructs"
)

func init() {
	flags.Deterministic = true
}

var now = time.Date(2009, time.November, 10, 23, 00, 00, 00, time.UTC)

func intPtr(n int) *int { return &n }

type test struct {
	label     string       // Test name
	x, y      interface{}  // Input values to compare
	opts      []cmp.Option // Input options
	wantDiff  string       // The exact difference string
	wantPanic string       // Sub-string of an expected panic message
	reason    string       // The reason for the expected outcome
}

func TestDiff(t *testing.T) {
	var tests []test
	tests = append(tests, comparerTests()...)
	tests = append(tests, transformerTests()...)
	tests = append(tests, embeddedTests()...)
	tests = append(tests, methodTests()...)
	tests = append(tests, project1Tests()...)
	tests = append(tests, project2Tests()...)
	tests = append(tests, project3Tests()...)
	tests = append(tests, project4Tests()...)

	for _, tt := range tests {
		tt := tt
		t.Run(tt.label, func(t *testing.T) {
			t.Parallel()
			var gotDiff, gotPanic string
			func() {
				defer func() {
					if ex := recover(); ex != nil {
						if s, ok := ex.(string); ok {
							gotPanic = s
						} else {
							panic(ex)
						}
					}
				}()
				gotDiff = cmp.Diff(tt.x, tt.y, tt.opts...)
			}()
			// TODO: Require every test case to provide a reason.
			if tt.wantPanic == "" {
				if gotPanic != "" {
					t.Fatalf("unexpected panic message: %s\nreason: %v", gotPanic, tt.reason)
				}
				tt.wantDiff = strings.TrimPrefix(tt.wantDiff, "\n")
				if gotDiff != tt.wantDiff {
					t.Fatalf("difference message:\ngot:\n%s\nwant:\n%s\nreason: %v", gotDiff, tt.wantDiff, tt.reason)
				}
			} else {
				if !strings.Contains(gotPanic, tt.wantPanic) {
					t.Fatalf("panic message:\ngot:  %s\nwant: %s\nreason: %v", gotPanic, tt.wantPanic, tt.reason)
				}
			}
		})
	}
}

func comparerTests() []test {
	const label = "Comparer"

	type Iface1 interface {
		Method()
	}
	type Iface2 interface {
		Method()
	}

	type tarHeader struct {
		Name       string
		Mode       int64
		Uid        int
		Gid        int
		Size       int64
		ModTime    time.Time
		Typeflag   byte
		Linkname   string
		Uname      string
		Gname      string
		Devmajor   int64
		Devminor   int64
		AccessTime time.Time
		ChangeTime time.Time
		Xattrs     map[string]string
	}

	makeTarHeaders := func(tf byte) (hs []tarHeader) {
		for i := 0; i < 5; i++ {
			hs = append(hs, tarHeader{
				Name: fmt.Sprintf("some/dummy/test/file%d", i),
				Mode: 0664, Uid: i * 1000, Gid: i * 1000, Size: 1 << uint(i),
				ModTime: now.Add(time.Duration(i) * time.Hour),
				Uname:   "user", Gname: "group",
				Typeflag: tf,
			})
		}
		return hs
	}

	return []test{{
		label: label,
		x:     nil,
		y:     nil,
	}, {
		label: label,
		x:     1,
		y:     1,
	}, {
		label:     label,
		x:         1,
		y:         1,
		opts:      []cmp.Option{cmp.Ignore()},
		wantPanic: "cannot use an unfiltered option",
	}, {
		label:     label,
		x:         1,
		y:         1,
		opts:      []cmp.Option{cmp.Comparer(func(_, _ interface{}) bool { return true })},
		wantPanic: "cannot use an unfiltered option",
	}, {
		label:     label,
		x:         1,
		y:         1,
		opts:      []cmp.Option{cmp.Transformer("λ", func(x interface{}) interface{} { return x })},
		wantPanic: "cannot use an unfiltered option",
	}, {
		label: label,
		x:     1,
		y:     1,
		opts: []cmp.Option{
			cmp.Comparer(func(x, y int) bool { return true }),
			cmp.Transformer("λ", func(x int) float64 { return float64(x) }),
		},
		wantPanic: "ambiguous set of applicable options",
	}, {
		label: label,
		x:     1,
		y:     1,
		opts: []cmp.Option{
			cmp.FilterPath(func(p cmp.Path) bool {
				return len(p) > 0 && p[len(p)-1].Type().Kind() == reflect.Int
			}, cmp.Options{cmp.Ignore(), cmp.Ignore(), cmp.Ignore()}),
			cmp.Comparer(func(x, y int) bool { return true }),
			cmp.Transformer("λ", func(x int) float64 { return float64(x) }),
		},
	}, {
		label:     label,
		opts:      []cmp.Option{struct{ cmp.Option }{}},
		wantPanic: "unknown option",
	}, {
		label: label,
		x:     struct{ A, B, C int }{1, 2, 3},
		y:     struct{ A, B, C int }{1, 2, 3},
	}, {
		label: label,
		x:     struct{ A, B, C int }{1, 2, 3},
		y:     struct{ A, B, C int }{1, 2, 4},
		wantDiff: `
  struct{ A int; B int; C int }{
  	A: 1,
  	B: 2,
- 	C: 3,
+ 	C: 4,
  }
`,
	}, {
		label:     label,
		x:         struct{ a, b, c int }{1, 2, 3},
		y:         struct{ a, b, c int }{1, 2, 4},
		wantPanic: "cannot handle unexported field",
	}, {
		label: label,
		x:     &struct{ A *int }{intPtr(4)},
		y:     &struct{ A *int }{intPtr(4)},
	}, {
		label: label,
		x:     &struct{ A *int }{intPtr(4)},
		y:     &struct{ A *int }{intPtr(5)},
		wantDiff: `
  &struct{ A *int }{
- 	A: &4,
+ 	A: &5,
  }
`,
	}, {
		label: label,
		x:     &struct{ A *int }{intPtr(4)},
		y:     &struct{ A *int }{intPtr(5)},
		opts: []cmp.Option{
			cmp.Comparer(func(x, y int) bool { return true }),
		},
	}, {
		label: label,
		x:     &struct{ A *int }{intPtr(4)},
		y:     &struct{ A *int }{intPtr(5)},
		opts: []cmp.Option{
			cmp.Comparer(func(x, y *int) bool { return x != nil && y != nil }),
		},
	}, {
		label: label,
		x:     &struct{ R *bytes.Buffer }{},
		y:     &struct{ R *bytes.Buffer }{},
	}, {
		label: label,
		x:     &struct{ R *bytes.Buffer }{new(bytes.Buffer)},
		y:     &struct{ R *bytes.Buffer }{},
		wantDiff: `
  &struct{ R *bytes.Buffer }{
- 	R: s"",
+ 	R: nil,
  }
`,
	}, {
		label: label,
		x:     &struct{ R *bytes.Buffer }{new(bytes.Buffer)},
		y:     &struct{ R *bytes.Buffer }{},
		opts: []cmp.Option{
			cmp.Comparer(func(x, y io.Reader) bool { return true }),
		},
	}, {
		label:     label,
		x:         &struct{ R bytes.Buffer }{},
		y:         &struct{ R bytes.Buffer }{},
		wantPanic: "cannot handle unexported field",
	}, {
		label: label,
		x:     &struct{ R bytes.Buffer }{},
		y:     &struct{ R bytes.Buffer }{},
		opts: []cmp.Option{
			cmp.Comparer(func(x, y io.Reader) bool { return true }),
		},
		wantPanic: "cannot handle unexported field",
	}, {
		label: label,
		x:     &struct{ R bytes.Buffer }{},
		y:     &struct{ R bytes.Buffer }{},
		opts: []cmp.Option{
			cmp.Transformer("Ref", func(x bytes.Buffer) *bytes.Buffer { return &x }),
			cmp.Comparer(func(x, y io.Reader) bool { return true }),
		},
	}, {
		label:     label,
		x:         []*regexp.Regexp{nil, regexp.MustCompile("a*b*c*")},
		y:         []*regexp.Regexp{nil, regexp.MustCompile("a*b*c*")},
		wantPanic: "cannot handle unexported field",
	}, {
		label: label,
		x:     []*regexp.Regexp{nil, regexp.MustCompile("a*b*c*")},
		y:     []*regexp.Regexp{nil, regexp.MustCompile("a*b*c*")},
		opts: []cmp.Option{cmp.Comparer(func(x, y *regexp.Regexp) bool {
			if x == nil || y == nil {
				return x == nil && y == nil
			}
			return x.String() == y.String()
		})},
	}, {
		label: label,
		x:     []*regexp.Regexp{nil, regexp.MustCompile("a*b*c*")},
		y:     []*regexp.Regexp{nil, regexp.MustCompile("a*b*d*")},
		opts: []cmp.Option{cmp.Comparer(func(x, y *regexp.Regexp) bool {
			if x == nil || y == nil {
				return x == nil && y == nil
			}
			return x.String() == y.String()
		})},
		wantDiff: `
  []*regexp.Regexp{
  	nil,
- 	s"a*b*c*",
+ 	s"a*b*d*",
  }
`,
	}, {
		label: label,
		x: func() ***int {
			a := 0
			b := &a
			c := &b
			return &c
		}(),
		y: func() ***int {
			a := 0
			b := &a
			c := &b
			return &c
		}(),
	}, {
		label: label,
		x: func() ***int {
			a := 0
			b := &a
			c := &b
			return &c
		}(),
		y: func() ***int {
			a := 1
			b := &a
			c := &b
			return &c
		}(),
		wantDiff: `
  &&&int(
- 	0,
+ 	1,
  )
`,
	}, {
		label: label,
		x:     []int{1, 2, 3, 4, 5}[:3],
		y:     []int{1, 2, 3},
	}, {
		label: label,
		x:     struct{ fmt.Stringer }{bytes.NewBufferString("hello")},
		y:     struct{ fmt.Stringer }{regexp.MustCompile("hello")},
		opts:  []cmp.Option{cmp.Comparer(func(x, y fmt.Stringer) bool { return x.String() == y.String() })},
	}, {
		label: label,
		x:     struct{ fmt.Stringer }{bytes.NewBufferString("hello")},
		y:     struct{ fmt.Stringer }{regexp.MustCompile("hello2")},
		opts:  []cmp.Option{cmp.Comparer(func(x, y fmt.Stringer) bool { return x.String() == y.String() })},
		wantDiff: `
  struct{ fmt.Stringer }(
- 	s"hello",
+ 	s"hello2",
  )
`,
	}, {
		label: label,
		x:     md5.Sum([]byte{'a'}),
		y:     md5.Sum([]byte{'b'}),
		wantDiff: `
  [16]uint8{
- 	0x0c, 0xc1, 0x75, 0xb9, 0xc0, 0xf1, 0xb6, 0xa8, 0x31, 0xc3, 0x99, 0xe2, 0x69, 0x77, 0x26, 0x61,
+ 	0x92, 0xeb, 0x5f, 0xfe, 0xe6, 0xae, 0x2f, 0xec, 0x3a, 0xd7, 0x1c, 0x77, 0x75, 0x31, 0x57, 0x8f,
  }
`,
	}, {
		label: label,
		x:     new(fmt.Stringer),
		y:     nil,
		wantDiff: `
  interface{}(
- 	&fmt.Stringer(nil),
  )
`,
	}, {
		label: label,
		x:     makeTarHeaders('0'),
		y:     makeTarHeaders('\x00'),
		wantDiff: `
  []cmp_test.tarHeader{
  	{
  		... // 4 identical fields
  		Size:     1,
  		ModTime:  s"2009-11-10 23:00:00 +0000 UTC",
- 		Typeflag: 0x30,
+ 		Typeflag: 0x00,
  		Linkname: "",
  		Uname:    "user",
  		... // 6 identical fields
  	},
  	{
  		... // 4 identical fields
  		Size:     2,
  		ModTime:  s"2009-11-11 00:00:00 +0000 UTC",
- 		Typeflag: 0x30,
+ 		Typeflag: 0x00,
  		Linkname: "",
  		Uname:    "user",
  		... // 6 identical fields
  	},
  	{
  		... // 4 identical fields
  		Size:     4,
  		ModTime:  s"2009-11-11 01:00:00 +0000 UTC",
- 		Typeflag: 0x30,
+ 		Typeflag: 0x00,
  		Linkname: "",
  		Uname:    "user",
  		... // 6 identical fields
  	},
  	{
  		... // 4 identical fields
  		Size:     8,
  		ModTime:  s"2009-11-11 02:00:00 +0000 UTC",
- 		Typeflag: 0x30,
+ 		Typeflag: 0x00,
  		Linkname: "",
  		Uname:    "user",
  		... // 6 identical fields
  	},
  	{
  		... // 4 identical fields
  		Size:     16,
  		ModTime:  s"2009-11-11 03:00:00 +0000 UTC",
- 		Typeflag: 0x30,
+ 		Typeflag: 0x00,
  		Linkname: "",
  		Uname:    "user",
  		... // 6 identical fields
  	},
  }
`,
	}, {
		label: label,
		x:     make([]int, 1000),
		y:     make([]int, 1000),
		opts: []cmp.Option{
			cmp.Comparer(func(_, _ int) bool {
				return rand.Intn(2) == 0
			}),
		},
		wantPanic: "non-deterministic or non-symmetric function detected",
	}, {
		label: label,
		x:     make([]int, 1000),
		y:     make([]int, 1000),
		opts: []cmp.Option{
			cmp.FilterValues(func(_, _ int) bool {
				return rand.Intn(2) == 0
			}, cmp.Ignore()),
		},
		wantPanic: "non-deterministic or non-symmetric function detected",
	}, {
		label: label,
		x:     []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
		y:     []int{10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
		opts: []cmp.Option{
			cmp.Comparer(func(x, y int) bool {
				return x < y
			}),
		},
		wantPanic: "non-deterministic or non-symmetric function detected",
	}, {
		label: label,
		x:     make([]string, 1000),
		y:     make([]string, 1000),
		opts: []cmp.Option{
			cmp.Transformer("λ", func(x string) int {
				return rand.Int()
			}),
		},
		wantPanic: "non-deterministic function detected",
	}, {
		// Make sure the dynamic checks don't raise a false positive for
		// non-reflexive comparisons.
		label: label,
		x:     make([]int, 10),
		y:     make([]int, 10),
		opts: []cmp.Option{
			cmp.Transformer("λ", func(x int) float64 {
				return math.NaN()
			}),
		},
		wantDiff: `
  []int{
- 	Inverse(λ, float64(NaN)),
+ 	Inverse(λ, float64(NaN)),
- 	Inverse(λ, float64(NaN)),
+ 	Inverse(λ, float64(NaN)),
- 	Inverse(λ, float64(NaN)),
+ 	Inverse(λ, float64(NaN)),
- 	Inverse(λ, float64(NaN)),
+ 	Inverse(λ, float64(NaN)),
- 	Inverse(λ, float64(NaN)),
+ 	Inverse(λ, float64(NaN)),
- 	Inverse(λ, float64(NaN)),
+ 	Inverse(λ, float64(NaN)),
- 	Inverse(λ, float64(NaN)),
+ 	Inverse(λ, float64(NaN)),
- 	Inverse(λ, float64(NaN)),
+ 	Inverse(λ, float64(NaN)),
- 	Inverse(λ, float64(NaN)),
+ 	Inverse(λ, float64(NaN)),
- 	Inverse(λ, float64(NaN)),
+ 	Inverse(λ, float64(NaN)),
  }
`,
	}, {
		// Ensure reasonable Stringer formatting of map keys.
		label: label,
		x:     map[*pb.Stringer]*pb.Stringer{{"hello"}: {"world"}},
		y:     map[*pb.Stringer]*pb.Stringer(nil),
		wantDiff: `
  map[*testprotos.Stringer]*testprotos.Stringer(
- 	{⟪0xdeadf00f⟫: s"world"},
+ 	nil,
  )
`,
	}, {
		// Ensure Stringer avoids double-quote escaping if possible.
		label: label,
		x:     []*pb.Stringer{{`multi\nline\nline\nline`}},
		wantDiff: strings.Replace(`
  interface{}(
- 	[]*testprotos.Stringer{s'multi\nline\nline\nline'},
  )
`, "'", "`", -1),
	}, {
		label: label,
		x:     struct{ I Iface2 }{},
		y:     struct{ I Iface2 }{},
		opts: []cmp.Option{
			cmp.Comparer(func(x, y Iface1) bool {
				return x == nil && y == nil
			}),
		},
	}, {
		label: label,
		x:     struct{ I Iface2 }{},
		y:     struct{ I Iface2 }{},
		opts: []cmp.Option{
			cmp.Transformer("λ", func(v Iface1) bool {
				return v == nil
			}),
		},
	}, {
		label: label,
		x:     struct{ I Iface2 }{},
		y:     struct{ I Iface2 }{},
		opts: []cmp.Option{
			cmp.FilterValues(func(x, y Iface1) bool {
				return x == nil && y == nil
			}, cmp.Ignore()),
		},
	}, {
		label: label,
		x:     []interface{}{map[string]interface{}{"avg": 0.278, "hr": 65, "name": "Mark McGwire"}, map[string]interface{}{"avg": 0.288, "hr": 63, "name": "Sammy Sosa"}},
		y:     []interface{}{map[string]interface{}{"avg": 0.278, "hr": 65.0, "name": "Mark McGwire"}, map[string]interface{}{"avg": 0.288, "hr": 63.0, "name": "Sammy Sosa"}},
		wantDiff: `
  []interface{}{
  	map[string]interface{}{
  		"avg":  float64(0.278),
- 		"hr":   int(65),
+ 		"hr":   float64(65),
  		"name": string("Mark McGwire"),
  	},
  	map[string]interface{}{
  		"avg":  float64(0.288),
- 		"hr":   int(63),
+ 		"hr":   float64(63),
  		"name": string("Sammy Sosa"),
  	},
  }
`,
	}, {
		label: label,
		x: map[*int]string{
			new(int): "hello",
		},
		y: map[*int]string{
			new(int): "world",
		},
		wantDiff: `
  map[*int]string{
- 	⟪0xdeadf00f⟫: "hello",
+ 	⟪0xdeadf00f⟫: "world",
  }
`,
	}, {
		label: label,
		x:     intPtr(0),
		y:     intPtr(0),
		opts: []cmp.Option{
			cmp.Comparer(func(x, y *int) bool { return x == y }),
		},
		// TODO: This output is unhelpful and should show the address.
		wantDiff: `
  (*int)(
- 	&0,
+ 	&0,
  )
`,
	}, {
		label: label,
		x: [2][]int{
			{0, 0, 0, 1, 2, 3, 0, 0, 4, 5, 6, 7, 8, 0, 9, 0, 0},
			{0, 1, 0, 0, 0, 20},
		},
		y: [2][]int{
			{1, 2, 3, 0, 4, 5, 6, 7, 0, 8, 9, 0, 0, 0},
			{0, 0, 1, 2, 0, 0, 0},
		},
		opts: []cmp.Option{
			cmp.FilterPath(func(p cmp.Path) bool {
				vx, vy := p.Last().Values()
				if vx.IsValid() && vx.Kind() == reflect.Int && vx.Int() == 0 {
					return true
				}
				if vy.IsValid() && vy.Kind() == reflect.Int && vy.Int() == 0 {
					return true
				}
				return false
			}, cmp.Ignore()),
		},
		wantDiff: `
  [2][]int{
  	{..., 1, 2, 3, ..., 4, 5, 6, 7, ..., 8, ..., 9, ...},
  	{
  		... // 6 ignored and 1 identical elements
- 		20,
+ 		2,
  		... // 3 ignored elements
  	},
  }
`,
		reason: "all zero slice elements are ignored (even if missing)",
	}, {
		label: label,
		x: [2]map[string]int{
			{"ignore1": 0, "ignore2": 0, "keep1": 1, "keep2": 2, "KEEP3": 3, "IGNORE3": 0},
			{"keep1": 1, "ignore1": 0},
		},
		y: [2]map[string]int{
			{"ignore1": 0, "ignore3": 0, "ignore4": 0, "keep1": 1, "keep2": 2, "KEEP3": 3},
			{"keep1": 1, "keep2": 2, "ignore2": 0},
		},
		opts: []cmp.Option{
			cmp.FilterPath(func(p cmp.Path) bool {
				vx, vy := p.Last().Values()
				if vx.IsValid() && vx.Kind() == reflect.Int && vx.Int() == 0 {
					return true
				}
				if vy.IsValid() && vy.Kind() == reflect.Int && vy.Int() == 0 {
					return true
				}
				return false
			}, cmp.Ignore()),
		},
		wantDiff: `
  [2]map[string]int{
  	{"KEEP3": 3, "keep1": 1, "keep2": 2, ...},
  	{
  		... // 2 ignored entries
  		"keep1": 1,
+ 		"keep2": 2,
  	},
  }
`,
		reason: "all zero map entries are ignored (even if missing)",
	}}
}

func transformerTests() []test {
	type StringBytes struct {
		String string
		Bytes  []byte
	}

	const label = "Transformer"

	transformOnce := func(name string, f interface{}) cmp.Option {
		xform := cmp.Transformer(name, f)
		return cmp.FilterPath(func(p cmp.Path) bool {
			for _, ps := range p {
				if tr, ok := ps.(cmp.Transform); ok && tr.Option() == xform {
					return false
				}
			}
			return true
		}, xform)
	}

	return []test{{
		label: label,
		x:     uint8(0),
		y:     uint8(1),
		opts: []cmp.Option{
			cmp.Transformer("λ", func(in uint8) uint16 { return uint16(in) }),
			cmp.Transformer("λ", func(in uint16) uint32 { return uint32(in) }),
			cmp.Transformer("λ", func(in uint32) uint64 { return uint64(in) }),
		},
		wantDiff: `
  uint8(Inverse(λ, uint16(Inverse(λ, uint32(Inverse(λ, uint64(
- 	0x00,
+ 	0x01,
  )))))))
`,
	}, {
		label: label,
		x:     0,
		y:     1,
		opts: []cmp.Option{
			cmp.Transformer("λ", func(in int) int { return in / 2 }),
			cmp.Transformer("λ", func(in int) int { return in }),
		},
		wantPanic: "ambiguous set of applicable options",
	}, {
		label: label,
		x:     []int{0, -5, 0, -1},
		y:     []int{1, 3, 0, -5},
		opts: []cmp.Option{
			cmp.FilterValues(
				func(x, y int) bool { return x+y >= 0 },
				cmp.Transformer("λ", func(in int) int64 { return int64(in / 2) }),
			),
			cmp.FilterValues(
				func(x, y int) bool { return x+y < 0 },
				cmp.Transformer("λ", func(in int) int64 { return int64(in) }),
			),
		},
		wantDiff: `
  []int{
  	Inverse(λ, int64(0)),
- 	Inverse(λ, int64(-5)),
+ 	Inverse(λ, int64(3)),
  	Inverse(λ, int64(0)),
- 	Inverse(λ, int64(-1)),
+ 	Inverse(λ, int64(-5)),
  }
`,
	}, {
		label: label,
		x:     0,
		y:     1,
		opts: []cmp.Option{
			cmp.Transformer("λ", func(in int) interface{} {
				if in == 0 {
					return "zero"
				}
				return float64(in)
			}),
		},
		wantDiff: `
  int(Inverse(λ, interface{}(
- 	string("zero"),
+ 	float64(1),
  )))
`,
	}, {
		label: label,
		x: `{
		  "firstName": "John",
		  "lastName": "Smith",
		  "age": 25,
		  "isAlive": true,
		  "address": {
		    "city": "Los Angeles",
		    "postalCode": "10021-3100",
		    "state": "CA",
		    "streetAddress": "21 2nd Street"
		  },
		  "phoneNumbers": [{
		    "type": "home",
		    "number": "212 555-4321"
		  },{
		    "type": "office",
		    "number": "646 555-4567"
		  },{
		    "number": "123 456-7890",
		    "type": "mobile"
		  }],
		  "children": []
		}`,
		y: `{"firstName":"John","lastName":"Smith","isAlive":true,"age":25,
			"address":{"streetAddress":"21 2nd Street","city":"New York",
			"state":"NY","postalCode":"10021-3100"},"phoneNumbers":[{"type":"home",
			"number":"212 555-1234"},{"type":"office","number":"646 555-4567"},{
			"type":"mobile","number":"123 456-7890"}],"children":[],"spouse":null}`,
		opts: []cmp.Option{
			transformOnce("ParseJSON", func(s string) (m map[string]interface{}) {
				if err := json.Unmarshal([]byte(s), &m); err != nil {
					panic(err)
				}
				return m
			}),
		},
		wantDiff: `
  string(Inverse(ParseJSON, map[string]interface{}{
  	"address": map[string]interface{}{
- 		"city":          string("Los Angeles"),
+ 		"city":          string("New York"),
  		"postalCode":    string("10021-3100"),
- 		"state":         string("CA"),
+ 		"state":         string("NY"),
  		"streetAddress": string("21 2nd Street"),
  	},
  	"age":       float64(25),
  	"children":  []interface{}{},
  	"firstName": string("John"),
  	"isAlive":   bool(true),
  	"lastName":  string("Smith"),
  	"phoneNumbers": []interface{}{
  		map[string]interface{}{
- 			"number": string("212 555-4321"),
+ 			"number": string("212 555-1234"),
  			"type":   string("home"),
  		},
  		map[string]interface{}{"number": string("646 555-4567"), "type": string("office")},
  		map[string]interface{}{"number": string("123 456-7890"), "type": string("mobile")},
  	},
+ 	"spouse": nil,
  }))
`,
	}, {
		label: label,
		x:     StringBytes{String: "some\nmulti\nLine\nstring", Bytes: []byte("some\nmulti\nline\nbytes")},
		y:     StringBytes{String: "some\nmulti\nline\nstring", Bytes: []byte("some\nmulti\nline\nBytes")},
		opts: []cmp.Option{
			transformOnce("SplitString", func(s string) []string { return strings.Split(s, "\n") }),
			transformOnce("SplitBytes", func(b []byte) [][]byte { return bytes.Split(b, []byte("\n")) }),
		},
		wantDiff: `
  cmp_test.StringBytes{
  	String: Inverse(SplitString, []string{
  		"some",
  		"multi",
- 		"Line",
+ 		"line",
  		"string",
  	}),
  	Bytes: []uint8(Inverse(SplitBytes, [][]uint8{
  		{0x73, 0x6f, 0x6d, 0x65},
  		{0x6d, 0x75, 0x6c, 0x74, 0x69},
  		{0x6c, 0x69, 0x6e, 0x65},
  		{
- 			0x62,
+ 			0x42,
  			0x79,
  			0x74,
  			... // 2 identical elements
  		},
  	})),
  }
`,
	}, {
		x: "a\nb\nc\n",
		y: "a\nb\nc\n",
		opts: []cmp.Option{
			cmp.Transformer("SplitLines", func(s string) []string { return strings.Split(s, "\n") }),
		},
		wantPanic: "recursive set of Transformers detected",
	}, {
		x: complex64(0),
		y: complex64(0),
		opts: []cmp.Option{
			cmp.Transformer("T1", func(x complex64) complex128 { return complex128(x) }),
			cmp.Transformer("T2", func(x complex128) [2]float64 { return [2]float64{real(x), imag(x)} }),
			cmp.Transformer("T3", func(x float64) complex64 { return complex64(complex(x, 0)) }),
		},
		wantPanic: "recursive set of Transformers detected",
	}}
}

func reporterTests() []test {
	const label = "Reporter"

	type (
		MyString    string
		MyByte      byte
		MyBytes     []byte
		MyInt       int8
		MyInts      []int8
		MyUint      int16
		MyUints     []int16
		MyFloat     float32
		MyFloats    []float32
		MyComposite struct {
			StringA string
			StringB MyString
			BytesA  []byte
			BytesB  []MyByte
			BytesC  MyBytes
			IntsA   []int8
			IntsB   []MyInt
			IntsC   MyInts
			UintsA  []uint16
			UintsB  []MyUint
			UintsC  MyUints
			FloatsA []float32
			FloatsB []MyFloat
			FloatsC MyFloats
		}
	)

	return []test{{
		label: label,
		x:     MyComposite{IntsA: []int8{11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29}},
		y:     MyComposite{IntsA: []int8{10, 11, 21, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29}},
		wantDiff: `
  cmp_test.MyComposite{
  	... // 3 identical fields
  	BytesB: nil,
  	BytesC: nil,
  	IntsA: []int8{
+ 		10,
  		11,
- 		12,
+ 		21,
  		13,
  		14,
  		... // 15 identical elements
  	},
  	IntsB: nil,
  	IntsC: nil,
  	... // 6 identical fields
  }
`,
		reason: "unbatched diffing desired since few elements differ",
	}, {
		label: label,
		x:     MyComposite{IntsA: []int8{10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29}},
		y:     MyComposite{IntsA: []int8{12, 29, 13, 27, 22, 23, 17, 18, 19, 20, 21, 10, 26, 16, 25, 28, 11, 15, 24, 14}},
		wantDiff: `
  cmp_test.MyComposite{
  	... // 3 identical fields
  	BytesB: nil,
  	BytesC: nil,
  	IntsA: []int8{
- 		10, 11, 12, 13, 14, 15, 16,
+ 		12, 29, 13, 27, 22, 23,
  		17, 18, 19, 20, 21,
- 		22, 23, 24, 25, 26, 27, 28, 29,
+ 		10, 26, 16, 25, 28, 11, 15, 24, 14,
  	},
  	IntsB: nil,
  	IntsC: nil,
  	... // 6 identical fields
  }
`,
		reason: "batched diffing desired since many elements differ",
	}, {
		label: label,
		x: MyComposite{
			BytesA:  []byte{1, 2, 3},
			BytesB:  []MyByte{4, 5, 6},
			BytesC:  MyBytes{7, 8, 9},
			IntsA:   []int8{-1, -2, -3},
			IntsB:   []MyInt{-4, -5, -6},
			IntsC:   MyInts{-7, -8, -9},
			UintsA:  []uint16{1000, 2000, 3000},
			UintsB:  []MyUint{4000, 5000, 6000},
			UintsC:  MyUints{7000, 8000, 9000},
			FloatsA: []float32{1.5, 2.5, 3.5},
			FloatsB: []MyFloat{4.5, 5.5, 6.5},
			FloatsC: MyFloats{7.5, 8.5, 9.5},
		},
		y: MyComposite{
			BytesA:  []byte{3, 2, 1},
			BytesB:  []MyByte{6, 5, 4},
			BytesC:  MyBytes{9, 8, 7},
			IntsA:   []int8{-3, -2, -1},
			IntsB:   []MyInt{-6, -5, -4},
			IntsC:   MyInts{-9, -8, -7},
			UintsA:  []uint16{3000, 2000, 1000},
			UintsB:  []MyUint{6000, 5000, 4000},
			UintsC:  MyUints{9000, 8000, 7000},
			FloatsA: []float32{3.5, 2.5, 1.5},
			FloatsB: []MyFloat{6.5, 5.5, 4.5},
			FloatsC: MyFloats{9.5, 8.5, 7.5},
		},
		wantDiff: `
  cmp_test.MyComposite{
  	StringA: "",
  	StringB: "",
  	BytesA: []uint8{
- 		0x01, 0x02, 0x03, // -|...|
+ 		0x03, 0x02, 0x01, // +|...|
  	},
  	BytesB: []cmp_test.MyByte{
- 		0x04, 0x05, 0x06,
+ 		0x06, 0x05, 0x04,
  	},
  	BytesC: cmp_test.MyBytes{
- 		0x07, 0x08, 0x09, // -|...|
+ 		0x09, 0x08, 0x07, // +|...|
  	},
  	IntsA: []int8{
- 		-1, -2, -3,
+ 		-3, -2, -1,
  	},
  	IntsB: []cmp_test.MyInt{
- 		-4, -5, -6,
+ 		-6, -5, -4,
  	},
  	IntsC: cmp_test.MyInts{
- 		-7, -8, -9,
+ 		-9, -8, -7,
  	},
  	UintsA: []uint16{
- 		0x03e8, 0x07d0, 0x0bb8,
+ 		0x0bb8, 0x07d0, 0x03e8,
  	},
  	UintsB: []cmp_test.MyUint{
- 		4000, 5000, 6000,
+ 		6000, 5000, 4000,
  	},
  	UintsC: cmp_test.MyUints{
- 		7000, 8000, 9000,
+ 		9000, 8000, 7000,
  	},
  	FloatsA: []float32{
- 		1.5, 2.5, 3.5,
+ 		3.5, 2.5, 1.5,
  	},
  	FloatsB: []cmp_test.MyFloat{
- 		4.5, 5.5, 6.5,
+ 		6.5, 5.5, 4.5,
  	},
  	FloatsC: cmp_test.MyFloats{
- 		7.5, 8.5, 9.5,
+ 		9.5, 8.5, 7.5,
  	},
  }
`,
		reason: "batched diffing available for both named and unnamed slices",
	}, {
		label: label,
		x:     MyComposite{BytesA: []byte("\xf3\x0f\x8a\xa4\xd3\x12R\t$\xbeX\x95A\xfd$fX\x8byT\xac\r\xd8qwp\x20j\\s\u007f\x8c\x17U\xc04\xcen\xf7\xaaG\xee2\x9d\xc5\xca\x1eX\xaf\x8f'\xf3\x02J\x90\xedi.p2\xb4\xab0 \xb6\xbd\\b4\x17\xb0\x00\xbbO~'G\x06\xf4.f\xfdc\xd7\x04ݷ0\xb7\xd1U~{\xf6\xb3~\x1dWi \x9e\xbc\xdf\xe1M\xa9\xef\xa2\xd2\xed\xb4Gx\xc9\xc9'\xa4\xc6\xce\xecDp]")},
		y:     MyComposite{BytesA: []byte("\xf3\x0f\x8a\xa4\xd3\x12R\t$\xbeT\xac\r\xd8qwp\x20j\\s\u007f\x8c\x17U\xc04\xcen\xf7\xaaG\xee2\x9d\xc5\xca\x1eX\xaf\x8f'\xf3\x02J\x90\xedi.p2\xb4\xab0 \xb6\xbd\\b4\x17\xb0\x00\xbbO~'G\x06\xf4.f\xfdc\xd7\x04ݷ0\xb7\xd1u-[]]\xf6\xb3haha~\x1dWI \x9e\xbc\xdf\xe1M\xa9\xef\xa2\xd2\xed\xb4Gx\xc9\xc9'\xa4\xc6\xce\xecDp]")},
		wantDiff: `
  cmp_test.MyComposite{
  	StringA: "",
  	StringB: "",
  	BytesA: []uint8{
  		0xf3, 0x0f, 0x8a, 0xa4, 0xd3, 0x12, 0x52, 0x09, 0x24, 0xbe,                                     //  |......R.$.|
- 		0x58, 0x95, 0x41, 0xfd, 0x24, 0x66, 0x58, 0x8b, 0x79,                                           // -|X.A.$fX.y|
  		0x54, 0xac, 0x0d, 0xd8, 0x71, 0x77, 0x70, 0x20, 0x6a, 0x5c, 0x73, 0x7f, 0x8c, 0x17, 0x55, 0xc0, //  |T...qwp j\s...U.|
  		0x34, 0xce, 0x6e, 0xf7, 0xaa, 0x47, 0xee, 0x32, 0x9d, 0xc5, 0xca, 0x1e, 0x58, 0xaf, 0x8f, 0x27, //  |4.n..G.2....X..'|
  		0xf3, 0x02, 0x4a, 0x90, 0xed, 0x69, 0x2e, 0x70, 0x32, 0xb4, 0xab, 0x30, 0x20, 0xb6, 0xbd, 0x5c, //  |..J..i.p2..0 ..\|
  		0x62, 0x34, 0x17, 0xb0, 0x00, 0xbb, 0x4f, 0x7e, 0x27, 0x47, 0x06, 0xf4, 0x2e, 0x66, 0xfd, 0x63, //  |b4....O~'G...f.c|
  		0xd7, 0x04, 0xdd, 0xb7, 0x30, 0xb7, 0xd1,                                                       //  |....0..|
- 		0x55, 0x7e, 0x7b, 0xf6, 0xb3, 0x7e, 0x1d, 0x57, 0x69,                                           // -|U~{..~.Wi|
+ 		0x75, 0x2d, 0x5b, 0x5d, 0x5d, 0xf6, 0xb3, 0x68, 0x61, 0x68, 0x61, 0x7e, 0x1d, 0x57, 0x49,       // +|u-[]]..haha~.WI|
  		0x20, 0x9e, 0xbc, 0xdf, 0xe1, 0x4d, 0xa9, 0xef, 0xa2, 0xd2, 0xed, 0xb4, 0x47, 0x78, 0xc9, 0xc9, //  | ....M......Gx..|
  		0x27, 0xa4, 0xc6, 0xce, 0xec, 0x44, 0x70, 0x5d,                                                 //  |'....Dp]|
  	},
  	BytesB: nil,
  	BytesC: nil,
  	... // 9 identical fields
  }
`,
		reason: "binary diff in hexdump form since data is binary data",
	}, {
		label: label,
		x:     MyComposite{StringB: MyString("readme.txt\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x000000600\x000000000\x000000000\x0000000000046\x0000000000000\x00011173\x00 0\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00ustar\x0000\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x000000000\x000000000\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00")},
		y:     MyComposite{StringB: MyString("gopher.txt\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x000000600\x000000000\x000000000\x0000000000043\x0000000000000\x00011217\x00 0\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00ustar\x0000\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x000000000\x000000000\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00")},
		wantDiff: `
  cmp_test.MyComposite{
  	StringA: "",
  	StringB: cmp_test.MyString{
- 		0x72, 0x65, 0x61, 0x64, 0x6d, 0x65,                                                             // -|readme|
+ 		0x67, 0x6f, 0x70, 0x68, 0x65, 0x72,                                                             // +|gopher|
  		0x2e, 0x74, 0x78, 0x74, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, //  |.txt............|
  		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, //  |................|
  		... // 64 identical bytes
  		0x30, 0x30, 0x36, 0x30, 0x30, 0x00, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x00, 0x30, 0x30, //  |00600.0000000.00|
  		0x30, 0x30, 0x30, 0x30, 0x30, 0x00, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x34, //  |00000.0000000004|
- 		0x36,                                                                                           // -|6|
+ 		0x33,                                                                                           // +|3|
  		0x00, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x00, 0x30, 0x31, 0x31, //  |.00000000000.011|
- 		0x31, 0x37, 0x33,                                                                               // -|173|
+ 		0x32, 0x31, 0x37,                                                                               // +|217|
  		0x00, 0x20, 0x30, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, //  |. 0.............|
  		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, //  |................|
  		... // 326 identical bytes
  	},
  	BytesA: nil,
  	BytesB: nil,
  	... // 10 identical fields
  }
`,
		reason: "binary diff desired since string looks like binary data",
	}, {
		label: label,
		x:     MyComposite{BytesA: []byte(`{"firstName":"John","lastName":"Smith","isAlive":true,"age":27,"address":{"streetAddress":"314 54th Avenue","city":"New York","state":"NY","postalCode":"10021-3100"},"phoneNumbers":[{"type":"home","number":"212 555-1234"},{"type":"office","number":"646 555-4567"},{"type":"mobile","number":"123 456-7890"}],"children":[],"spouse":null}`)},
		y:     MyComposite{BytesA: []byte(`{"firstName":"John","lastName":"Smith","isAlive":true,"age":27,"address":{"streetAddress":"21 2nd Street","city":"New York","state":"NY","postalCode":"10021-3100"},"phoneNumbers":[{"type":"home","number":"212 555-1234"},{"type":"office","number":"646 555-4567"},{"type":"mobile","number":"123 456-7890"}],"children":[],"spouse":null}`)},
		wantDiff: strings.Replace(`
  cmp_test.MyComposite{
  	StringA: "",
  	StringB: "",
  	BytesA: bytes.Join({
  		'{"firstName":"John","lastName":"Smith","isAlive":true,"age":27,"',
  		'address":{"streetAddress":"',
- 		"314 54th Avenue",
+ 		"21 2nd Street",
  		'","city":"New York","state":"NY","postalCode":"10021-3100"},"pho',
  		'neNumbers":[{"type":"home","number":"212 555-1234"},{"type":"off',
  		... // 101 identical bytes
  	}, ""),
  	BytesB: nil,
  	BytesC: nil,
  	... // 9 identical fields
  }
`, "'", "`", -1),
		reason: "batched textual diff desired since bytes looks like textual data",
	}, {
		label: label,
		x: MyComposite{
			StringA: strings.TrimPrefix(`
Package cmp determines equality of values.

This package is intended to be a more powerful and safer alternative to
reflect.DeepEqual for comparing whether two values are semantically equal.

The primary features of cmp are:

• When the default behavior of equality does not suit the needs of the test,
custom equality functions can override the equality operation.
For example, an equality function may report floats as equal so long as they
are within some tolerance of each other.

• Types that have an Equal method may use that method to determine equality.
This allows package authors to determine the equality operation for the types
that they define.

• If no custom equality functions are used and no Equal method is defined,
equality is determined by recursively comparing the primitive kinds on both
values, much like reflect.DeepEqual. Unlike reflect.DeepEqual, unexported
fields are not compared by default; they result in panics unless suppressed
by using an Ignore option (see cmpopts.IgnoreUnexported) or explicitly compared
using the AllowUnexported option.
`, "\n"),
		},
		y: MyComposite{
			StringA: strings.TrimPrefix(`
Package cmp determines equality of value.

This package is intended to be a more powerful and safer alternative to
reflect.DeepEqual for comparing whether two values are semantically equal.

The primary features of cmp are:

• When the default behavior of equality does not suit the needs of the test,
custom equality functions can override the equality operation.
For example, an equality function may report floats as equal so long as they
are within some tolerance of each other.

• If no custom equality functions are used and no Equal method is defined,
equality is determined by recursively comparing the primitive kinds on both
values, much like reflect.DeepEqual. Unlike reflect.DeepEqual, unexported
fields are not compared by default; they result in panics unless suppressed
by using an Ignore option (see cmpopts.IgnoreUnexported) or explicitly compared
using the AllowUnexported option.`, "\n"),
		},
		wantDiff: `
  cmp_test.MyComposite{
  	StringA: strings.Join({
- 		"Package cmp determines equality of values.",
+ 		"Package cmp determines equality of value.",
  		"",
  		"This package is intended to be a more powerful and safer alternative to",
  		... // 6 identical lines
  		"For example, an equality function may report floats as equal so long as they",
  		"are within some tolerance of each other.",
- 		"",
- 		"• Types that have an Equal method may use that method to determine equality.",
- 		"This allows package authors to determine the equality operation for the types",
- 		"that they define.",
  		"",
  		"• If no custom equality functions are used and no Equal method is defined,",
  		... // 3 identical lines
  		"by using an Ignore option (see cmpopts.IgnoreUnexported) or explicitly compared",
  		"using the AllowUnexported option.",
- 		"",
  	}, "\n"),
  	StringB: "",
  	BytesA:  nil,
  	... // 11 identical fields
  }
`,
		reason: "batched per-line diff desired since string looks like multi-line textual data",
	}}
}

func embeddedTests() []test {
	const label = "EmbeddedStruct/"

	privateStruct := *new(ts.ParentStructA).PrivateStruct()

	createStructA := func(i int) ts.ParentStructA {
		s := ts.ParentStructA{}
		s.PrivateStruct().Public = 1 + i
		s.PrivateStruct().SetPrivate(2 + i)
		return s
	}

	createStructB := func(i int) ts.ParentStructB {
		s := ts.ParentStructB{}
		s.PublicStruct.Public = 1 + i
		s.PublicStruct.SetPrivate(2 + i)
		return s
	}

	createStructC := func(i int) ts.ParentStructC {
		s := ts.ParentStructC{}
		s.PrivateStruct().Public = 1 + i
		s.PrivateStruct().SetPrivate(2 + i)
		s.Public = 3 + i
		s.SetPrivate(4 + i)
		return s
	}

	createStructD := func(i int) ts.ParentStructD {
		s := ts.ParentStructD{}
		s.PublicStruct.Public = 1 + i
		s.PublicStruct.SetPrivate(2 + i)
		s.Public = 3 + i
		s.SetPrivate(4 + i)
		return s
	}

	createStructE := func(i int) ts.ParentStructE {
		s := ts.ParentStructE{}
		s.PrivateStruct().Public = 1 + i
		s.PrivateStruct().SetPrivate(2 + i)
		s.PublicStruct.Public = 3 + i
		s.PublicStruct.SetPrivate(4 + i)
		return s
	}

	createStructF := func(i int) ts.ParentStructF {
		s := ts.ParentStructF{}
		s.PrivateStruct().Public = 1 + i
		s.PrivateStruct().SetPrivate(2 + i)
		s.PublicStruct.Public = 3 + i
		s.PublicStruct.SetPrivate(4 + i)
		s.Public = 5 + i
		s.SetPrivate(6 + i)
		return s
	}

	createStructG := func(i int) *ts.ParentStructG {
		s := ts.NewParentStructG()
		s.PrivateStruct().Public = 1 + i
		s.PrivateStruct().SetPrivate(2 + i)
		return s
	}

	createStructH := func(i int) *ts.ParentStructH {
		s := ts.NewParentStructH()
		s.PublicStruct.Public = 1 + i
		s.PublicStruct.SetPrivate(2 + i)
		return s
	}

	createStructI := func(i int) *ts.ParentStructI {
		s := ts.NewParentStructI()
		s.PrivateStruct().Public = 1 + i
		s.PrivateStruct().SetPrivate(2 + i)
		s.PublicStruct.Public = 3 + i
		s.PublicStruct.SetPrivate(4 + i)
		return s
	}

	createStructJ := func(i int) *ts.ParentStructJ {
		s := ts.NewParentStructJ()
		s.PrivateStruct().Public = 1 + i
		s.PrivateStruct().SetPrivate(2 + i)
		s.PublicStruct.Public = 3 + i
		s.PublicStruct.SetPrivate(4 + i)
		s.Private().Public = 5 + i
		s.Private().SetPrivate(6 + i)
		s.Public.Public = 7 + i
		s.Public.SetPrivate(8 + i)
		return s
	}

	// TODO(dsnet): Workaround for reflect bug (https://golang.org/issue/21122).
	wantPanicNotGo110 := func(s string) string {
		if !flags.AtLeastGo110 {
			return ""
		}
		return s
	}

	return []test{{
		label:     label + "ParentStructA",
		x:         ts.ParentStructA{},
		y:         ts.ParentStructA{},
		wantPanic: "cannot handle unexported field",
	}, {
		label: label + "ParentStructA",
		x:     ts.ParentStructA{},
		y:     ts.ParentStructA{},
		opts: []cmp.Option{
			cmpopts.IgnoreUnexported(ts.ParentStructA{}),
		},
	}, {
		label: label + "ParentStructA",
		x:     createStructA(0),
		y:     createStructA(0),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructA{}),
		},
		wantPanic: "cannot handle unexported field",
	}, {
		label: label + "ParentStructA",
		x:     createStructA(0),
		y:     createStructA(0),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructA{}, privateStruct),
		},
	}, {
		label: label + "ParentStructA",
		x:     createStructA(0),
		y:     createStructA(1),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructA{}, privateStruct),
		},
		wantDiff: `
  teststructs.ParentStructA{
  	privateStruct: teststructs.privateStruct{
- 		Public:  1,
+ 		Public:  2,
- 		private: 2,
+ 		private: 3,
  	},
  }
`,
	}, {
		label: label + "ParentStructB",
		x:     ts.ParentStructB{},
		y:     ts.ParentStructB{},
		opts: []cmp.Option{
			cmpopts.IgnoreUnexported(ts.ParentStructB{}),
		},
		wantPanic: "cannot handle unexported field",
	}, {
		label: label + "ParentStructB",
		x:     ts.ParentStructB{},
		y:     ts.ParentStructB{},
		opts: []cmp.Option{
			cmpopts.IgnoreUnexported(ts.ParentStructB{}),
			cmpopts.IgnoreUnexported(ts.PublicStruct{}),
		},
	}, {
		label: label + "ParentStructB",
		x:     createStructB(0),
		y:     createStructB(0),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructB{}),
		},
		wantPanic: "cannot handle unexported field",
	}, {
		label: label + "ParentStructB",
		x:     createStructB(0),
		y:     createStructB(0),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructB{}, ts.PublicStruct{}),
		},
	}, {
		label: label + "ParentStructB",
		x:     createStructB(0),
		y:     createStructB(1),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructB{}, ts.PublicStruct{}),
		},
		wantDiff: `
  teststructs.ParentStructB{
  	PublicStruct: teststructs.PublicStruct{
- 		Public:  1,
+ 		Public:  2,
- 		private: 2,
+ 		private: 3,
  	},
  }
`,
	}, {
		label:     label + "ParentStructC",
		x:         ts.ParentStructC{},
		y:         ts.ParentStructC{},
		wantPanic: "cannot handle unexported field",
	}, {
		label: label + "ParentStructC",
		x:     ts.ParentStructC{},
		y:     ts.ParentStructC{},
		opts: []cmp.Option{
			cmpopts.IgnoreUnexported(ts.ParentStructC{}),
		},
	}, {
		label: label + "ParentStructC",
		x:     createStructC(0),
		y:     createStructC(0),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructC{}),
		},
		wantPanic: "cannot handle unexported field",
	}, {
		label: label + "ParentStructC",
		x:     createStructC(0),
		y:     createStructC(0),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructC{}, privateStruct),
		},
	}, {
		label: label + "ParentStructC",
		x:     createStructC(0),
		y:     createStructC(1),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructC{}, privateStruct),
		},
		wantDiff: `
  teststructs.ParentStructC{
  	privateStruct: teststructs.privateStruct{
- 		Public:  1,
+ 		Public:  2,
- 		private: 2,
+ 		private: 3,
  	},
- 	Public:  3,
+ 	Public:  4,
- 	private: 4,
+ 	private: 5,
  }
`,
	}, {
		label: label + "ParentStructD",
		x:     ts.ParentStructD{},
		y:     ts.ParentStructD{},
		opts: []cmp.Option{
			cmpopts.IgnoreUnexported(ts.ParentStructD{}),
		},
		wantPanic: "cannot handle unexported field",
	}, {
		label: label + "ParentStructD",
		x:     ts.ParentStructD{},
		y:     ts.ParentStructD{},
		opts: []cmp.Option{
			cmpopts.IgnoreUnexported(ts.ParentStructD{}),
			cmpopts.IgnoreUnexported(ts.PublicStruct{}),
		},
	}, {
		label: label + "ParentStructD",
		x:     createStructD(0),
		y:     createStructD(0),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructD{}),
		},
		wantPanic: "cannot handle unexported field",
	}, {
		label: label + "ParentStructD",
		x:     createStructD(0),
		y:     createStructD(0),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructD{}, ts.PublicStruct{}),
		},
	}, {
		label: label + "ParentStructD",
		x:     createStructD(0),
		y:     createStructD(1),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructD{}, ts.PublicStruct{}),
		},
		wantDiff: `
  teststructs.ParentStructD{
  	PublicStruct: teststructs.PublicStruct{
- 		Public:  1,
+ 		Public:  2,
- 		private: 2,
+ 		private: 3,
  	},
- 	Public:  3,
+ 	Public:  4,
- 	private: 4,
+ 	private: 5,
  }
`,
	}, {
		label: label + "ParentStructE",
		x:     ts.ParentStructE{},
		y:     ts.ParentStructE{},
		opts: []cmp.Option{
			cmpopts.IgnoreUnexported(ts.ParentStructE{}),
		},
		wantPanic: "cannot handle unexported field",
	}, {
		label: label + "ParentStructE",
		x:     ts.ParentStructE{},
		y:     ts.ParentStructE{},
		opts: []cmp.Option{
			cmpopts.IgnoreUnexported(ts.ParentStructE{}),
			cmpopts.IgnoreUnexported(ts.PublicStruct{}),
		},
	}, {
		label: label + "ParentStructE",
		x:     createStructE(0),
		y:     createStructE(0),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructE{}),
		},
		wantPanic: "cannot handle unexported field",
	}, {
		label: label + "ParentStructE",
		x:     createStructE(0),
		y:     createStructE(0),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructE{}, ts.PublicStruct{}),
		},
		wantPanic: "cannot handle unexported field",
	}, {
		label: label + "ParentStructE",
		x:     createStructE(0),
		y:     createStructE(0),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructE{}, ts.PublicStruct{}, privateStruct),
		},
	}, {
		label: label + "ParentStructE",
		x:     createStructE(0),
		y:     createStructE(1),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructE{}, ts.PublicStruct{}, privateStruct),
		},
		wantDiff: `
  teststructs.ParentStructE{
  	privateStruct: teststructs.privateStruct{
- 		Public:  1,
+ 		Public:  2,
- 		private: 2,
+ 		private: 3,
  	},
  	PublicStruct: teststructs.PublicStruct{
- 		Public:  3,
+ 		Public:  4,
- 		private: 4,
+ 		private: 5,
  	},
  }
`,
	}, {
		label: label + "ParentStructF",
		x:     ts.ParentStructF{},
		y:     ts.ParentStructF{},
		opts: []cmp.Option{
			cmpopts.IgnoreUnexported(ts.ParentStructF{}),
		},
		wantPanic: "cannot handle unexported field",
	}, {
		label: label + "ParentStructF",
		x:     ts.ParentStructF{},
		y:     ts.ParentStructF{},
		opts: []cmp.Option{
			cmpopts.IgnoreUnexported(ts.ParentStructF{}),
			cmpopts.IgnoreUnexported(ts.PublicStruct{}),
		},
	}, {
		label: label + "ParentStructF",
		x:     createStructF(0),
		y:     createStructF(0),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructF{}),
		},
		wantPanic: "cannot handle unexported field",
	}, {
		label: label + "ParentStructF",
		x:     createStructF(0),
		y:     createStructF(0),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructF{}, ts.PublicStruct{}),
		},
		wantPanic: "cannot handle unexported field",
	}, {
		label: label + "ParentStructF",
		x:     createStructF(0),
		y:     createStructF(0),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructF{}, ts.PublicStruct{}, privateStruct),
		},
	}, {
		label: label + "ParentStructF",
		x:     createStructF(0),
		y:     createStructF(1),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructF{}, ts.PublicStruct{}, privateStruct),
		},
		wantDiff: `
  teststructs.ParentStructF{
  	privateStruct: teststructs.privateStruct{
- 		Public:  1,
+ 		Public:  2,
- 		private: 2,
+ 		private: 3,
  	},
  	PublicStruct: teststructs.PublicStruct{
- 		Public:  3,
+ 		Public:  4,
- 		private: 4,
+ 		private: 5,
  	},
- 	Public:  5,
+ 	Public:  6,
- 	private: 6,
+ 	private: 7,
  }
`,
	}, {
		label:     label + "ParentStructG",
		x:         ts.ParentStructG{},
		y:         ts.ParentStructG{},
		wantPanic: wantPanicNotGo110("cannot handle unexported field"),
	}, {
		label: label + "ParentStructG",
		x:     ts.ParentStructG{},
		y:     ts.ParentStructG{},
		opts: []cmp.Option{
			cmpopts.IgnoreUnexported(ts.ParentStructG{}),
		},
	}, {
		label: label + "ParentStructG",
		x:     createStructG(0),
		y:     createStructG(0),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructG{}),
		},
		wantPanic: "cannot handle unexported field",
	}, {
		label: label + "ParentStructG",
		x:     createStructG(0),
		y:     createStructG(0),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructG{}, privateStruct),
		},
	}, {
		label: label + "ParentStructG",
		x:     createStructG(0),
		y:     createStructG(1),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructG{}, privateStruct),
		},
		wantDiff: `
  &teststructs.ParentStructG{
  	privateStruct: &teststructs.privateStruct{
- 		Public:  1,
+ 		Public:  2,
- 		private: 2,
+ 		private: 3,
  	},
  }
`,
	}, {
		label: label + "ParentStructH",
		x:     ts.ParentStructH{},
		y:     ts.ParentStructH{},
	}, {
		label:     label + "ParentStructH",
		x:         createStructH(0),
		y:         createStructH(0),
		wantPanic: "cannot handle unexported field",
	}, {
		label: label + "ParentStructH",
		x:     ts.ParentStructH{},
		y:     ts.ParentStructH{},
		opts: []cmp.Option{
			cmpopts.IgnoreUnexported(ts.ParentStructH{}),
		},
	}, {
		label: label + "ParentStructH",
		x:     createStructH(0),
		y:     createStructH(0),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructH{}),
		},
		wantPanic: "cannot handle unexported field",
	}, {
		label: label + "ParentStructH",
		x:     createStructH(0),
		y:     createStructH(0),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructH{}, ts.PublicStruct{}),
		},
	}, {
		label: label + "ParentStructH",
		x:     createStructH(0),
		y:     createStructH(1),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructH{}, ts.PublicStruct{}),
		},
		wantDiff: `
  &teststructs.ParentStructH{
  	PublicStruct: &teststructs.PublicStruct{
- 		Public:  1,
+ 		Public:  2,
- 		private: 2,
+ 		private: 3,
  	},
  }
`,
	}, {
		label:     label + "ParentStructI",
		x:         ts.ParentStructI{},
		y:         ts.ParentStructI{},
		wantPanic: wantPanicNotGo110("cannot handle unexported field"),
	}, {
		label: label + "ParentStructI",
		x:     ts.ParentStructI{},
		y:     ts.ParentStructI{},
		opts: []cmp.Option{
			cmpopts.IgnoreUnexported(ts.ParentStructI{}),
		},
	}, {
		label: label + "ParentStructI",
		x:     createStructI(0),
		y:     createStructI(0),
		opts: []cmp.Option{
			cmpopts.IgnoreUnexported(ts.ParentStructI{}),
		},
		wantPanic: "cannot handle unexported field",
	}, {
		label: label + "ParentStructI",
		x:     createStructI(0),
		y:     createStructI(0),
		opts: []cmp.Option{
			cmpopts.IgnoreUnexported(ts.ParentStructI{}, ts.PublicStruct{}),
		},
	}, {
		label: label + "ParentStructI",
		x:     createStructI(0),
		y:     createStructI(0),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructI{}),
		},
		wantPanic: "cannot handle unexported field",
	}, {
		label: label + "ParentStructI",
		x:     createStructI(0),
		y:     createStructI(0),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructI{}, ts.PublicStruct{}, privateStruct),
		},
	}, {
		label: label + "ParentStructI",
		x:     createStructI(0),
		y:     createStructI(1),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructI{}, ts.PublicStruct{}, privateStruct),
		},
		wantDiff: `
  &teststructs.ParentStructI{
  	privateStruct: &teststructs.privateStruct{
- 		Public:  1,
+ 		Public:  2,
- 		private: 2,
+ 		private: 3,
  	},
  	PublicStruct: &teststructs.PublicStruct{
- 		Public:  3,
+ 		Public:  4,
- 		private: 4,
+ 		private: 5,
  	},
  }
`,
	}, {
		label:     label + "ParentStructJ",
		x:         ts.ParentStructJ{},
		y:         ts.ParentStructJ{},
		wantPanic: "cannot handle unexported field",
	}, {
		label: label + "ParentStructJ",
		x:     ts.ParentStructJ{},
		y:     ts.ParentStructJ{},
		opts: []cmp.Option{
			cmpopts.IgnoreUnexported(ts.ParentStructJ{}),
		},
		wantPanic: "cannot handle unexported field",
	}, {
		label: label + "ParentStructJ",
		x:     ts.ParentStructJ{},
		y:     ts.ParentStructJ{},
		opts: []cmp.Option{
			cmpopts.IgnoreUnexported(ts.ParentStructJ{}, ts.PublicStruct{}),
		},
	}, {
		label: label + "ParentStructJ",
		x:     createStructJ(0),
		y:     createStructJ(0),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructJ{}, ts.PublicStruct{}),
		},
		wantPanic: "cannot handle unexported field",
	}, {
		label: label + "ParentStructJ",
		x:     createStructJ(0),
		y:     createStructJ(0),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructJ{}, ts.PublicStruct{}, privateStruct),
		},
	}, {
		label: label + "ParentStructJ",
		x:     createStructJ(0),
		y:     createStructJ(1),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructJ{}, ts.PublicStruct{}, privateStruct),
		},
		wantDiff: `
  &teststructs.ParentStructJ{
  	privateStruct: &teststructs.privateStruct{
- 		Public:  1,
+ 		Public:  2,
- 		private: 2,
+ 		private: 3,
  	},
  	PublicStruct: &teststructs.PublicStruct{
- 		Public:  3,
+ 		Public:  4,
- 		private: 4,
+ 		private: 5,
  	},
  	Public: teststructs.PublicStruct{
- 		Public:  7,
+ 		Public:  8,
- 		private: 8,
+ 		private: 9,
  	},
  	private: teststructs.privateStruct{
- 		Public:  5,
+ 		Public:  6,
- 		private: 6,
+ 		private: 7,
  	},
  }
`,
	}}
}

func methodTests() []test {
	const label = "EqualMethod/"

	// A common mistake that the Equal method is on a pointer receiver,
	// but only a non-pointer value is present in the struct.
	// A transform can be used to forcibly reference the value.
	derefTransform := cmp.FilterPath(func(p cmp.Path) bool {
		if len(p) == 0 {
			return false
		}
		t := p[len(p)-1].Type()
		if _, ok := t.MethodByName("Equal"); ok || t.Kind() == reflect.Ptr {
			return false
		}
		if m, ok := reflect.PtrTo(t).MethodByName("Equal"); ok {
			tf := m.Func.Type()
			return !tf.IsVariadic() && tf.NumIn() == 2 && tf.NumOut() == 1 &&
				tf.In(0).AssignableTo(tf.In(1)) && tf.Out(0) == reflect.TypeOf(true)
		}
		return false
	}, cmp.Transformer("Ref", func(x interface{}) interface{} {
		v := reflect.ValueOf(x)
		vp := reflect.New(v.Type())
		vp.Elem().Set(v)
		return vp.Interface()
	}))

	// For each of these types, there is an Equal method defined, which always
	// returns true, while the underlying data are fundamentally different.
	// Since the method should be called, these are expected to be equal.
	return []test{{
		label: label + "StructA",
		x:     ts.StructA{X: "NotEqual"},
		y:     ts.StructA{X: "not_equal"},
	}, {
		label: label + "StructA",
		x:     &ts.StructA{X: "NotEqual"},
		y:     &ts.StructA{X: "not_equal"},
	}, {
		label: label + "StructB",
		x:     ts.StructB{X: "NotEqual"},
		y:     ts.StructB{X: "not_equal"},
		wantDiff: `
  teststructs.StructB{
- 	X: "NotEqual",
+ 	X: "not_equal",
  }
`,
	}, {
		label: label + "StructB",
		x:     ts.StructB{X: "NotEqual"},
		y:     ts.StructB{X: "not_equal"},
		opts:  []cmp.Option{derefTransform},
	}, {
		label: label + "StructB",
		x:     &ts.StructB{X: "NotEqual"},
		y:     &ts.StructB{X: "not_equal"},
	}, {
		label: label + "StructC",
		x:     ts.StructC{X: "NotEqual"},
		y:     ts.StructC{X: "not_equal"},
	}, {
		label: label + "StructC",
		x:     &ts.StructC{X: "NotEqual"},
		y:     &ts.StructC{X: "not_equal"},
	}, {
		label: label + "StructD",
		x:     ts.StructD{X: "NotEqual"},
		y:     ts.StructD{X: "not_equal"},
		wantDiff: `
  teststructs.StructD{
- 	X: "NotEqual",
+ 	X: "not_equal",
  }
`,
	}, {
		label: label + "StructD",
		x:     ts.StructD{X: "NotEqual"},
		y:     ts.StructD{X: "not_equal"},
		opts:  []cmp.Option{derefTransform},
	}, {
		label: label + "StructD",
		x:     &ts.StructD{X: "NotEqual"},
		y:     &ts.StructD{X: "not_equal"},
	}, {
		label: label + "StructE",
		x:     ts.StructE{X: "NotEqual"},
		y:     ts.StructE{X: "not_equal"},
		wantDiff: `
  teststructs.StructE{
- 	X: "NotEqual",
+ 	X: "not_equal",
  }
`,
	}, {
		label: label + "StructE",
		x:     ts.StructE{X: "NotEqual"},
		y:     ts.StructE{X: "not_equal"},
		opts:  []cmp.Option{derefTransform},
	}, {
		label: label + "StructE",
		x:     &ts.StructE{X: "NotEqual"},
		y:     &ts.StructE{X: "not_equal"},
	}, {
		label: label + "StructF",
		x:     ts.StructF{X: "NotEqual"},
		y:     ts.StructF{X: "not_equal"},
		wantDiff: `
  teststructs.StructF{
- 	X: "NotEqual",
+ 	X: "not_equal",
  }
`,
	}, {
		label: label + "StructF",
		x:     &ts.StructF{X: "NotEqual"},
		y:     &ts.StructF{X: "not_equal"},
	}, {
		label: label + "StructA1",
		x:     ts.StructA1{StructA: ts.StructA{X: "NotEqual"}, X: "equal"},
		y:     ts.StructA1{StructA: ts.StructA{X: "not_equal"}, X: "equal"},
	}, {
		label: label + "StructA1",
		x:     ts.StructA1{StructA: ts.StructA{X: "NotEqual"}, X: "NotEqual"},
		y:     ts.StructA1{StructA: ts.StructA{X: "not_equal"}, X: "not_equal"},
		wantDiff: `
  teststructs.StructA1{
  	StructA: teststructs.StructA{X: "NotEqual"},
- 	X:       "NotEqual",
+ 	X:       "not_equal",
  }
`,
	}, {
		label: label + "StructA1",
		x:     &ts.StructA1{StructA: ts.StructA{X: "NotEqual"}, X: "equal"},
		y:     &ts.StructA1{StructA: ts.StructA{X: "not_equal"}, X: "equal"},
	}, {
		label: label + "StructA1",
		x:     &ts.StructA1{StructA: ts.StructA{X: "NotEqual"}, X: "NotEqual"},
		y:     &ts.StructA1{StructA: ts.StructA{X: "not_equal"}, X: "not_equal"},
		wantDiff: `
  &teststructs.StructA1{
  	StructA: teststructs.StructA{X: "NotEqual"},
- 	X:       "NotEqual",
+ 	X:       "not_equal",
  }
`,
	}, {
		label: label + "StructB1",
		x:     ts.StructB1{StructB: ts.StructB{X: "NotEqual"}, X: "equal"},
		y:     ts.StructB1{StructB: ts.StructB{X: "not_equal"}, X: "equal"},
		opts:  []cmp.Option{derefTransform},
	}, {
		label: label + "StructB1",
		x:     ts.StructB1{StructB: ts.StructB{X: "NotEqual"}, X: "NotEqual"},
		y:     ts.StructB1{StructB: ts.StructB{X: "not_equal"}, X: "not_equal"},
		opts:  []cmp.Option{derefTransform},
		wantDiff: `
  teststructs.StructB1{
  	StructB: teststructs.StructB(Inverse(Ref, &teststructs.StructB{X: "NotEqual"})),
- 	X:       "NotEqual",
+ 	X:       "not_equal",
  }
`,
	}, {
		label: label + "StructB1",
		x:     &ts.StructB1{StructB: ts.StructB{X: "NotEqual"}, X: "equal"},
		y:     &ts.StructB1{StructB: ts.StructB{X: "not_equal"}, X: "equal"},
		opts:  []cmp.Option{derefTransform},
	}, {
		label: label + "StructB1",
		x:     &ts.StructB1{StructB: ts.StructB{X: "NotEqual"}, X: "NotEqual"},
		y:     &ts.StructB1{StructB: ts.StructB{X: "not_equal"}, X: "not_equal"},
		opts:  []cmp.Option{derefTransform},
		wantDiff: `
  &teststructs.StructB1{
  	StructB: teststructs.StructB(Inverse(Ref, &teststructs.StructB{X: "NotEqual"})),
- 	X:       "NotEqual",
+ 	X:       "not_equal",
  }
`,
	}, {
		label: label + "StructC1",
		x:     ts.StructC1{StructC: ts.StructC{X: "NotEqual"}, X: "NotEqual"},
		y:     ts.StructC1{StructC: ts.StructC{X: "not_equal"}, X: "not_equal"},
	}, {
		label: label + "StructC1",
		x:     &ts.StructC1{StructC: ts.StructC{X: "NotEqual"}, X: "NotEqual"},
		y:     &ts.StructC1{StructC: ts.StructC{X: "not_equal"}, X: "not_equal"},
	}, {
		label: label + "StructD1",
		x:     ts.StructD1{StructD: ts.StructD{X: "NotEqual"}, X: "NotEqual"},
		y:     ts.StructD1{StructD: ts.StructD{X: "not_equal"}, X: "not_equal"},
		wantDiff: `
  teststructs.StructD1{
- 	StructD: teststructs.StructD{X: "NotEqual"},
+ 	StructD: teststructs.StructD{X: "not_equal"},
- 	X:       "NotEqual",
+ 	X:       "not_equal",
  }
`,
	}, {
		label: label + "StructD1",
		x:     ts.StructD1{StructD: ts.StructD{X: "NotEqual"}, X: "NotEqual"},
		y:     ts.StructD1{StructD: ts.StructD{X: "not_equal"}, X: "not_equal"},
		opts:  []cmp.Option{derefTransform},
	}, {
		label: label + "StructD1",
		x:     &ts.StructD1{StructD: ts.StructD{X: "NotEqual"}, X: "NotEqual"},
		y:     &ts.StructD1{StructD: ts.StructD{X: "not_equal"}, X: "not_equal"},
	}, {
		label: label + "StructE1",
		x:     ts.StructE1{StructE: ts.StructE{X: "NotEqual"}, X: "NotEqual"},
		y:     ts.StructE1{StructE: ts.StructE{X: "not_equal"}, X: "not_equal"},
		wantDiff: `
  teststructs.StructE1{
- 	StructE: teststructs.StructE{X: "NotEqual"},
+ 	StructE: teststructs.StructE{X: "not_equal"},
- 	X:       "NotEqual",
+ 	X:       "not_equal",
  }
`,
	}, {
		label: label + "StructE1",
		x:     ts.StructE1{StructE: ts.StructE{X: "NotEqual"}, X: "NotEqual"},
		y:     ts.StructE1{StructE: ts.StructE{X: "not_equal"}, X: "not_equal"},
		opts:  []cmp.Option{derefTransform},
	}, {
		label: label + "StructE1",
		x:     &ts.StructE1{StructE: ts.StructE{X: "NotEqual"}, X: "NotEqual"},
		y:     &ts.StructE1{StructE: ts.StructE{X: "not_equal"}, X: "not_equal"},
	}, {
		label: label + "StructF1",
		x:     ts.StructF1{StructF: ts.StructF{X: "NotEqual"}, X: "NotEqual"},
		y:     ts.StructF1{StructF: ts.StructF{X: "not_equal"}, X: "not_equal"},
		wantDiff: `
  teststructs.StructF1{
- 	StructF: teststructs.StructF{X: "NotEqual"},
+ 	StructF: teststructs.StructF{X: "not_equal"},
- 	X:       "NotEqual",
+ 	X:       "not_equal",
  }
`,
	}, {
		label: label + "StructF1",
		x:     &ts.StructF1{StructF: ts.StructF{X: "NotEqual"}, X: "NotEqual"},
		y:     &ts.StructF1{StructF: ts.StructF{X: "not_equal"}, X: "not_equal"},
	}, {
		label: label + "StructA2",
		x:     ts.StructA2{StructA: &ts.StructA{X: "NotEqual"}, X: "equal"},
		y:     ts.StructA2{StructA: &ts.StructA{X: "not_equal"}, X: "equal"},
	}, {
		label: label + "StructA2",
		x:     ts.StructA2{StructA: &ts.StructA{X: "NotEqual"}, X: "NotEqual"},
		y:     ts.StructA2{StructA: &ts.StructA{X: "not_equal"}, X: "not_equal"},
		wantDiff: `
  teststructs.StructA2{
  	StructA: &teststructs.StructA{X: "NotEqual"},
- 	X:       "NotEqual",
+ 	X:       "not_equal",
  }
`,
	}, {
		label: label + "StructA2",
		x:     &ts.StructA2{StructA: &ts.StructA{X: "NotEqual"}, X: "equal"},
		y:     &ts.StructA2{StructA: &ts.StructA{X: "not_equal"}, X: "equal"},
	}, {
		label: label + "StructA2",
		x:     &ts.StructA2{StructA: &ts.StructA{X: "NotEqual"}, X: "NotEqual"},
		y:     &ts.StructA2{StructA: &ts.StructA{X: "not_equal"}, X: "not_equal"},
		wantDiff: `
  &teststructs.StructA2{
  	StructA: &teststructs.StructA{X: "NotEqual"},
- 	X:       "NotEqual",
+ 	X:       "not_equal",
  }
`,
	}, {
		label: label + "StructB2",
		x:     ts.StructB2{StructB: &ts.StructB{X: "NotEqual"}, X: "equal"},
		y:     ts.StructB2{StructB: &ts.StructB{X: "not_equal"}, X: "equal"},
	}, {
		label: label + "StructB2",
		x:     ts.StructB2{StructB: &ts.StructB{X: "NotEqual"}, X: "NotEqual"},
		y:     ts.StructB2{StructB: &ts.StructB{X: "not_equal"}, X: "not_equal"},
		wantDiff: `
  teststructs.StructB2{
  	StructB: &teststructs.StructB{X: "NotEqual"},
- 	X:       "NotEqual",
+ 	X:       "not_equal",
  }
`,
	}, {
		label: label + "StructB2",
		x:     &ts.StructB2{StructB: &ts.StructB{X: "NotEqual"}, X: "equal"},
		y:     &ts.StructB2{StructB: &ts.StructB{X: "not_equal"}, X: "equal"},
	}, {
		label: label + "StructB2",
		x:     &ts.StructB2{StructB: &ts.StructB{X: "NotEqual"}, X: "NotEqual"},
		y:     &ts.StructB2{StructB: &ts.StructB{X: "not_equal"}, X: "not_equal"},
		wantDiff: `
  &teststructs.StructB2{
  	StructB: &teststructs.StructB{X: "NotEqual"},
- 	X:       "NotEqual",
+ 	X:       "not_equal",
  }
`,
	}, {
		label: label + "StructC2",
		x:     ts.StructC2{StructC: &ts.StructC{X: "NotEqual"}, X: "NotEqual"},
		y:     ts.StructC2{StructC: &ts.StructC{X: "not_equal"}, X: "not_equal"},
	}, {
		label: label + "StructC2",
		x:     &ts.StructC2{StructC: &ts.StructC{X: "NotEqual"}, X: "NotEqual"},
		y:     &ts.StructC2{StructC: &ts.StructC{X: "not_equal"}, X: "not_equal"},
	}, {
		label: label + "StructD2",
		x:     ts.StructD2{StructD: &ts.StructD{X: "NotEqual"}, X: "NotEqual"},
		y:     ts.StructD2{StructD: &ts.StructD{X: "not_equal"}, X: "not_equal"},
	}, {
		label: label + "StructD2",
		x:     &ts.StructD2{StructD: &ts.StructD{X: "NotEqual"}, X: "NotEqual"},
		y:     &ts.StructD2{StructD: &ts.StructD{X: "not_equal"}, X: "not_equal"},
	}, {
		label: label + "StructE2",
		x:     ts.StructE2{StructE: &ts.StructE{X: "NotEqual"}, X: "NotEqual"},
		y:     ts.StructE2{StructE: &ts.StructE{X: "not_equal"}, X: "not_equal"},
	}, {
		label: label + "StructE2",
		x:     &ts.StructE2{StructE: &ts.StructE{X: "NotEqual"}, X: "NotEqual"},
		y:     &ts.StructE2{StructE: &ts.StructE{X: "not_equal"}, X: "not_equal"},
	}, {
		label: label + "StructF2",
		x:     ts.StructF2{StructF: &ts.StructF{X: "NotEqual"}, X: "NotEqual"},
		y:     ts.StructF2{StructF: &ts.StructF{X: "not_equal"}, X: "not_equal"},
	}, {
		label: label + "StructF2",
		x:     &ts.StructF2{StructF: &ts.StructF{X: "NotEqual"}, X: "NotEqual"},
		y:     &ts.StructF2{StructF: &ts.StructF{X: "not_equal"}, X: "not_equal"},
	}, {
		label: label + "StructNo",
		x:     ts.StructNo{X: "NotEqual"},
		y:     ts.StructNo{X: "not_equal"},
		wantDiff: `
  teststructs.StructNo{
- 	X: "NotEqual",
+ 	X: "not_equal",
  }
`,
	}, {
		label: label + "AssignA",
		x:     ts.AssignA(func() int { return 0 }),
		y:     ts.AssignA(func() int { return 1 }),
	}, {
		label: label + "AssignB",
		x:     ts.AssignB(struct{ A int }{0}),
		y:     ts.AssignB(struct{ A int }{1}),
	}, {
		label: label + "AssignC",
		x:     ts.AssignC(make(chan bool)),
		y:     ts.AssignC(make(chan bool)),
	}, {
		label: label + "AssignD",
		x:     ts.AssignD(make(chan bool)),
		y:     ts.AssignD(make(chan bool)),
	}}
}

func project1Tests() []test {
	const label = "Project1"

	ignoreUnexported := cmpopts.IgnoreUnexported(
		ts.EagleImmutable{},
		ts.DreamerImmutable{},
		ts.SlapImmutable{},
		ts.GoatImmutable{},
		ts.DonkeyImmutable{},
		ts.LoveRadius{},
		ts.SummerLove{},
		ts.SummerLoveSummary{},
	)

	createEagle := func() ts.Eagle {
		return ts.Eagle{
			Name:   "eagle",
			Hounds: []string{"buford", "tannen"},
			Desc:   "some description",
			Dreamers: []ts.Dreamer{{}, {
				Name: "dreamer2",
				Animal: []interface{}{
					ts.Goat{
						Target: "corporation",
						Immutable: &ts.GoatImmutable{
							ID:      "southbay",
							State:   (*pb.Goat_States)(intPtr(5)),
							Started: now,
						},
					},
					ts.Donkey{},
				},
				Amoeba: 53,
			}},
			Slaps: []ts.Slap{{
				Name: "slapID",
				Args: &pb.MetaData{Stringer: pb.Stringer{X: "metadata"}},
				Immutable: &ts.SlapImmutable{
					ID:       "immutableSlap",
					MildSlap: true,
					Started:  now,
					LoveRadius: &ts.LoveRadius{
						Summer: &ts.SummerLove{
							Summary: &ts.SummerLoveSummary{
								Devices:    []string{"foo", "bar", "baz"},
								ChangeType: []pb.SummerType{1, 2, 3},
							},
						},
					},
				},
			}},
			Immutable: &ts.EagleImmutable{
				ID:          "eagleID",
				Birthday:    now,
				MissingCall: (*pb.Eagle_MissingCalls)(intPtr(55)),
			},
		}
	}

	return []test{{
		label: label,
		x: ts.Eagle{Slaps: []ts.Slap{{
			Args: &pb.MetaData{Stringer: pb.Stringer{X: "metadata"}},
		}}},
		y: ts.Eagle{Slaps: []ts.Slap{{
			Args: &pb.MetaData{Stringer: pb.Stringer{X: "metadata"}},
		}}},
		wantPanic: "cannot handle unexported field",
	}, {
		label: label,
		x: ts.Eagle{Slaps: []ts.Slap{{
			Args: &pb.MetaData{Stringer: pb.Stringer{X: "metadata"}},
		}}},
		y: ts.Eagle{Slaps: []ts.Slap{{
			Args: &pb.MetaData{Stringer: pb.Stringer{X: "metadata"}},
		}}},
		opts: []cmp.Option{cmp.Comparer(pb.Equal)},
	}, {
		label: label,
		x: ts.Eagle{Slaps: []ts.Slap{{}, {}, {}, {}, {
			Args: &pb.MetaData{Stringer: pb.Stringer{X: "metadata"}},
		}}},
		y: ts.Eagle{Slaps: []ts.Slap{{}, {}, {}, {}, {
			Args: &pb.MetaData{Stringer: pb.Stringer{X: "metadata2"}},
		}}},
		opts: []cmp.Option{cmp.Comparer(pb.Equal)},
		wantDiff: `
  teststructs.Eagle{
  	... // 4 identical fields
  	Dreamers: nil,
  	Prong:    0,
  	Slaps: []teststructs.Slap{
  		... // 2 identical elements
  		{},
  		{},
  		{
  			Name:     "",
  			Desc:     "",
  			DescLong: "",
- 			Args:     s"metadata",
+ 			Args:     s"metadata2",
  			Tense:    0,
  			Interval: 0,
  			... // 3 identical fields
  		},
  	},
  	StateGoverner: "",
  	PrankRating:   "",
  	... // 2 identical fields
  }
`,
	}, {
		label: label,
		x:     createEagle(),
		y:     createEagle(),
		opts:  []cmp.Option{ignoreUnexported, cmp.Comparer(pb.Equal)},
	}, {
		label: label,
		x: func() ts.Eagle {
			eg := createEagle()
			eg.Dreamers[1].Animal[0].(ts.Goat).Immutable.ID = "southbay2"
			eg.Dreamers[1].Animal[0].(ts.Goat).Immutable.State = (*pb.Goat_States)(intPtr(6))
			eg.Slaps[0].Immutable.MildSlap = false
			return eg
		}(),
		y: func() ts.Eagle {
			eg := createEagle()
			devs := eg.Slaps[0].Immutable.LoveRadius.Summer.Summary.Devices
			eg.Slaps[0].Immutable.LoveRadius.Summer.Summary.Devices = devs[:1]
			return eg
		}(),
		opts: []cmp.Option{ignoreUnexported, cmp.Comparer(pb.Equal)},
		wantDiff: `
  teststructs.Eagle{
  	... // 2 identical fields
  	Desc:     "some description",
  	DescLong: "",
  	Dreamers: []teststructs.Dreamer{
  		{},
  		{
  			... // 4 identical fields
  			ContSlaps:         nil,
  			ContSlapsInterval: 0,
  			Animal: []interface{}{
  				teststructs.Goat{
  					Target:     "corporation",
  					Slaps:      nil,
  					FunnyPrank: "",
  					Immutable: &teststructs.GoatImmutable{
- 						ID:      "southbay2",
+ 						ID:      "southbay",
- 						State:   &6,
+ 						State:   &5,
  						Started: s"2009-11-10 23:00:00 +0000 UTC",
  						Stopped: s"0001-01-01 00:00:00 +0000 UTC",
  						... // 1 ignored and 1 identical fields
  					},
  				},
  				teststructs.Donkey{},
  			},
  			Ornamental: false,
  			Amoeba:     53,
  			... // 5 identical fields
  		},
  	},
  	Prong: 0,
  	Slaps: []teststructs.Slap{
  		{
  			... // 6 identical fields
  			Homeland:   0x00,
  			FunnyPrank: "",
  			Immutable: &teststructs.SlapImmutable{
  				ID:          "immutableSlap",
  				Out:         nil,
- 				MildSlap:    false,
+ 				MildSlap:    true,
  				PrettyPrint: "",
  				State:       nil,
  				Started:     s"2009-11-10 23:00:00 +0000 UTC",
  				Stopped:     s"0001-01-01 00:00:00 +0000 UTC",
  				LastUpdate:  s"0001-01-01 00:00:00 +0000 UTC",
  				LoveRadius: &teststructs.LoveRadius{
  					Summer: &teststructs.SummerLove{
  						Summary: &teststructs.SummerLoveSummary{
  							Devices: []string{
  								"foo",
- 								"bar",
- 								"baz",
  							},
  							ChangeType: []testprotos.SummerType{1, 2, 3},
  							... // 1 ignored field
  						},
  						... // 1 ignored field
  					},
  					... // 1 ignored field
  				},
  				... // 1 ignored field
  			},
  		},
  	},
  	StateGoverner: "",
  	PrankRating:   "",
  	... // 2 identical fields
  }
`,
	}}
}

type germSorter []*pb.Germ

func (gs germSorter) Len() int           { return len(gs) }
func (gs germSorter) Less(i, j int) bool { return gs[i].String() < gs[j].String() }
func (gs germSorter) Swap(i, j int)      { gs[i], gs[j] = gs[j], gs[i] }

func project2Tests() []test {
	const label = "Project2"

	sortGerms := cmp.Transformer("Sort", func(in []*pb.Germ) []*pb.Germ {
		out := append([]*pb.Germ(nil), in...) // Make copy
		sort.Sort(germSorter(out))
		return out
	})

	equalDish := cmp.Comparer(func(x, y *ts.Dish) bool {
		if x == nil || y == nil {
			return x == nil && y == nil
		}
		px, err1 := x.Proto()
		py, err2 := y.Proto()
		if err1 != nil || err2 != nil {
			return err1 == err2
		}
		return pb.Equal(px, py)
	})

	createBatch := func() ts.GermBatch {
		return ts.GermBatch{
			DirtyGerms: map[int32][]*pb.Germ{
				17: {
					{Stringer: pb.Stringer{X: "germ1"}},
				},
				18: {
					{Stringer: pb.Stringer{X: "germ2"}},
					{Stringer: pb.Stringer{X: "germ3"}},
					{Stringer: pb.Stringer{X: "germ4"}},
				},
			},
			GermMap: map[int32]*pb.Germ{
				13: {Stringer: pb.Stringer{X: "germ13"}},
				21: {Stringer: pb.Stringer{X: "germ21"}},
			},
			DishMap: map[int32]*ts.Dish{
				0: ts.CreateDish(nil, io.EOF),
				1: ts.CreateDish(nil, io.ErrUnexpectedEOF),
				2: ts.CreateDish(&pb.Dish{Stringer: pb.Stringer{X: "dish"}}, nil),
			},
			HasPreviousResult: true,
			DirtyID:           10,
			GermStrain:        421,
			InfectedAt:        now,
		}
	}

	return []test{{
		label:     label,
		x:         createBatch(),
		y:         createBatch(),
		wantPanic: "cannot handle unexported field",
	}, {
		label: label,
		x:     createBatch(),
		y:     createBatch(),
		opts:  []cmp.Option{cmp.Comparer(pb.Equal), sortGerms, equalDish},
	}, {
		label: label,
		x:     createBatch(),
		y: func() ts.GermBatch {
			gb := createBatch()
			s := gb.DirtyGerms[18]
			s[0], s[1], s[2] = s[1], s[2], s[0]
			return gb
		}(),
		opts: []cmp.Option{cmp.Comparer(pb.Equal), equalDish},
		wantDiff: `
  teststructs.GermBatch{
  	DirtyGerms: map[int32][]*testprotos.Germ{
  		17: {s"germ1"},
  		18: {
- 			s"germ2",
  			s"germ3",
  			s"germ4",
+ 			s"germ2",
  		},
  	},
  	CleanGerms: nil,
  	GermMap:    map[int32]*testprotos.Germ{13: s"germ13", 21: s"germ21"},
  	... // 7 identical fields
  }
`,
	}, {
		label: label,
		x:     createBatch(),
		y: func() ts.GermBatch {
			gb := createBatch()
			s := gb.DirtyGerms[18]
			s[0], s[1], s[2] = s[1], s[2], s[0]
			return gb
		}(),
		opts: []cmp.Option{cmp.Comparer(pb.Equal), sortGerms, equalDish},
	}, {
		label: label,
		x: func() ts.GermBatch {
			gb := createBatch()
			delete(gb.DirtyGerms, 17)
			gb.DishMap[1] = nil
			return gb
		}(),
		y: func() ts.GermBatch {
			gb := createBatch()
			gb.DirtyGerms[18] = gb.DirtyGerms[18][:2]
			gb.GermStrain = 22
			return gb
		}(),
		opts: []cmp.Option{cmp.Comparer(pb.Equal), sortGerms, equalDish},
		wantDiff: `
  teststructs.GermBatch{
  	DirtyGerms: map[int32][]*testprotos.Germ{
+ 		17: {s"germ1"},
  		18: Inverse(Sort, []*testprotos.Germ{
  			s"germ2",
  			s"germ3",
- 			s"germ4",
  		}),
  	},
  	CleanGerms: nil,
  	GermMap:    map[int32]*testprotos.Germ{13: s"germ13", 21: s"germ21"},
  	DishMap: map[int32]*teststructs.Dish{
  		0: &{err: &errors.errorString{s: "EOF"}},
- 		1: nil,
+ 		1: &{err: &errors.errorString{s: "unexpected EOF"}},
  		2: &{pb: &testprotos.Dish{Stringer: testprotos.Stringer{X: "dish"}}},
  	},
  	HasPreviousResult: true,
  	DirtyID:           10,
  	CleanID:           0,
- 	GermStrain:        421,
+ 	GermStrain:        22,
  	TotalDirtyGerms:   0,
  	InfectedAt:        s"2009-11-10 23:00:00 +0000 UTC",
  }
`,
	}}
}

func project3Tests() []test {
	const label = "Project3"

	allowVisibility := cmp.AllowUnexported(ts.Dirt{})

	ignoreLocker := cmpopts.IgnoreInterfaces(struct{ sync.Locker }{})

	transformProtos := cmp.Transformer("λ", func(x pb.Dirt) *pb.Dirt {
		return &x
	})

	equalTable := cmp.Comparer(func(x, y ts.Table) bool {
		tx, ok1 := x.(*ts.MockTable)
		ty, ok2 := y.(*ts.MockTable)
		if !ok1 || !ok2 {
			panic("table type must be MockTable")
		}
		return cmp.Equal(tx.State(), ty.State())
	})

	createDirt := func() (d ts.Dirt) {
		d.SetTable(ts.CreateMockTable([]string{"a", "b", "c"}))
		d.SetTimestamp(12345)
		d.Discord = 554
		d.Proto = pb.Dirt{Stringer: pb.Stringer{X: "proto"}}
		d.SetWizard(map[string]*pb.Wizard{
			"harry": {Stringer: pb.Stringer{X: "potter"}},
			"albus": {Stringer: pb.Stringer{X: "dumbledore"}},
		})
		d.SetLastTime(54321)
		return d
	}

	return []test{{
		label:     label,
		x:         createDirt(),
		y:         createDirt(),
		wantPanic: "cannot handle unexported field",
	}, {
		label:     label,
		x:         createDirt(),
		y:         createDirt(),
		opts:      []cmp.Option{allowVisibility, ignoreLocker, cmp.Comparer(pb.Equal), equalTable},
		wantPanic: "cannot handle unexported field",
	}, {
		label: label,
		x:     createDirt(),
		y:     createDirt(),
		opts:  []cmp.Option{allowVisibility, transformProtos, ignoreLocker, cmp.Comparer(pb.Equal), equalTable},
	}, {
		label: label,
		x: func() ts.Dirt {
			d := createDirt()
			d.SetTable(ts.CreateMockTable([]string{"a", "c"}))
			d.Proto = pb.Dirt{Stringer: pb.Stringer{X: "blah"}}
			return d
		}(),
		y: func() ts.Dirt {
			d := createDirt()
			d.Discord = 500
			d.SetWizard(map[string]*pb.Wizard{
				"harry": {Stringer: pb.Stringer{X: "otter"}},
			})
			return d
		}(),
		opts: []cmp.Option{allowVisibility, transformProtos, ignoreLocker, cmp.Comparer(pb.Equal), equalTable},
		wantDiff: `
  teststructs.Dirt{
- 	table:   &teststructs.MockTable{state: []string{"a", "c"}},
+ 	table:   &teststructs.MockTable{state: []string{"a", "b", "c"}},
  	ts:      12345,
- 	Discord: 554,
+ 	Discord: 500,
- 	Proto:   testprotos.Dirt(Inverse(λ, s"blah")),
+ 	Proto:   testprotos.Dirt(Inverse(λ, s"proto")),
  	wizard: map[string]*testprotos.Wizard{
- 		"albus": s"dumbledore",
- 		"harry": s"potter",
+ 		"harry": s"otter",
  	},
  	sadistic: nil,
  	lastTime: 54321,
  	... // 1 ignored field
  }
`,
	}}
}

func project4Tests() []test {
	const label = "Project4"

	allowVisibility := cmp.AllowUnexported(
		ts.Cartel{},
		ts.Headquarter{},
		ts.Poison{},
	)

	transformProtos := cmp.Transformer("λ", func(x pb.Restrictions) *pb.Restrictions {
		return &x
	})

	createCartel := func() ts.Cartel {
		var p ts.Poison
		p.SetPoisonType(5)
		p.SetExpiration(now)
		p.SetManufacturer("acme")

		var hq ts.Headquarter
		hq.SetID(5)
		hq.SetLocation("moon")
		hq.SetSubDivisions([]string{"alpha", "bravo", "charlie"})
		hq.SetMetaData(&pb.MetaData{Stringer: pb.Stringer{X: "metadata"}})
		hq.SetPublicMessage([]byte{1, 2, 3, 4, 5})
		hq.SetHorseBack("abcdef")
		hq.SetStatus(44)

		var c ts.Cartel
		c.Headquarter = hq
		c.SetSource("mars")
		c.SetCreationTime(now)
		c.SetBoss("al capone")
		c.SetPoisons([]*ts.Poison{&p})

		return c
	}

	return []test{{
		label:     label,
		x:         createCartel(),
		y:         createCartel(),
		wantPanic: "cannot handle unexported field",
	}, {
		label:     label,
		x:         createCartel(),
		y:         createCartel(),
		opts:      []cmp.Option{allowVisibility, cmp.Comparer(pb.Equal)},
		wantPanic: "cannot handle unexported field",
	}, {
		label: label,
		x:     createCartel(),
		y:     createCartel(),
		opts:  []cmp.Option{allowVisibility, transformProtos, cmp.Comparer(pb.Equal)},
	}, {
		label: label,
		x: func() ts.Cartel {
			d := createCartel()
			var p1, p2 ts.Poison
			p1.SetPoisonType(1)
			p1.SetExpiration(now)
			p1.SetManufacturer("acme")
			p2.SetPoisonType(2)
			p2.SetManufacturer("acme2")
			d.SetPoisons([]*ts.Poison{&p1, &p2})
			return d
		}(),
		y: func() ts.Cartel {
			d := createCartel()
			d.SetSubDivisions([]string{"bravo", "charlie"})
			d.SetPublicMessage([]byte{1, 2, 4, 3, 5})
			return d
		}(),
		opts: []cmp.Option{allowVisibility, transformProtos, cmp.Comparer(pb.Equal)},
		wantDiff: `
  teststructs.Cartel{
  	Headquarter: teststructs.Headquarter{
  		id:       0x05,
  		location: "moon",
  		subDivisions: []string{
- 			"alpha",
  			"bravo",
  			"charlie",
  		},
  		incorporatedDate: s"0001-01-01 00:00:00 +0000 UTC",
  		metaData:         s"metadata",
  		privateMessage:   nil,
  		publicMessage: []uint8{
  			0x01,
  			0x02,
- 			0x03,
+ 			0x04,
- 			0x04,
+ 			0x03,
  			0x05,
  		},
  		horseBack: "abcdef",
  		rattle:    "",
  		... // 5 identical fields
  	},
  	source:        "mars",
  	creationDate:  s"0001-01-01 00:00:00 +0000 UTC",
  	boss:          "al capone",
  	lastCrimeDate: s"0001-01-01 00:00:00 +0000 UTC",
  	poisons: []*teststructs.Poison{
  		&{
- 			poisonType:   1,
+ 			poisonType:   5,
  			expiration:   s"2009-11-10 23:00:00 +0000 UTC",
  			manufacturer: "acme",
  			potency:      0,
  		},
- 		&{poisonType: 2, manufacturer: "acme2"},
  	},
  }
`,
	}}
}

// BenchmarkBytes benchmarks the performance of performing Equal or Diff on
// large slices of bytes.
func BenchmarkBytes(b *testing.B) {
	// Create a list of PathFilters that never apply, but are evaluated.
	const maxFilters = 5
	var filters cmp.Options
	errorIface := reflect.TypeOf((*error)(nil)).Elem()
	for i := 0; i <= maxFilters; i++ {
		filters = append(filters, cmp.FilterPath(func(p cmp.Path) bool {
			return p.Last().Type().AssignableTo(errorIface) // Never true
		}, cmp.Ignore()))
	}

	type benchSize struct {
		label string
		size  int64
	}
	for _, ts := range []benchSize{
		{"4KiB", 1 << 12},
		{"64KiB", 1 << 16},
		{"1MiB", 1 << 20},
		{"16MiB", 1 << 24},
	} {
		bx := append(append(make([]byte, ts.size/2), 'x'), make([]byte, ts.size/2)...)
		by := append(append(make([]byte, ts.size/2), 'y'), make([]byte, ts.size/2)...)
		b.Run(ts.label, func(b *testing.B) {
			// Iteratively add more filters that never apply, but are evaluated
			// to measure the cost of simply evaluating each filter.
			for i := 0; i <= maxFilters; i++ {
				b.Run(fmt.Sprintf("EqualFilter%d", i), func(b *testing.B) {
					b.ReportAllocs()
					b.SetBytes(2 * ts.size)
					for j := 0; j < b.N; j++ {
						cmp.Equal(bx, by, filters[:i]...)
					}
				})
			}
			for i := 0; i <= maxFilters; i++ {
				b.Run(fmt.Sprintf("DiffFilter%d", i), func(b *testing.B) {
					b.ReportAllocs()
					b.SetBytes(2 * ts.size)
					for j := 0; j < b.N; j++ {
						cmp.Diff(bx, by, filters[:i]...)
					}
				})
			}
		})
	}
}
