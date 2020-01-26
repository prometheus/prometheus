// Copyright 2016 The Oklog Authors
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

package ulid_test

import (
	"bytes"
	crand "crypto/rand"
	"fmt"
	"io"
	"math"
	"math/rand"
	"strings"
	"testing"
	"testing/iotest"
	"testing/quick"
	"time"

	"github.com/oklog/ulid"
)

func ExampleULID() {
	t := time.Unix(1000000, 0)
	entropy := ulid.Monotonic(rand.New(rand.NewSource(t.UnixNano())), 0)
	fmt.Println(ulid.MustNew(ulid.Timestamp(t), entropy))
	// Output: 0000XSNJG0MQJHBF4QX1EFD6Y3
}

func TestNew(t *testing.T) {
	t.Parallel()

	t.Run("ULID", testULID(func(ms uint64, e io.Reader) ulid.ULID {
		id, err := ulid.New(ms, e)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}))

	t.Run("Error", func(t *testing.T) {
		_, err := ulid.New(ulid.MaxTime()+1, nil)
		if got, want := err, ulid.ErrBigTime; got != want {
			t.Errorf("got err %v, want %v", got, want)
		}

		_, err = ulid.New(0, strings.NewReader(""))
		if got, want := err, io.EOF; got != want {
			t.Errorf("got err %v, want %v", got, want)
		}
	})
}

func TestMustNew(t *testing.T) {
	t.Parallel()

	t.Run("ULID", testULID(ulid.MustNew))

	t.Run("Panic", func(t *testing.T) {
		defer func() {
			if got, want := recover(), io.EOF; got != want {
				t.Errorf("panic with err %v, want %v", got, want)
			}
		}()
		_ = ulid.MustNew(0, strings.NewReader(""))
	})
}

func TestMustParse(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		fn   func(string) ulid.ULID
	}{
		{"MustParse", ulid.MustParse},
		{"MustParseStrict", ulid.MustParseStrict},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if got, want := recover(), ulid.ErrDataSize; got != want {
					t.Errorf("got panic %v, want %v", got, want)
				}
			}()
			_ = tc.fn("")
		})

	}
}

func testULID(mk func(uint64, io.Reader) ulid.ULID) func(*testing.T) {
	return func(t *testing.T) {
		want := ulid.ULID{0x0, 0x0, 0x0, 0x1, 0x86, 0xa0}
		if got := mk(1e5, nil); got != want { // optional entropy
			t.Errorf("\ngot  %#v\nwant %#v", got, want)
		}

		entropy := bytes.Repeat([]byte{0xFF}, 16)
		copy(want[6:], entropy)
		if got := mk(1e5, bytes.NewReader(entropy)); got != want {
			t.Errorf("\ngot  %#v\nwant %#v", got, want)
		}
	}
}

func TestRoundTrips(t *testing.T) {
	t.Parallel()

	prop := func(id ulid.ULID) bool {
		bin, err := id.MarshalBinary()
		if err != nil {
			t.Fatal(err)
		}

		txt, err := id.MarshalText()
		if err != nil {
			t.Fatal(err)
		}

		var a ulid.ULID
		if err = a.UnmarshalBinary(bin); err != nil {
			t.Fatal(err)
		}

		var b ulid.ULID
		if err = b.UnmarshalText(txt); err != nil {
			t.Fatal(err)
		}

		return id == a && b == id &&
			id == ulid.MustParse(id.String()) &&
			id == ulid.MustParseStrict(id.String())
	}

	err := quick.Check(prop, &quick.Config{MaxCount: 1E5})
	if err != nil {
		t.Fatal(err)
	}
}

func TestMarshalingErrors(t *testing.T) {
	t.Parallel()

	var id ulid.ULID
	for _, tc := range []struct {
		name string
		fn   func([]byte) error
		err  error
	}{
		{"UnmarshalBinary", id.UnmarshalBinary, ulid.ErrDataSize},
		{"UnmarshalText", id.UnmarshalText, ulid.ErrDataSize},
		{"MarshalBinaryTo", id.MarshalBinaryTo, ulid.ErrBufferSize},
		{"MarshalTextTo", id.MarshalTextTo, ulid.ErrBufferSize},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got, want := tc.fn([]byte{}), tc.err; got != want {
				t.Errorf("got err %v, want %v", got, want)
			}
		})

	}
}

