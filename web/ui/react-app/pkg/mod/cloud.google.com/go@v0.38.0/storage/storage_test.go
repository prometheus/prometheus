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

package storage

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/iam"
	"cloud.google.com/go/internal/testutil"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	raw "google.golang.org/api/storage/v1"
)

func TestHeaderSanitization(t *testing.T) {
	t.Parallel()
	var tests = []struct {
		desc string
		in   []string
		want []string
	}{
		{
			desc: "already sanitized headers should not be modified",
			in:   []string{"x-goog-header1:true", "x-goog-header2:0"},
			want: []string{"x-goog-header1:true", "x-goog-header2:0"},
		},
		{
			desc: "sanitized headers should be sorted",
			in:   []string{"x-goog-header2:0", "x-goog-header1:true"},
			want: []string{"x-goog-header1:true", "x-goog-header2:0"},
		},
		{
			desc: "non-canonical headers should be removed",
			in:   []string{"x-goog-header1:true", "x-goog-no-value", "non-canonical-header:not-of-use"},
			want: []string{"x-goog-header1:true"},
		},
		{
			desc: "excluded canonical headers should be removed",
			in:   []string{"x-goog-header1:true", "x-goog-encryption-key:my_key", "x-goog-encryption-key-sha256:my_sha256"},
			want: []string{"x-goog-header1:true"},
		},
		{
			desc: "dirty headers should be formatted correctly",
			in:   []string{" x-goog-header1 : \textra-spaces ", "X-Goog-Header2:CamelCaseValue"},
			want: []string{"x-goog-header1:extra-spaces", "x-goog-header2:CamelCaseValue"},
		},
		{
			desc: "duplicate headers should be merged",
			in:   []string{"x-goog-header1:value1", "X-Goog-Header1:value2"},
			want: []string{"x-goog-header1:value1,value2"},
		},
	}
	for _, test := range tests {
		got := sanitizeHeaders(test.in)
		if !testutil.Equal(got, test.want) {
			t.Errorf("%s: got %v, want %v", test.desc, got, test.want)
		}
	}
}

func TestSignedURL(t *testing.T) {
	t.Parallel()
	expires, _ := time.Parse(time.RFC3339, "2002-10-02T10:00:00-05:00")
	url, err := SignedURL("bucket-name", "object-name", &SignedURLOptions{
		GoogleAccessID: "xxx@clientid",
		PrivateKey:     dummyKey("rsa"),
		Method:         "GET",
		MD5:            "ICy5YqxZB1uWSwcVLSNLcA==",
		Expires:        expires,
		ContentType:    "application/json",
		Headers:        []string{"x-goog-header1:true", "x-goog-header2:false"},
	})
	if err != nil {
		t.Error(err)
	}
	want := "https://storage.googleapis.com/bucket-name/object-name?" +
		"Expires=1033570800&GoogleAccessId=xxx%40clientid&Signature=" +
		"RfsHlPtbB2JUYjzCgNr2Mi%2BjggdEuL1V7E6N9o6aaqwVLBDuTv3I0%2B9" +
		"x94E6rmmr%2FVgnmZigkIUxX%2Blfl7LgKf30uPGLt0mjKGH2p7r9ey1ONJ" +
		"%2BhVec23FnTRcSgopglvHPuCMWU2oNJE%2F1y8EwWE27baHrG1RhRHbLVF" +
		"bPpLZ9xTRFK20pluIkfHV00JGljB1imqQHXM%2B2XPWqBngLr%2FwqxLN7i" +
		"FcUiqR8xQEOHF%2F2e7fbkTHPNq4TazaLZ8X0eZ3eFdJ55A5QmNi8atlN4W" +
		"5q7Hvs0jcxElG3yqIbx439A995BkspLiAcA%2Fo4%2BxAwEMkGLICdbvakq" +
		"3eEprNCojw%3D%3D"
	if url != want {
		t.Fatalf("Unexpected signed URL; found %v", url)
	}
}

