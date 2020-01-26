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

package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"google.golang.org/api/option"
)

const readData = "0123456789"

func TestRangeReader(t *testing.T) {
	hc, close := newTestServer(handleRangeRead)
	defer close()
	ctx := context.Background()
	c, err := NewClient(ctx, option.WithHTTPClient(hc))
	if err != nil {
		t.Fatal(err)
	}
	obj := c.Bucket("b").Object("o")
	for _, test := range []struct {
		offset, length int64
		want           string
	}{
		{0, -1, readData},
		{0, 10, readData},
		{0, 5, readData[:5]},
		{1, 3, readData[1:4]},
		{6, -1, readData[6:]},
		{4, 20, readData[4:]},
	} {
		r, err := obj.NewRangeReader(ctx, test.offset, test.length)
		if err != nil {
			t.Errorf("%d/%d: %v", test.offset, test.length, err)
			continue
		}
		gotb, err := ioutil.ReadAll(r)
		if err != nil {
			t.Errorf("%d/%d: %v", test.offset, test.length, err)
			continue
		}
		if got := string(gotb); got != test.want {
			t.Errorf("%d/%d: got %q, want %q", test.offset, test.length, got, test.want)
		}
	}
}

func handleRangeRead(w http.ResponseWriter, r *http.Request) {
	rh := strings.TrimSpace(r.Header.Get("Range"))
	data := readData
	var from, to int
	if rh == "" {
		from = 0
		to = len(data)
	} else {
		// assume "bytes=N-" or "bytes=N-M"
		var err error
		i := strings.IndexRune(rh, '=')
		j := strings.IndexRune(rh, '-')
		from, err = strconv.Atoi(rh[i+1 : j])
		if err != nil {
			w.WriteHeader(500)
			return
		}
		to = len(data)
		if j+1 < len(rh) {
			to, err = strconv.Atoi(rh[j+1:])
			if err != nil {
				w.WriteHeader(500)
				return
			}
			to++ // Range header is inclusive, Go slice is exclusive
		}
		if from >= len(data) && to != from {
			w.WriteHeader(416)
			return
		}
		if from > len(data) {
			from = len(data)
		}
		if to > len(data) {
			to = len(data)
		}
	}
	data = data[from:to]
	if data != readData {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", from, to-1, len(readData)))
		w.WriteHeader(http.StatusPartialContent)
	}
	if _, err := w.Write([]byte(data)); err != nil {
		panic(err)
	}
}

type http2Error string

func (h http2Error) Error() string {
	return string(h)
}

func TestRangeReaderRetry(t *testing.T) {
	retryErr := http2Error("blah blah INTERNAL_ERROR")
	readBytes := []byte(readData)
	hc, close := newTestServer(handleRangeRead)
	defer close()
	ctx := context.Background()
	c, err := NewClient(ctx, option.WithHTTPClient(hc))
	if err != nil {
		t.Fatal(err)
	}

	obj := c.Bucket("b").Object("o")
	for i, test := range []struct {
		offset, length int64
		bodies         []fakeReadCloser
		want           string
	}{
		{
			offset: 0,
			length: -1,
			bodies: []fakeReadCloser{
				{data: readBytes, counts: []int{10}, err: io.EOF},
			},
			want: readData,
		},
		{
			offset: 0,
			length: -1,
			bodies: []fakeReadCloser{
				{data: readBytes, counts: []int{3}, err: retryErr},
				{data: readBytes[3:], counts: []int{5, 2}, err: io.EOF},
			},
			want: readData,
		},
		{
			offset: 0,
			length: -1,
			bodies: []fakeReadCloser{
				{data: readBytes, counts: []int{5}, err: retryErr},
				{data: readBytes[5:], counts: []int{1, 3}, err: retryErr},
				{data: readBytes[9:], counts: []int{1}, err: io.EOF},
			},
			want: readData,
		},
		{
			offset: 0,
			length: 5,
			bodies: []fakeReadCloser{
				{data: readBytes, counts: []int{3}, err: retryErr},
				{data: readBytes[3:], counts: []int{2}, err: io.EOF},
			},
			want: readData[:5],
		},
		{
			offset: 1,
			length: 5,
			bodies: []fakeReadCloser{
				{data: readBytes, counts: []int{3}, err: retryErr},
				{data: readBytes[3:], counts: []int{2}, err: io.EOF},
			},
			want: readData[:5],
		},
		{
			offset: 1,
			length: 3,
			bodies: []fakeReadCloser{
				{data: readBytes[1:], counts: []int{1}, err: retryErr},
				{data: readBytes[2:], counts: []int{2}, err: io.EOF},
			},
			want: readData[1:4],
		},
		{
			offset: 4,
			length: -1,
			bodies: []fakeReadCloser{
				{data: readBytes[4:], counts: []int{1}, err: retryErr},
				{data: readBytes[5:], counts: []int{4}, err: retryErr},
				{data: readBytes[9:], counts: []int{1}, err: io.EOF},
			},
			want: readData[4:],
		},
	} {
		r, err := obj.NewRangeReader(ctx, test.offset, test.length)
		if err != nil {
			t.Errorf("#%d: %v", i, err)
			continue
		}
		r.body = &test.bodies[0]
		b := 0
		r.reopen = func(int64) (*http.Response, error) {
			b++
			return &http.Response{Body: &test.bodies[b]}, nil
		}
		buf := make([]byte, len(readData)/2)
		var gotb []byte
		for {
			n, err := r.Read(buf)
			gotb = append(gotb, buf[:n]...)
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("#%d: %v", i, err)
			}
		}
		if err != nil {
			t.Errorf("#%d: %v", i, err)
			continue
		}
		if got := string(gotb); got != test.want {
			t.Errorf("#%d: got %q, want %q", i, got, test.want)
		}
	}
}

type fakeReadCloser struct {
	data   []byte
	counts []int // how much of data to deliver on each read
	err    error // error to return with last count

	d int // current position in data
	c int // current position in counts
}

func (f *fakeReadCloser) Close() error {
	return nil
}

func (f *fakeReadCloser) Read(buf []byte) (int, error) {
	i := f.c
	n := 0
	if i < len(f.counts) {
		n = f.counts[i]
	}
	var err error
	if i >= len(f.counts)-1 {
		err = f.err
	}
	copy(buf, f.data[f.d:f.d+n])
	if len(buf) < n {
		n = len(buf)
		f.counts[i] -= n
		err = nil
	} else {
		f.c++
	}
	f.d += n
	return n, err
}

func TestFakeReadCloser(t *testing.T) {
	e := errors.New("")
	f := &fakeReadCloser{
		data:   []byte(readData),
		counts: []int{1, 2, 3},
		err:    e,
	}
	wants := []string{"0", "12", "345"}
	buf := make([]byte, 10)
	for i := 0; i < 3; i++ {
		n, err := f.Read(buf)
		if got, want := n, f.counts[i]; got != want {
			t.Fatalf("i=%d: got %d, want %d", i, got, want)
		}
		var wantErr error
		if i == 2 {
			wantErr = e
		}
		if err != wantErr {
			t.Fatalf("i=%d: got error %v, want %v", i, err, wantErr)
		}
		if got, want := string(buf[:n]), wants[i]; got != want {
			t.Fatalf("i=%d: got %q, want %q", i, got, want)
		}
	}
}