func TestParseStrictInvalidCharacters(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name  string
		input string
	}
	testCases := []testCase{}
	base := "0000XSNJG0MQJHBF4QX1EFD6Y3"
	for i := 0; i < ulid.EncodedSize; i++ {
		testCases = append(testCases, testCase{
			name:  fmt.Sprintf("Invalid 0xFF at index %d", i),
			input: base[:i] + "\xff" + base[i+1:],
		})
		testCases = append(testCases, testCase{
			name:  fmt.Sprintf("Invalid 0x00 at index %d", i),
			input: base[:i] + "\x00" + base[i+1:],
		})
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ulid.ParseStrict(tt.input)
			if err != ulid.ErrInvalidCharacters {
				t.Errorf("Parse(%q): got err %v, want %v", tt.input, err, ulid.ErrInvalidCharacters)
			}
		})
	}
}

func TestAlizainCompatibility(t *testing.T) {
	t.Parallel()

	ts := uint64(1469918176385)
	got := ulid.MustNew(ts, bytes.NewReader(make([]byte, 16)))
	want := ulid.MustParse("01ARYZ6S410000000000000000")
	if got != want {
		t.Fatalf("with time=%d, got %q, want %q", ts, got, want)
	}
}

func TestEncoding(t *testing.T) {
	t.Parallel()

	enc := make(map[rune]bool, len(ulid.Encoding))
	for _, r := range ulid.Encoding {
		enc[r] = true
	}

	prop := func(id ulid.ULID) bool {
		for _, r := range id.String() {
			if !enc[r] {
				return false
			}
		}
		return true
	}

	if err := quick.Check(prop, &quick.Config{MaxCount: 1E5}); err != nil {
		t.Fatal(err)
	}
}

func TestLexicographicalOrder(t *testing.T) {
	t.Parallel()

	prop := func(a, b ulid.ULID) bool {
		t1, t2 := a.Time(), b.Time()
		s1, s2 := a.String(), b.String()
		ord := bytes.Compare(a[:], b[:])
		return t1 == t2 ||
			(t1 > t2 && s1 > s2 && ord == +1) ||
			(t1 < t2 && s1 < s2 && ord == -1)
	}

	top := ulid.MustNew(ulid.MaxTime(), nil)
	for i := 0; i < 10; i++ { // test upper boundary state space
		next := ulid.MustNew(top.Time()-1, nil)
		if !prop(top, next) {
			t.Fatalf("bad lexicographical order: (%v, %q) > (%v, %q) == false",
				top.Time(), top,
				next.Time(), next,
			)
		}
		top = next
	}

	if err := quick.Check(prop, &quick.Config{MaxCount: 1E6}); err != nil {
		t.Fatal(err)
	}
}

func TestCaseInsensitivity(t *testing.T) {
	t.Parallel()

	upper := func(id ulid.ULID) (out ulid.ULID) {
		return ulid.MustParse(strings.ToUpper(id.String()))
	}

	lower := func(id ulid.ULID) (out ulid.ULID) {
		return ulid.MustParse(strings.ToLower(id.String()))
	}

	err := quick.CheckEqual(upper, lower, nil)
	if err != nil {
		t.Fatal(err)
	}
}

func TestParseRobustness(t *testing.T) {
	t.Parallel()

	cases := [][]byte{
		{0x1, 0xc0, 0x73, 0x62, 0x4a, 0xaf, 0x39, 0x78, 0x51, 0x4e, 0xf8, 0x44, 0x3b,
			0xb2, 0xa8, 0x59, 0xc7, 0x5f, 0xc3, 0xcc, 0x6a, 0xf2, 0x6d, 0x5a, 0xaa, 0x20},
	}

	for _, tc := range cases {
		if _, err := ulid.Parse(string(tc)); err != nil {
			t.Error(err)
		}
	}

	prop := func(s [26]byte) (ok bool) {
		defer func() {
			if err := recover(); err != nil {
				t.Error(err)
				ok = false
			}
		}()

		// quick.Check doesn't constrain input,
		// so we need to do so artificially.
		if s[0] > '7' {
			s[0] %= '7'
		}

		var err error
		if _, err = ulid.Parse(string(s[:])); err != nil {
			t.Error(err)
		}

		return err == nil
	}

	err := quick.Check(prop, &quick.Config{MaxCount: 1E4})
	if err != nil {
		t.Fatal(err)
	}
}