func TestSignedURL_PEMPrivateKey(t *testing.T) {
	t.Parallel()
	expires, _ := time.Parse(time.RFC3339, "2002-10-02T10:00:00-05:00")
	url, err := SignedURL("bucket-name", "object-name", &SignedURLOptions{
		GoogleAccessID: "xxx@clientid",
		PrivateKey:     dummyKey("pem"),
		Method:         "GET",
		MD5:            "ICy5YqxZB1uWSwcVLSNLcA==",
		Expires:        expires,
		ContentType:    "application/json",
		Headers:        []string{"x-goog-header1:true", "x-goog-header2:false"},
	})
	if err != nil {
		t.Error(err)
	}
	want := "https://storage.googleapis.com/bucket-name/object-name?" +
		"Expires=1033570800&GoogleAccessId=xxx%40clientid&Signature=" +
		"TiyKD%2FgGb6Kh0kkb2iF%2FfF%2BnTx7L0J4YiZua8AcTmnidutePEGIU5" +
		"NULYlrGl6l52gz4zqFb3VFfIRTcPXMdXnnFdMCDhz2QuJBUpsU1Ai9zlyTQ" +
		"dkb6ShG03xz9%2BEXWAUQO4GBybJw%2FULASuv37xA00SwLdkqj8YdyS5II" +
		"1lro%3D"
	if url != want {
		t.Fatalf("Unexpected signed URL; found %v", url)
	}
}

func TestSignedURL_SignBytes(t *testing.T) {
	t.Parallel()
	expires, _ := time.Parse(time.RFC3339, "2002-10-02T10:00:00-05:00")
	url, err := SignedURL("bucket-name", "object-name", &SignedURLOptions{
		GoogleAccessID: "xxx@clientid",
		SignBytes: func(b []byte) ([]byte, error) {
			return []byte("signed"), nil
		},
		Method:      "GET",
		MD5:         "ICy5YqxZB1uWSwcVLSNLcA==",
		Expires:     expires,
		ContentType: "application/json",
		Headers:     []string{"x-goog-header1:true", "x-goog-header2:false"},
	})
	if err != nil {
		t.Error(err)
	}
	want := "https://storage.googleapis.com/bucket-name/object-name?" +
		"Expires=1033570800&GoogleAccessId=xxx%40clientid&Signature=" +
		"c2lnbmVk" // base64('signed') == 'c2lnbmVk'
	if url != want {
		t.Fatalf("Unexpected signed URL\ngot:  %q\nwant: %q", url, want)
	}
}

func TestSignedURL_URLUnsafeObjectName(t *testing.T) {
	t.Parallel()
	expires, _ := time.Parse(time.RFC3339, "2002-10-02T10:00:00-05:00")
	url, err := SignedURL("bucket-name", "object name界", &SignedURLOptions{
		GoogleAccessID: "xxx@clientid",
		PrivateKey:     dummyKey("pem"),
		Method:         "GET",
		MD5:            "ICy5YqxZB1uWSwcVLSNLcA==",
		Expires:        expires,
		ContentType:    "application/json",
		Headers:        []string{"x-goog-header1:true", "x-goog-header2:false"},
	})
	if err != nil {
		t.Error(err)
	}
	want := "https://storage.googleapis.com/bucket-name/object%20name%E7%95%8C?" +
		"Expires=1033570800&GoogleAccessId=xxx%40clientid&Signature=bxVH1%2Bl%2" +
		"BSxpnj3XuqKz6mOFk6M94Y%2B4w85J6FCmJan%2FNhGSpndP6fAw1uLHlOn%2F8xUaY%2F" +
		"SfZ5GzcQ%2BbxOL1WA37yIwZ7xgLYlO%2ByAi3GuqMUmHZiNCai28emODXQ8RtWHvgv6dE" +
		"SQ%2F0KpDMIWW7rYCaUa63UkUyeSQsKhrVqkIA%3D"
	if url != want {
		t.Fatalf("Unexpected signed URL; found %v", url)
	}
}

