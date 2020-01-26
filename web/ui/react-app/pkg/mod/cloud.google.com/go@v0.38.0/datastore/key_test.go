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
	"bytes"
	"encoding/gob"
	"encoding/json"
	"testing"
)

func TestEqual(t *testing.T) {
	testCases := []struct {
		x, y  *Key
		equal bool
	}{
		{
			x:     nil,
			y:     nil,
			equal: true,
		},
		{
			x:     &Key{Kind: "kindA"},
			y:     &Key{Kind: "kindA"},
			equal: true,
		},
		{
			x:     &Key{Kind: "kindA", Name: "nameA"},
			y:     &Key{Kind: "kindA", Name: "nameA"},
			equal: true,
		},
		{
			x:     &Key{Kind: "kindA", Name: "nameA", Namespace: "gopherspace"},
			y:     &Key{Kind: "kindA", Name: "nameA", Namespace: "gopherspace"},
			equal: true,
		},
		{
			x:     &Key{Kind: "kindA", ID: 1337, Parent: &Key{Kind: "kindX", Name: "nameX"}},
			y:     &Key{Kind: "kindA", ID: 1337, Parent: &Key{Kind: "kindX", Name: "nameX"}},
			equal: true,
		},
		{
			x:     &Key{Kind: "kindA", Name: "nameA"},
			y:     &Key{Kind: "kindB", Name: "nameA"},
			equal: false,
		},
		{
			x:     &Key{Kind: "kindA", Name: "nameA"},
			y:     &Key{Kind: "kindA", Name: "nameB"},
			equal: false,
		},
		{
			x:     &Key{Kind: "kindA", Name: "nameA"},
			y:     &Key{Kind: "kindA", ID: 1337},
			equal: false,
		},
		{
			x:     &Key{Kind: "kindA", Name: "nameA"},
			y:     &Key{Kind: "kindA", Name: "nameA", Namespace: "gopherspace"},
			equal: false,
		},
		{
			x:     &Key{Kind: "kindA", ID: 1337, Parent: &Key{Kind: "kindX", Name: "nameX"}},
			y:     &Key{Kind: "kindA", ID: 1337, Parent: &Key{Kind: "kindY", Name: "nameX"}},
			equal: false,
		},
		{
			x:     &Key{Kind: "kindA", ID: 1337, Parent: &Key{Kind: "kindX", Name: "nameX"}},
			y:     &Key{Kind: "kindA", ID: 1337},
			equal: false,
		},
	}

	for _, tt := range testCases {
		if got := tt.x.Equal(tt.y); got != tt.equal {
			t.Errorf("Equal(%v, %v) = %t; want %t", tt.x, tt.y, got, tt.equal)
		}
		if got := tt.y.Equal(tt.x); got != tt.equal {
			t.Errorf("Equal(%v, %v) = %t; want %t", tt.y, tt.x, got, tt.equal)
		}
	}
}

func TestEncoding(t *testing.T) {
	testCases := []struct {
		k     *Key
		valid bool
	}{
		{
			k:     nil,
			valid: false,
		},
		{
			k:     &Key{},
			valid: false,
		},
		{
			k:     &Key{Kind: "kindA"},
			valid: true,
		},
		{
			k:     &Key{Kind: "kindA", Namespace: "gopherspace"},
			valid: true,
		},
		{
			k:     &Key{Kind: "kindA", Name: "nameA"},
			valid: true,
		},
		{
			k:     &Key{Kind: "kindA", ID: 1337},
			valid: true,
		},
		{
			k:     &Key{Kind: "kindA", Name: "nameA", ID: 1337},
			valid: false,
		},
		{
			k:     &Key{Kind: "kindA", Parent: &Key{Kind: "kindB", Name: "nameB"}},
			valid: true,
		},
		{
			k:     &Key{Kind: "kindA", Parent: &Key{Kind: "kindB"}},
			valid: false,
		},
		{
			k:     &Key{Kind: "kindA", Parent: &Key{Kind: "kindB", Name: "nameB", Namespace: "gopherspace"}},
			valid: false,
		},
	}

	for _, tt := range testCases {
		if got := tt.k.valid(); got != tt.valid {
			t.Errorf("valid(%v) = %t; want %t", tt.k, got, tt.valid)
		}

		// Check encoding/decoding for valid keys.
		if !tt.valid {
			continue
		}
		enc := tt.k.Encode()
		dec, err := DecodeKey(enc)
		if err != nil {
			t.Errorf("DecodeKey(%q) from %v: %v", enc, tt.k, err)
			continue
		}
		if !tt.k.Equal(dec) {
			t.Logf("Proto: %s", keyToProto(tt.k))
			t.Errorf("Decoded key %v not equal to %v", dec, tt.k)
		}

		b, err := json.Marshal(tt.k)
		if err != nil {
			t.Errorf("json.Marshal(%v): %v", tt.k, err)
			continue
		}
		key := &Key{}
		if err := json.Unmarshal(b, key); err != nil {
			t.Errorf("json.Unmarshal(%s) for key %v: %v", b, tt.k, err)
			continue
		}
		if !tt.k.Equal(key) {
			t.Errorf("JSON decoded key %v not equal to %v", dec, tt.k)
		}

		buf := &bytes.Buffer{}
		gobEnc := gob.NewEncoder(buf)
		if err := gobEnc.Encode(tt.k); err != nil {
			t.Errorf("gobEnc.Encode(%v): %v", tt.k, err)
			continue
		}
		gobDec := gob.NewDecoder(buf)
		key = &Key{}
		if err := gobDec.Decode(key); err != nil {
			t.Errorf("gobDec.Decode() for key %v: %v", tt.k, err)
		}
		if !tt.k.Equal(key) {
			t.Errorf("gob decoded key %v not equal to %v", dec, tt.k)
		}
	}
}

func TestInvalidKeyDecode(t *testing.T) {
	// Check that decoding an invalid key returns an err and doesn't panic.
	enc := NameKey("Kind", "Foo", nil).Encode()

	invalid := []string{
		"",
		"Laboratorio",
		enc + "Junk",
		enc[:len(enc)-4],
	}
	for _, enc := range invalid {
		key, err := DecodeKey(enc)
		if err == nil || key != nil {
			t.Errorf("DecodeKey(%q) = %v, %v; want nil, error", enc, key, err)
		}
	}
}