func TestNow(t *testing.T) {
	t.Parallel()

	before := ulid.Now()
	after := ulid.Timestamp(time.Now().UTC().Add(time.Millisecond))

	if before >= after {
		t.Fatalf("clock went mad: before %v, after %v", before, after)
	}
}

func TestTimestamp(t *testing.T) {
	t.Parallel()

	tm := time.Unix(1, 1000) // will be truncated
	if got, want := ulid.Timestamp(tm), uint64(1000); got != want {
		t.Errorf("for %v, got %v, want %v", tm, got, want)
	}

	mt := ulid.MaxTime()
	dt := time.Unix(int64(mt/1000), int64((mt%1000)*1000000)).Truncate(time.Millisecond)
	ts := ulid.Timestamp(dt)
	if got, want := ts, mt; got != want {
		t.Errorf("got timestamp %d, want %d", got, want)
	}
}

func TestTime(t *testing.T) {
	t.Parallel()

	original := time.Now()
	diff := original.Sub(ulid.Time(ulid.Timestamp(original)))
	if diff >= time.Millisecond {
		t.Errorf("difference between original and recovered time (%d) greater"+
			"than a millisecond", diff)
	}

}

func TestTimestampRoundTrips(t *testing.T) {
	t.Parallel()

	prop := func(ts uint64) bool {
		return ts == ulid.Timestamp(ulid.Time(ts))
	}

	err := quick.Check(prop, &quick.Config{MaxCount: 1E5})
	if err != nil {
		t.Fatal(err)
	}
}

func TestULIDTime(t *testing.T) {
	t.Parallel()

	maxTime := ulid.MaxTime()

	var id ulid.ULID
	if got, want := id.SetTime(maxTime+1), ulid.ErrBigTime; got != want {
		t.Errorf("got err %v, want %v", got, want)
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := 0; i < 1e6; i++ {
		ms := uint64(rng.Int63n(int64(maxTime)))

		var id ulid.ULID
		if err := id.SetTime(ms); err != nil {
			t.Fatal(err)
		}

		if got, want := id.Time(), ms; got != want {
			t.Fatalf("\nfor %v:\ngot  %v\nwant %v", id, got, want)
		}
	}
}

func TestEntropy(t *testing.T) {
	t.Parallel()

	var id ulid.ULID
	if got, want := id.SetEntropy([]byte{}), ulid.ErrDataSize; got != want {
		t.Errorf("got err %v, want %v", got, want)
	}

	prop := func(e [10]byte) bool {
		var id ulid.ULID
		if err := id.SetEntropy(e[:]); err != nil {
			t.Fatalf("got err %v", err)
		}

		got, want := id.Entropy(), e[:]
		eq := bytes.Equal(got, want)
		if !eq {
			t.Errorf("\n(!= %v\n    %v)", got, want)
		}

		return eq
	}

	if err := quick.Check(prop, nil); err != nil {
		t.Fatal(err)
	}
}

func TestEntropyRead(t *testing.T) {
	t.Parallel()

	prop := func(e [10]byte) bool {
		flakyReader := iotest.HalfReader(bytes.NewReader(e[:]))

		id, err := ulid.New(ulid.Now(), flakyReader)
		if err != nil {
			t.Fatalf("got err %v", err)
		}

		got, want := id.Entropy(), e[:]
		eq := bytes.Equal(got, want)
		if !eq {
			t.Errorf("\n(!= %v\n    %v)", got, want)
		}

		return eq
	}

	if err := quick.Check(prop, &quick.Config{MaxCount: 1E4}); err != nil {
		t.Fatal(err)
	}
}

func TestCompare(t *testing.T) {
	t.Parallel()

	a := func(a, b ulid.ULID) int {
		return strings.Compare(a.String(), b.String())
	}

	b := func(a, b ulid.ULID) int {
		return a.Compare(b)
	}

	err := quick.CheckEqual(a, b, &quick.Config{MaxCount: 1E5})
	if err != nil {
		t.Error(err)
	}
}

func TestOverflowHandling(t *testing.T) {
	for s, want := range map[string]error{
		"00000000000000000000000000": nil,
		"70000000000000000000000000": nil,
		"7ZZZZZZZZZZZZZZZZZZZZZZZZZ": nil,
		"80000000000000000000000000": ulid.ErrOverflow,
		"80000000000000000000000001": ulid.ErrOverflow,
		"ZZZZZZZZZZZZZZZZZZZZZZZZZZ": ulid.ErrOverflow,
	} {
		if _, have := ulid.Parse(s); want != have {
			t.Errorf("%s: want error %v, have %v", s, want, have)
		}
	}
}

func TestScan(t *testing.T) {
	id := ulid.MustNew(123, crand.Reader)

	for _, tc := range []struct {
		name string
		in   interface{}
		out  ulid.ULID
		err  error
	}{
		{"string", id.String(), id, nil},
		{"bytes", id[:], id, nil},
		{"nil", nil, ulid.ULID{}, nil},
		{"other", 44, ulid.ULID{}, ulid.ErrScanValue},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var out ulid.ULID
			err := out.Scan(tc.in)
			if got, want := out, tc.out; got.Compare(want) != 0 {
				t.Errorf("got ULID %s, want %s", got, want)
			}

			if got, want := fmt.Sprint(err), fmt.Sprint(tc.err); got != want {
				t.Errorf("got err %q, want %q", got, want)
			}
		})
	}
}