func TestSignedURL_MissingOptions(t *testing.T) {
	t.Parallel()
	pk := dummyKey("rsa")
	expires, _ := time.Parse(time.RFC3339, "2002-10-02T10:00:00-05:00")
	var tests = []struct {
		opts   *SignedURLOptions
		errMsg string
	}{
		{
			&SignedURLOptions{},
			"missing required GoogleAccessID",
		},
		{
			&SignedURLOptions{GoogleAccessID: "access_id"},
			"exactly one of PrivateKey or SignedBytes must be set",
		},
		{
			&SignedURLOptions{
				GoogleAccessID: "access_id",
				SignBytes:      func(b []byte) ([]byte, error) { return b, nil },
				PrivateKey:     pk,
			},
			"exactly one of PrivateKey or SignedBytes must be set",
		},
		{
			&SignedURLOptions{
				GoogleAccessID: "access_id",
				PrivateKey:     pk,
			},
			"missing required method",
		},
		{
			&SignedURLOptions{
				GoogleAccessID: "access_id",
				SignBytes:      func(b []byte) ([]byte, error) { return b, nil },
			},
			"missing required method",
		},
		{
			&SignedURLOptions{
				GoogleAccessID: "access_id",
				PrivateKey:     pk,
				Method:         "PUT",
			},
			"missing required expires",
		},
		{
			&SignedURLOptions{
				GoogleAccessID: "access_id",
				PrivateKey:     pk,
				Method:         "PUT",
				Expires:        expires,
				MD5:            "invalid",
			},
			"invalid MD5 checksum",
		},
	}
	for _, test := range tests {
		_, err := SignedURL("bucket", "name", test.opts)
		if !strings.Contains(err.Error(), test.errMsg) {
			t.Errorf("expected err: %v, found: %v", test.errMsg, err)
		}
	}
}

func dummyKey(kind string) []byte {
	slurp, err := ioutil.ReadFile(fmt.Sprintf("./testdata/dummy_%s", kind))
	if err != nil {
		log.Fatal(err)
	}
	return slurp
}

func TestObjectNames(t *testing.T) {
	t.Parallel()
	// Naming requirements: https://cloud.google.com/storage/docs/bucket-naming
	const maxLegalLength = 1024

	type testT struct {
		name, want string
	}
	tests := []testT{
		// Embedded characters important in URLs.
		{"foo % bar", "foo%20%25%20bar"},
		{"foo ? bar", "foo%20%3F%20bar"},
		{"foo / bar", "foo%20/%20bar"},
		{"foo %?/ bar", "foo%20%25%3F/%20bar"},

		// Non-Roman scripts
		{"타코", "%ED%83%80%EC%BD%94"},
		{"世界", "%E4%B8%96%E7%95%8C"},

		// Longest legal name
		{strings.Repeat("a", maxLegalLength), strings.Repeat("a", maxLegalLength)},

		// Line terminators besides CR and LF: https://en.wikipedia.org/wiki/Newline#Unicode
		{"foo \u000b bar", "foo%20%0B%20bar"},
		{"foo \u000c bar", "foo%20%0C%20bar"},
		{"foo \u0085 bar", "foo%20%C2%85%20bar"},
		{"foo \u2028 bar", "foo%20%E2%80%A8%20bar"},
		{"foo \u2029 bar", "foo%20%E2%80%A9%20bar"},

		// Null byte.
		{"foo \u0000 bar", "foo%20%00%20bar"},

		// Non-control characters that are discouraged, but not forbidden, according to the documentation.
		{"foo # bar", "foo%20%23%20bar"},
		{"foo []*? bar", "foo%20%5B%5D%2A%3F%20bar"},

		// Angstrom symbol singleton and normalized forms: http://unicode.org/reports/tr15/
		{"foo \u212b bar", "foo%20%E2%84%AB%20bar"},
		{"foo \u0041\u030a bar", "foo%20A%CC%8A%20bar"},
		{"foo \u00c5 bar", "foo%20%C3%85%20bar"},

		// Hangul separating jamo: http://www.unicode.org/versions/Unicode7.0.0/ch18.pdf (Table 18-10)
		{"foo \u3131\u314f bar", "foo%20%E3%84%B1%E3%85%8F%20bar"},
		{"foo \u1100\u1161 bar", "foo%20%E1%84%80%E1%85%A1%20bar"},
		{"foo \uac00 bar", "foo%20%EA%B0%80%20bar"},
	}

	// C0 control characters not forbidden by the docs.
	var runes []rune
	for r := rune(0x01); r <= rune(0x1f); r++ {
		if r != '\u000a' && r != '\u000d' {
			runes = append(runes, r)
		}
	}
	tests = append(tests, testT{fmt.Sprintf("foo %s bar", string(runes)), "foo%20%01%02%03%04%05%06%07%08%09%0B%0C%0E%0F%10%11%12%13%14%15%16%17%18%19%1A%1B%1C%1D%1E%1F%20bar"})

	// C1 control characters, plus DEL.
	runes = nil
	for r := rune(0x7f); r <= rune(0x9f); r++ {
		runes = append(runes, r)
	}
	tests = append(tests, testT{fmt.Sprintf("foo %s bar", string(runes)), "foo%20%7F%C2%80%C2%81%C2%82%C2%83%C2%84%C2%85%C2%86%C2%87%C2%88%C2%89%C2%8A%C2%8B%C2%8C%C2%8D%C2%8E%C2%8F%C2%90%C2%91%C2%92%C2%93%C2%94%C2%95%C2%96%C2%97%C2%98%C2%99%C2%9A%C2%9B%C2%9C%C2%9D%C2%9E%C2%9F%20bar"})

	opts := &SignedURLOptions{
		GoogleAccessID: "xxx@clientid",
		PrivateKey:     dummyKey("rsa"),
		Method:         "GET",
		MD5:            "ICy5YqxZB1uWSwcVLSNLcA==",
		Expires:        time.Date(2002, time.October, 2, 10, 0, 0, 0, time.UTC),
		ContentType:    "application/json",
		Headers:        []string{"x-goog-header1", "x-goog-header2"},
	}

	for _, test := range tests {
		g, err := SignedURL("bucket-name", test.name, opts)
		if err != nil {
			t.Errorf("SignedURL(%q) err=%v, want nil", test.name, err)
		}
		if w := "/bucket-name/" + test.want; !strings.Contains(g, w) {
			t.Errorf("SignedURL(%q)=%q, want substring %q", test.name, g, w)
		}
	}
}

