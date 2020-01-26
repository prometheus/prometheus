// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pipeline

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"golang.org/x/text/language"
)

var genFiles = flag.Bool("gen", false, "generate output files instead of comparing")

// setHelper is testing.T.Helper on Go 1.9+, overridden by go19_test.go.
var setHelper = func(t *testing.T) {}

func TestFullCycle(t *testing.T) {
	if runtime.GOOS == "android" {
		t.Skip("cannot load outside packages on android")
	}
	const path = "./testdata"
	dirs, err := ioutil.ReadDir(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range dirs {
		t.Run(f.Name(), func(t *testing.T) {
			chk := func(t *testing.T, err error) {
				setHelper(t)
				if err != nil {
					t.Fatal(err)
				}
			}
			dir := filepath.Join(path, f.Name())
			pkgPath := fmt.Sprintf("%s/%s", path, f.Name())
			config := Config{
				SourceLanguage: language.AmericanEnglish,
				Packages:       []string{pkgPath},
				Dir:            filepath.Join(dir, "locales"),
				GenFile:        "catalog_gen.go",
				GenPackage:     pkgPath,
			}
			// TODO: load config if available.
			s, err := Extract(&config)
			chk(t, err)
			chk(t, s.Import())
			chk(t, s.Merge())
			// TODO:
			//  for range s.Config.Actions {
			//  	//  TODO: do the actions.
			//  }
			chk(t, s.Export())
			chk(t, s.Generate())

			writeJSON(t, filepath.Join(dir, "extracted.gotext.json"), s.Extracted)
			checkOutput(t, dir)
		})
	}
}

func checkOutput(t *testing.T, p string) {
	filepath.Walk(p, func(p string, f os.FileInfo, err error) error {
		if f.IsDir() {
			return nil
		}
		if filepath.Ext(p) != ".want" {
			return nil
		}
		gotFile := p[:len(p)-len(".want")]
		got, err := ioutil.ReadFile(gotFile)
		if err != nil {
			t.Errorf("failed to read %q", p)
			return nil
		}
		if *genFiles {
			if err := ioutil.WriteFile(p, got, 0644); err != nil {
				t.Fatal(err)
			}
		}
		want, err := ioutil.ReadFile(p)
		if err != nil {
			t.Errorf("failed to read %q", p)
		} else {
			scanGot := bufio.NewScanner(bytes.NewReader(got))
			scanWant := bufio.NewScanner(bytes.NewReader(want))
			line := 0
			clean := func(s string) string {
				if i := strings.LastIndex(s, "//"); i != -1 {
					s = s[:i]
				}
				return path.Clean(filepath.ToSlash(s))
			}
			for scanGot.Scan() && scanWant.Scan() {
				got := clean(scanGot.Text())
				want := clean(scanWant.Text())
				if got != want {
					t.Errorf("file %q differs from .want file at line %d:\n\t%s\n\t%s", gotFile, line, got, want)
					break
				}
				line++
			}
			if scanGot.Scan() || scanWant.Scan() {
				t.Errorf("file %q differs from .want file at line %d.", gotFile, line)
			}
		}
		return nil
	})
}

func writeJSON(t *testing.T, path string, x interface{}) {
	data, err := json.MarshalIndent(x, "", "    ")
	if err != nil {
		t.Fatal(err)
	}
	if err := ioutil.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
}