func TestMonotonic(t *testing.T) {
	now := ulid.Now()
	for _, e := range []struct {
		name string
		mk   func() io.Reader
	}{
		{"cryptorand", func() io.Reader { return crand.Reader }},
		{"mathrand", func() io.Reader { return rand.New(rand.NewSource(int64(now))) }},
	} {
		for _, inc := range []uint64{
			0,
			1,
			2,
			math.MaxUint8 + 1,
			math.MaxUint16 + 1,
			math.MaxUint32 + 1,
		} {
			inc := inc
			entropy := ulid.Monotonic(e.mk(), uint64(inc))

			t.Run(fmt.Sprintf("entropy=%s/inc=%d", e.name, inc), func(t *testing.T) {
				t.Parallel()

				var prev ulid.ULID
				for i := 0; i < 10000; i++ {
					next, err := ulid.New(123, entropy)
					if err != nil {
						t.Fatal(err)
					}

					if prev.Compare(next) >= 0 {
						t.Fatalf("prev: %v %v > next: %v %v",
							prev.Time(), prev.Entropy(), next.Time(), next.Entropy())
					}

					prev = next
				}
			})
		}
	}
}

func TestMonotonicOverflow(t *testing.T) {
	t.Parallel()

	entropy := ulid.Monotonic(
		io.MultiReader(
			bytes.NewReader(bytes.Repeat([]byte{0xFF}, 10)), // Entropy for first ULID
			crand.Reader, // Following random entropy
		),
		0,
	)

	prev, err := ulid.New(0, entropy)
	if err != nil {
		t.Fatal(err)
	}

	next, err := ulid.New(prev.Time(), entropy)
	if have, want := err, ulid.ErrMonotonicOverflow; have != want {
		t.Errorf("have ulid: %v %v err: %v, want err: %v",
			next.Time(), next.Entropy(), have, want)
	}
}

func BenchmarkNew(b *testing.B) {
	benchmarkMakeULID(b, func(timestamp uint64, entropy io.Reader) {
		_, _ = ulid.New(timestamp, entropy)
	})
}

func BenchmarkMustNew(b *testing.B) {
	benchmarkMakeULID(b, func(timestamp uint64, entropy io.Reader) {
		_ = ulid.MustNew(timestamp, entropy)
	})
}

func benchmarkMakeULID(b *testing.B, f func(uint64, io.Reader)) {
	b.ReportAllocs()
	b.SetBytes(int64(len(ulid.ULID{})))

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	for _, tc := range []struct {
		name       string
		timestamps []uint64
		entropy    io.Reader
	}{
		{"WithCrypoEntropy", []uint64{123}, crand.Reader},
		{"WithEntropy", []uint64{123}, rng},
		{"WithMonotonicEntropy_SameTimestamp_Inc0", []uint64{123}, ulid.Monotonic(rng, 0)},
		{"WithMonotonicEntropy_DifferentTimestamp_Inc0", []uint64{122, 123}, ulid.Monotonic(rng, 0)},
		{"WithMonotonicEntropy_SameTimestamp_Inc1", []uint64{123}, ulid.Monotonic(rng, 1)},
		{"WithMonotonicEntropy_DifferentTimestamp_Inc1", []uint64{122, 123}, ulid.Monotonic(rng, 1)},
		{"WithCryptoMonotonicEntropy_SameTimestamp_Inc1", []uint64{123}, ulid.Monotonic(crand.Reader, 1)},
		{"WithCryptoMonotonicEntropy_DifferentTimestamp_Inc1", []uint64{122, 123}, ulid.Monotonic(crand.Reader, 1)},
		{"WithoutEntropy", []uint64{123}, nil},
	} {
		tc := tc
		b.Run(tc.name, func(b *testing.B) {
			b.StopTimer()
			b.ResetTimer()
			b.StartTimer()
			for i := 0; i < b.N; i++ {
				f(tc.timestamps[i%len(tc.timestamps)], tc.entropy)
			}
		})
	}
}