func TestCondition(t *testing.T) {
	t.Parallel()
	gotReq := make(chan *http.Request, 1)
	hc, close := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(ioutil.Discard, r.Body)
		gotReq <- r
		w.WriteHeader(200)
	})
	defer close()
	ctx := context.Background()
	c, err := NewClient(ctx, option.WithHTTPClient(hc))
	if err != nil {
		t.Fatal(err)
	}

	obj := c.Bucket("buck").Object("obj")
	dst := c.Bucket("dstbuck").Object("dst")
	tests := []struct {
		fn   func() error
		want string
	}{
		{
			func() error {
				_, err := obj.Generation(1234).NewReader(ctx)
				return err
			},
			"GET /buck/obj?generation=1234",
		},
		{
			func() error {
				_, err := obj.If(Conditions{GenerationMatch: 1234}).NewReader(ctx)
				return err
			},
			"GET /buck/obj?ifGenerationMatch=1234",
		},
		{
			func() error {
				_, err := obj.If(Conditions{GenerationNotMatch: 1234}).NewReader(ctx)
				return err
			},
			"GET /buck/obj?ifGenerationNotMatch=1234",
		},
		{
			func() error {
				_, err := obj.If(Conditions{MetagenerationMatch: 1234}).NewReader(ctx)
				return err
			},
			"GET /buck/obj?ifMetagenerationMatch=1234",
		},
		{
			func() error {
				_, err := obj.If(Conditions{MetagenerationNotMatch: 1234}).NewReader(ctx)
				return err
			},
			"GET /buck/obj?ifMetagenerationNotMatch=1234",
		},
		{
			func() error {
				_, err := obj.If(Conditions{MetagenerationNotMatch: 1234}).Attrs(ctx)
				return err
			},
			"GET /storage/v1/b/buck/o/obj?alt=json&ifMetagenerationNotMatch=1234&prettyPrint=false&projection=full",
		},
		{
			func() error {
				_, err := obj.If(Conditions{MetagenerationMatch: 1234}).Update(ctx, ObjectAttrsToUpdate{})
				return err
			},
			"PATCH /storage/v1/b/buck/o/obj?alt=json&ifMetagenerationMatch=1234&prettyPrint=false&projection=full",
		},
		{
			func() error { return obj.Generation(1234).Delete(ctx) },
			"DELETE /storage/v1/b/buck/o/obj?alt=json&generation=1234&prettyPrint=false",
		},
		{
			func() error {
				w := obj.If(Conditions{GenerationMatch: 1234}).NewWriter(ctx)
				w.ContentType = "text/plain"
				return w.Close()
			},
			"POST /upload/storage/v1/b/buck/o?alt=json&ifGenerationMatch=1234&prettyPrint=false&projection=full&uploadType=multipart",
		},
		{
			func() error {
				w := obj.If(Conditions{DoesNotExist: true}).NewWriter(ctx)
				w.ContentType = "text/plain"
				return w.Close()
			},
			"POST /upload/storage/v1/b/buck/o?alt=json&ifGenerationMatch=0&prettyPrint=false&projection=full&uploadType=multipart",
		},
		{
			func() error {
				_, err := dst.If(Conditions{MetagenerationMatch: 5678}).CopierFrom(obj.If(Conditions{GenerationMatch: 1234})).Run(ctx)
				return err
			},
			"POST /storage/v1/b/buck/o/obj/rewriteTo/b/dstbuck/o/dst?alt=json&ifMetagenerationMatch=5678&ifSourceGenerationMatch=1234&prettyPrint=false&projection=full",
		},
	}

	for i, tt := range tests {
		if err := tt.fn(); err != nil && err != io.EOF {
			t.Error(err)
			continue
		}
		select {
		case r := <-gotReq:
			got := r.Method + " " + r.RequestURI
			if got != tt.want {
				t.Errorf("%d. RequestURI = %q; want %q", i, got, tt.want)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("%d. timeout", i)
		}
		if err != nil {
			t.Fatal(err)
		}
	}

	// Test an error, too:
	err = obj.Generation(1234).NewWriter(ctx).Close()
	if err == nil || !strings.Contains(err.Error(), "NewWriter: generation not supported") {
		t.Errorf("want error about unsupported generation; got %v", err)
	}
}

