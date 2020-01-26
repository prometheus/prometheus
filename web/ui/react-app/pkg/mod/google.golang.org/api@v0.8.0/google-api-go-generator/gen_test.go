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

package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/api/internal/version"
)

var updateGolden = flag.Bool("update_golden", false, "If true, causes TestAPIs to update golden files")

func TestAPIs(t *testing.T) {
	*copyrightYear = "YEAR"

	names := []string{
		"any",
		"arrayofarray-1",
		"arrayofenum",
		"arrayofmapofobjects",
		"arrayofmapofstrings",
		"blogger-3",
		"floats",
		"getwithoutbody",
		"http-body",
		"json-body",
		"mapofany",
		"mapofarrayofobjects",
		"mapofint64strings",
		"mapofobjects",
		"mapofstrings-1",
		"param-rename",
		"quotednum",
		"repeated",
		"required-query",
		"resource-named-service", // appengine/v1/appengine-api.json
		"unfortunatedefaults",
		"variants",
		"wrapnewlines",
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			api, err := apiFromFile(filepath.Join("testdata", name+".json"))
			if err != nil {
				t.Fatalf("Error loading API testdata/%s.json: %v", name, err)
			}
			clean, err := api.GenerateCode()
			if err != nil {
				t.Fatalf("Error generating code for %s: %v", name, err)
			}
			goldenFile := filepath.Join("testdata", name+".want")
			if *updateGolden {
				if err := ioutil.WriteFile(goldenFile, clean, 0644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := ioutil.ReadFile(goldenFile)
			if err != nil {
				t.Fatal(err)
			}
			wantStr := strings.Replace(string(want), "gl-go/1.12.5", fmt.Sprintf("gl-go/%s", version.Go()), -1)
			wantStr = strings.Replace(wantStr, "gdcl/00000000", fmt.Sprintf("gdcl/%s", version.Repo), -1)
			want = []byte(wantStr)
			if !bytes.Equal(want, clean) {
				tf, _ := ioutil.TempFile("", "api-"+name+"-got-json.")
				if _, err := tf.Write(clean); err != nil {
					t.Fatal(err)
				}
				if err := tf.Close(); err != nil {
					t.Fatal(err)
				}
				t.Errorf("Output for API %s differs: diff -u %s %s", name, goldenFile, tf.Name())
			}
		})
	}
}

func TestScope(t *testing.T) {
	tests := [][]string{
		{
			"https://www.googleapis.com/auth/somescope",
			"SomescopeScope",
		},
		{
			"https://mail.google.com/somescope",
			"MailGoogleComSomescopeScope",
		},
		{
			"https://mail.google.com/",
			"MailGoogleComScope",
		},
	}
	for _, test := range tests {
		if got := scopeIdentifierFromURL(test[0]); got != test[1] {
			t.Errorf("scopeIdentifierFromURL(%q) got %q, want %q", test[0], got, test[1])
		}
	}
}

func TestDepunct(t *testing.T) {
	tests := []struct {
		needCap  bool
		in, want string
	}{
		{
			needCap: true,
			in:      "part__description",
			want:    "Part__description",
		},
		{
			needCap: true,
			in:      "Part_description",
			want:    "PartDescription",
		},
		{
			needCap: false,
			in:      "part_description",
			want:    "partDescription",
		},
		{
			needCap: false,
			in:      "part-description",
			want:    "partDescription",
		},
		{
			needCap: false,
			in:      "part.description",
			want:    "partDescription",
		},
		{
			needCap: false,
			in:      "part$description",
			want:    "partDescription",
		},
		{
			needCap: false,
			in:      "part/description",
			want:    "partDescription",
		},
		{
			needCap: true,
			in:      "Part__description_name",
			want:    "Part__descriptionName",
		},
		{
			needCap: true,
			in:      "Part_description_name",
			want:    "PartDescriptionName",
		},
		{
			needCap: true,
			in:      "Part__description__name",
			want:    "Part__description__name",
		},
		{
			needCap: true,
			in:      "Part_description__name",
			want:    "PartDescription__name",
		},
	}
	for _, test := range tests {
		if got := depunct(test.in, test.needCap); got != test.want {
			t.Errorf("depunct(%q,%v) = %q; want %q", test.in, test.needCap, got, test.want)
		}
	}
}

func TestRenameVersion(t *testing.T) {
	tests := []struct {
		version, want string
	}{
		{
			version: "directory_v1",
			want:    "directory/v1",
		},
		{
			version: "email_migration_v1",
			want:    "email_migration/v1",
		},
		{
			version: "my_api_v1.2",
			want:    "my_api/v1.2",
		},
	}
	for _, test := range tests {
		if got := renameVersion(test.version); got != test.want {
			t.Errorf("renameVersion(%q) = %q; want %q", test.version, got, test.want)
		}
	}
}

func TestSupportsPaging(t *testing.T) {
	api, err := apiFromFile(filepath.Join("testdata", "paging.json"))
	if err != nil {
		t.Fatalf("Error loading API testdata/paging.json: %v", err)
	}
	api.PopulateSchemas()
	res := api.doc.Resources[0]
	for _, meth := range api.resourceMethods(res) {
		_, _, got := meth.supportsPaging()
		want := strings.HasPrefix(meth.m.Name, "yes")
		if got != want {
			t.Errorf("method %s supports paging: got %t, want %t", meth.m.Name, got, want)
		}
	}
}