func BenchmarkParse(b *testing.B) {
	const s = "0000XSNJG0MQJHBF4QX1EFD6Y3"
	b.SetBytes(int64(len(s)))
	for i := 0; i < b.N; i++ {
		_, _ = ulid.Parse(s)
	}
}

func BenchmarkParseStrict(b *testing.B) {
	const s = "0000XSNJG0MQJHBF4QX1EFD6Y3"
	b.SetBytes(int64(len(s)))
	for i := 0; i < b.N; i++ {
		_, _ = ulid.ParseStrict(s)
	}
}

func BenchmarkMustParse(b *testing.B) {
	const s = "0000XSNJG0MQJHBF4QX1EFD6Y3"
	b.SetBytes(int64(len(s)))
	for i := 0; i < b.N; i++ {
		_ = ulid.MustParse(s)
	}
}

func BenchmarkString(b *testing.B) {
	entropy := rand.New(rand.NewSource(time.Now().UnixNano()))
	id := ulid.MustNew(123456, entropy)
	b.SetBytes(int64(len(id)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = id.String()
	}
}

func BenchmarkMarshal(b *testing.B) {
	entropy := rand.New(rand.NewSource(time.Now().UnixNano()))
	buf := make([]byte, ulid.EncodedSize)
	id := ulid.MustNew(123456, entropy)

	b.Run("Text", func(b *testing.B) {
		b.SetBytes(int64(len(id)))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = id.MarshalText()
		}
	})

	b.Run("TextTo", func(b *testing.B) {
		b.SetBytes(int64(len(id)))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = id.MarshalTextTo(buf)
		}
	})

	b.Run("Binary", func(b *testing.B) {
		b.SetBytes(int64(len(id)))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = id.MarshalBinary()
		}
	})

	b.Run("BinaryTo", func(b *testing.B) {
		b.SetBytes(int64(len(id)))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = id.MarshalBinaryTo(buf)
		}
	})
}

func BenchmarkUnmarshal(b *testing.B) {
	var id ulid.ULID
	s := "0000XSNJG0MQJHBF4QX1EFD6Y3"
	txt := []byte(s)
	bin, _ := ulid.MustParse(s).MarshalBinary()

	b.Run("Text", func(b *testing.B) {
		b.SetBytes(int64(len(txt)))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = id.UnmarshalText(txt)
		}
	})

	b.Run("Binary", func(b *testing.B) {
		b.SetBytes(int64(len(bin)))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = id.UnmarshalBinary(bin)
		}
	})
}

func BenchmarkNow(b *testing.B) {
	b.SetBytes(8)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ulid.Now()
	}
}

func BenchmarkTimestamp(b *testing.B) {
	now := time.Now()
	b.SetBytes(8)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ulid.Timestamp(now)
	}
}

func BenchmarkTime(b *testing.B) {
	id := ulid.MustNew(123456789, nil)
	b.SetBytes(8)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = id.Time()
	}
}

func BenchmarkSetTime(b *testing.B) {
	var id ulid.ULID
	b.SetBytes(8)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = id.SetTime(123456789)
	}
}

func BenchmarkEntropy(b *testing.B) {
	id := ulid.MustNew(0, strings.NewReader("ABCDEFGHIJKLMNOP"))
	b.SetBytes(10)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = id.Entropy()
	}
}

func BenchmarkSetEntropy(b *testing.B) {
	var id ulid.ULID
	e := []byte("ABCDEFGHIJKLMNOP")
	b.SetBytes(10)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = id.SetEntropy(e)
	}
}

func BenchmarkCompare(b *testing.B) {
	id, other := ulid.MustNew(12345, nil), ulid.MustNew(54321, nil)
	b.SetBytes(int64(len(id) * 2))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = id.Compare(other)
	}
}