func TestConditionErrors(t *testing.T) {
	t.Parallel()
	for _, conds := range []Conditions{
		{GenerationMatch: 0},
		{DoesNotExist: false}, // same as above, actually
		{GenerationMatch: 1, GenerationNotMatch: 2},
		{GenerationNotMatch: 2, DoesNotExist: true},
		{MetagenerationMatch: 1, MetagenerationNotMatch: 2},
	} {
		if err := conds.validate(""); err == nil {
			t.Errorf("%+v: got nil, want error", conds)
		}
	}
}

// Test object compose.
func TestObjectCompose(t *testing.T) {
	t.Parallel()
	gotURL := make(chan string, 1)
	gotBody := make(chan []byte, 1)
	hc, close := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		body, _ := ioutil.ReadAll(r.Body)
		gotURL <- r.URL.String()
		gotBody <- body
		w.Write([]byte("{}"))
	})
	defer close()
	ctx := context.Background()
	c, err := NewClient(ctx, option.WithHTTPClient(hc))
	if err != nil {
		t.Fatal(err)
	}

	testCases := []struct {
		desc    string
		dst     *ObjectHandle
		srcs    []*ObjectHandle
		attrs   *ObjectAttrs
		wantReq raw.ComposeRequest
		wantURL string
		wantErr bool
	}{
		{
			desc: "basic case",
			dst:  c.Bucket("foo").Object("bar"),
			srcs: []*ObjectHandle{
				c.Bucket("foo").Object("baz"),
				c.Bucket("foo").Object("quux"),
			},
			wantURL: "/storage/v1/b/foo/o/bar/compose?alt=json&prettyPrint=false",
			wantReq: raw.ComposeRequest{
				Destination: &raw.Object{Bucket: "foo"},
				SourceObjects: []*raw.ComposeRequestSourceObjects{
					{Name: "baz"},
					{Name: "quux"},
				},
			},
		},
		{
			desc: "with object attrs",
			dst:  c.Bucket("foo").Object("bar"),
			srcs: []*ObjectHandle{
				c.Bucket("foo").Object("baz"),
				c.Bucket("foo").Object("quux"),
			},
			attrs: &ObjectAttrs{
				Name:        "not-bar",
				ContentType: "application/json",
			},
			wantURL: "/storage/v1/b/foo/o/bar/compose?alt=json&prettyPrint=false",
			wantReq: raw.ComposeRequest{
				Destination: &raw.Object{
					Bucket:      "foo",
					Name:        "not-bar",
					ContentType: "application/json",
				},
				SourceObjects: []*raw.ComposeRequestSourceObjects{
					{Name: "baz"},
					{Name: "quux"},
				},
			},
		},
		{
			desc: "with conditions",
			dst: c.Bucket("foo").Object("bar").If(Conditions{
				GenerationMatch:     12,
				MetagenerationMatch: 34,
			}),
			srcs: []*ObjectHandle{
				c.Bucket("foo").Object("baz").Generation(56),
				c.Bucket("foo").Object("quux").If(Conditions{GenerationMatch: 78}),
			},
			wantURL: "/storage/v1/b/foo/o/bar/compose?alt=json&ifGenerationMatch=12&ifMetagenerationMatch=34&prettyPrint=false",
			wantReq: raw.ComposeRequest{
				Destination: &raw.Object{Bucket: "foo"},
				SourceObjects: []*raw.ComposeRequestSourceObjects{
					{
						Name:       "baz",
						Generation: 56,
					},
					{
						Name: "quux",
						ObjectPreconditions: &raw.ComposeRequestSourceObjectsObjectPreconditions{
							IfGenerationMatch: 78,
						},
					},
				},
			},
		},
		{
			desc:    "no sources",
			dst:     c.Bucket("foo").Object("bar"),
			wantErr: true,
		},
		{
			desc: "destination, no bucket",
			dst:  c.Bucket("").Object("bar"),
			srcs: []*ObjectHandle{
				c.Bucket("foo").Object("baz"),
			},
			wantErr: true,
		},
		{
			desc: "destination, no object",
			dst:  c.Bucket("foo").Object(""),
			srcs: []*ObjectHandle{
				c.Bucket("foo").Object("baz"),
			},
			wantErr: true,
		},
		{
			desc: "source, different bucket",
			dst:  c.Bucket("foo").Object("bar"),
			srcs: []*ObjectHandle{
				c.Bucket("otherbucket").Object("baz"),
			},
			wantErr: true,
		},
		{
			desc: "source, no object",
			dst:  c.Bucket("foo").Object("bar"),
			srcs: []*ObjectHandle{
				c.Bucket("foo").Object(""),
			},
			wantErr: true,
		},
		{
			desc: "destination, bad condition",
			dst:  c.Bucket("foo").Object("bar").Generation(12),
			srcs: []*ObjectHandle{
				c.Bucket("foo").Object("baz"),
			},
			wantErr: true,
		},
		{
			desc: "source, bad condition",
			dst:  c.Bucket("foo").Object("bar"),
			srcs: []*ObjectHandle{
				c.Bucket("foo").Object("baz").If(Conditions{MetagenerationMatch: 12}),
			},
			wantErr: true,
		},
	}

	for _, tt := range testCases {
		composer := tt.dst.ComposerFrom(tt.srcs...)
		if tt.attrs != nil {
			composer.ObjectAttrs = *tt.attrs
		}
		_, err := composer.Run(ctx)
		if gotErr := err != nil; gotErr != tt.wantErr {
			t.Errorf("%s: got error %v; want err %t", tt.desc, err, tt.wantErr)
			continue
		}
		if tt.wantErr {
			continue
		}
		url, body := <-gotURL, <-gotBody
		if url != tt.wantURL {
			t.Errorf("%s: request URL\ngot  %q\nwant %q", tt.desc, url, tt.wantURL)
		}
		var req raw.ComposeRequest
		if err := json.Unmarshal(body, &req); err != nil {
			t.Errorf("%s: json.Unmarshal %v (body %s)", tt.desc, err, body)
		}
		if !testutil.Equal(req, tt.wantReq) {
			// Print to JSON.
			wantReq, _ := json.Marshal(tt.wantReq)
			t.Errorf("%s: request body\ngot  %s\nwant %s", tt.desc, body, wantReq)
		}
	}
}

