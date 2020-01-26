// +build go1.9

// Copyright 2018 Microsoft Corporation and contributors.  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//
// See the License for the specific language governing permissions and
// limitations under the License.

package model_test

import (
	"bytes"
	"io"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Azure/azure-sdk-for-go/tools/profileBuilder/model"
)

func TestList_Enumerate(t *testing.T) {
	smallProfile, err := os.Open(filepath.Join("testdata", "smallProfile.txt"))
	if err != nil {
		t.Error(err)
		return
	}

	gopath := strings.Replace(os.Getenv("GOPATH"), "\\", "/", -1)

	testCases := []struct {
		io.Reader
		expected map[string]struct{}
	}{
		{
			bytes.NewReader([]byte("a.md")),
			map[string]struct{}{path.Join(gopath, "src", "a.md"): struct{}{}},
		},
		{
			smallProfile,
			map[string]struct{}{
				path.Join(gopath, "src", "github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2015-06-15/compute"): struct{}{},
				path.Join(gopath, "src", "github.com/Azure/azure-sdk-for-go/services/network/mgmt/2015-06-15/network"): struct{}{},
			},
		},
	}

	for _, tc := range testCases {
		subject := model.ListStrategy{Reader: tc}
		t.Run("", func(t *testing.T) {
			done := make(chan struct{})
			defer close(done)

			for currentLine := range subject.Enumerate(done) {
				cast, ok := currentLine.(string)
				if !ok {
					t.Logf("ListStrategy returned a %q instead of a \"string\"", reflect.TypeOf(currentLine).Name())
					t.Fail()
					return
				}

				cast = strings.Replace(cast, "\\", "/", -1)

				if _, ok = tc.expected[cast]; !ok {
					t.Logf("Unexpected value %q encountered", cast)
					t.Fail()
				} else {
					delete(tc.expected, cast)
				}
			}

			for unseen := range tc.expected {
				t.Logf("Expected value %q was unencountered", unseen)
				t.Fail()
			}
		})
	}
}