// Test that ObjectIterator's Next and NextPage methods correctly terminate
// if there is nothing to iterate over.
func TestEmptyObjectIterator(t *testing.T) {
	t.Parallel()
	hClient, close := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(ioutil.Discard, r.Body)
		fmt.Fprintf(w, "{}")
	})
	defer close()
	ctx := context.Background()
	client, err := NewClient(ctx, option.WithHTTPClient(hClient))
	if err != nil {
		t.Fatal(err)
	}
	it := client.Bucket("b").Objects(ctx, nil)
	_, err = it.Next()
	if err != iterator.Done {
		t.Errorf("got %v, want Done", err)
	}
}

// Test that BucketIterator's Next method correctly terminates if there is
// nothing to iterate over.
func TestEmptyBucketIterator(t *testing.T) {
	t.Parallel()
	hClient, close := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(ioutil.Discard, r.Body)
		fmt.Fprintf(w, "{}")
	})
	defer close()
	ctx := context.Background()
	client, err := NewClient(ctx, option.WithHTTPClient(hClient))
	if err != nil {
		t.Fatal(err)
	}
	it := client.Buckets(ctx, "project")
	_, err = it.Next()
	if err != iterator.Done {
		t.Errorf("got %v, want Done", err)
	}

}

func TestCodecUint32(t *testing.T) {
	t.Parallel()
	for _, u := range []uint32{0, 1, 256, 0xFFFFFFFF} {
		s := encodeUint32(u)
		d, err := decodeUint32(s)
		if err != nil {
			t.Fatal(err)
		}
		if d != u {
			t.Errorf("got %d, want input %d", d, u)
		}
	}
}

func TestUserProject(t *testing.T) {
	// Verify that the userProject query param is sent.
	t.Parallel()
	ctx := context.Background()
	gotURL := make(chan *url.URL, 1)
	hClient, close := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(ioutil.Discard, r.Body)
		gotURL <- r.URL
		if strings.Contains(r.URL.String(), "/rewriteTo/") {
			res := &raw.RewriteResponse{Done: true}
			bytes, err := res.MarshalJSON()
			if err != nil {
				t.Fatal(err)
			}
			w.Write(bytes)
		} else {
			fmt.Fprintf(w, "{}")
		}
	})
	defer close()
	client, err := NewClient(ctx, option.WithHTTPClient(hClient))
	if err != nil {
		t.Fatal(err)
	}

	re := regexp.MustCompile(`\buserProject=p\b`)
	b := client.Bucket("b").UserProject("p")
	o := b.Object("o")

	check := func(msg string, f func()) {
		f()
		select {
		case u := <-gotURL:
			if !re.MatchString(u.RawQuery) {
				t.Errorf("%s: query string %q does not contain userProject", msg, u.RawQuery)
			}
		case <-time.After(2 * time.Second):
			t.Errorf("%s: timed out", msg)
		}
	}

	check("buckets.delete", func() { b.Delete(ctx) })
	check("buckets.get", func() { b.Attrs(ctx) })
	check("buckets.patch", func() { b.Update(ctx, BucketAttrsToUpdate{}) })
	check("storage.objects.compose", func() { o.ComposerFrom(b.Object("x")).Run(ctx) })
	check("storage.objects.delete", func() { o.Delete(ctx) })
	check("storage.objects.get", func() { o.Attrs(ctx) })
	check("storage.objects.insert", func() { o.NewWriter(ctx).Close() })
	check("storage.objects.list", func() { b.Objects(ctx, nil).Next() })
	check("storage.objects.patch", func() { o.Update(ctx, ObjectAttrsToUpdate{}) })
	check("storage.objects.rewrite", func() { o.CopierFrom(b.Object("x")).Run(ctx) })
	check("storage.objectAccessControls.list", func() { o.ACL().List(ctx) })
	check("storage.objectAccessControls.update", func() { o.ACL().Set(ctx, "", "") })
	check("storage.objectAccessControls.delete", func() { o.ACL().Delete(ctx, "") })
	check("storage.bucketAccessControls.list", func() { b.ACL().List(ctx) })
	check("storage.bucketAccessControls.update", func() { b.ACL().Set(ctx, "", "") })
	check("storage.bucketAccessControls.delete", func() { b.ACL().Delete(ctx, "") })
	check("storage.defaultObjectAccessControls.list",
		func() { b.DefaultObjectACL().List(ctx) })
	check("storage.defaultObjectAccessControls.update",
		func() { b.DefaultObjectACL().Set(ctx, "", "") })
	check("storage.defaultObjectAccessControls.delete",
		func() { b.DefaultObjectACL().Delete(ctx, "") })
	check("buckets.getIamPolicy", func() { b.IAM().Policy(ctx) })
	check("buckets.setIamPolicy", func() {
		p := &iam.Policy{}
		p.Add("m", iam.Owner)
		b.IAM().SetPolicy(ctx, p)
	})
	check("buckets.testIamPermissions", func() { b.IAM().TestPermissions(ctx, nil) })
	check("storage.notifications.insert", func() {
		b.AddNotification(ctx, &Notification{TopicProjectID: "p", TopicID: "t"})
	})
	check("storage.notifications.delete", func() { b.DeleteNotification(ctx, "n") })
	check("storage.notifications.list", func() { b.Notifications(ctx) })
}

func newTestServer(handler func(w http.ResponseWriter, r *http.Request)) (*http.Client, func()) {
	ts := httptest.NewTLSServer(http.HandlerFunc(handler))
	tlsConf := &tls.Config{InsecureSkipVerify: true}
	tr := &http.Transport{
		TLSClientConfig: tlsConf,
		DialTLS: func(netw, addr string) (net.Conn, error) {
			return tls.Dial("tcp", ts.Listener.Addr().String(), tlsConf)
		},
	}
	return &http.Client{Transport: tr}, func() {
		tr.CloseIdleConnections()
		ts.Close()
	}
}

func TestRawObjectToObjectAttrs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   *raw.Object
		want *ObjectAttrs
	}{
		{in: nil, want: nil},
		{
			in: &raw.Object{
				Bucket:                  "Test",
				ContentLanguage:         "en-us",
				ContentType:             "video/mpeg",
				EventBasedHold:          false,
				Etag:                    "Zkyw9ACJZUvcYmlFaKGChzhmtnE/dt1zHSfweiWpwzdGsqXwuJZqiD0",
				Generation:              7,
				Md5Hash:                 "MTQ2ODNjYmE0NDRkYmNjNmRiMjk3NjQ1ZTY4M2Y1YzE=",
				Name:                    "foo.mp4",
				RetentionExpirationTime: "2019-03-31T19:33:36Z",
				Size:                    1 << 20,
				TimeCreated:             "2019-03-31T19:32:10Z",
				TimeDeleted:             "2019-03-31T19:33:39Z",
				TemporaryHold:           true,
			},
			want: &ObjectAttrs{
				Bucket:                  "Test",
				Created:                 time.Date(2019, 3, 31, 19, 32, 10, 0, time.UTC),
				ContentLanguage:         "en-us",
				ContentType:             "video/mpeg",
				Deleted:                 time.Date(2019, 3, 31, 19, 33, 39, 0, time.UTC),
				EventBasedHold:          false,
				Etag:                    "Zkyw9ACJZUvcYmlFaKGChzhmtnE/dt1zHSfweiWpwzdGsqXwuJZqiD0",
				Generation:              7,
				MD5:                     []byte("14683cba444dbcc6db297645e683f5c1"),
				Name:                    "foo.mp4",
				RetentionExpirationTime: time.Date(2019, 3, 31, 19, 33, 36, 0, time.UTC),
				Size:                    1 << 20,
				TemporaryHold:           true,
			},
		},
	}

	for i, tt := range tests {
		got := newObject(tt.in)
		if diff := testutil.Diff(got, tt.want); diff != "" {
			t.Errorf("#%d: newObject mismatches:\ngot=-, want=+:\n%s", i, diff)
		}
	}
}

func TestObjectAttrsToRawObject(t *testing.T) {
	t.Parallel()
	bucketName := "the-bucket"
	tests := []struct {
		in   *ObjectAttrs
		want *raw.Object
	}{
		{

			in: &ObjectAttrs{
				Bucket:                  "Test",
				Created:                 time.Date(2019, 3, 31, 19, 32, 10, 0, time.UTC),
				ContentLanguage:         "en-us",
				ContentType:             "video/mpeg",
				Deleted:                 time.Date(2019, 3, 31, 19, 33, 39, 0, time.UTC),
				EventBasedHold:          false,
				Etag:                    "Zkyw9ACJZUvcYmlFaKGChzhmtnE/dt1zHSfweiWpwzdGsqXwuJZqiD0",
				Generation:              7,
				MD5:                     []byte("14683cba444dbcc6db297645e683f5c1"),
				Name:                    "foo.mp4",
				RetentionExpirationTime: time.Date(2019, 3, 31, 19, 33, 36, 0, time.UTC),
				Size:                    1 << 20,
				TemporaryHold:           true,
			},
			want: &raw.Object{
				Bucket:                  bucketName,
				ContentLanguage:         "en-us",
				ContentType:             "video/mpeg",
				EventBasedHold:          false,
				Name:                    "foo.mp4",
				RetentionExpirationTime: "2019-03-31T19:33:36Z",
				TemporaryHold:           true,
			},
		},
	}

	for i, tt := range tests {
		got := tt.in.toRawObject(bucketName)
		if !testutil.Equal(got, tt.want) {
			if diff := testutil.Diff(got, tt.want); diff != "" {
				t.Errorf("#%d: toRawObject mismatches:\ngot=-, want=+:\n%s", i, diff)
			}
		}
	}
}
