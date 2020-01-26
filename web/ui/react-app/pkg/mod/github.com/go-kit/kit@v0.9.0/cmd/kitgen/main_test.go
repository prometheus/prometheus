package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/aryann/difflib"
)

var update = flag.Bool("update", false, "update golden files")

func TestProcess(t *testing.T) {
	cases, err := filepath.Glob("testdata/*")
	if err != nil {
		t.Fatal(err)
	}

	laidout := func(t *testing.T, inpath, dir, kind string, layout layout, in []byte) {
		t.Run(kind, func(t *testing.T) {
			targetDir := filepath.Join(dir, kind)
			tree, err := process(inpath, bytes.NewBuffer(in), layout)
			if err != nil {
				t.Fatal(inpath, fmt.Sprintf("%+#v", err))
			}

			if *update {
				err := splat(targetDir, tree)
				if err != nil {
					t.Fatal(kind, err)
				}
				// otherwise we need to do some tomfoolery with resetting buffers
				// I'm willing to just run the tests again - besides, we shouldn't be
				// regerating the golden files that often
				t.Error("Updated outputs - DID NOT COMPARE! (run tests again without -update)")
				return
			}

			for filename, buf := range tree {
				actual, err := ioutil.ReadAll(buf)
				if err != nil {
					t.Fatal(kind, filename, err)
				}

				outpath := filepath.Join(targetDir, filename)

				expected, err := ioutil.ReadFile(outpath)
				if err != nil {
					t.Fatal(outpath, err)
				}

				if !bytes.Equal(expected, actual) {
					results := difflib.Diff(splitLines(expected), splitLines(actual))
					for _, result := range results {
						if result.Delta == difflib.Common {
							continue
						}
						t.Error(result)
					}
				}
			}

			if !t.Failed() {
				build := exec.Command("go", "build", "./...")
				build.Dir = targetDir
				out, err := build.CombinedOutput()
				if err != nil {
					t.Fatalf("Cannot build output: %v\n%s", err, string(out))
				}
			}
		})

	}

	testcase := func(dir string) {
		name := filepath.Base(dir)
		t.Run(name, func(t *testing.T) {
			inpath := filepath.Join(dir, "in.go")

			in, err := ioutil.ReadFile(inpath)
			if err != nil {
				t.Fatal(inpath, err)
			}
			laidout(t, inpath, dir, "flat", flat{}, in)
			laidout(t, inpath, dir, "default", deflayout{
				targetDir: filepath.Join("github.com/go-kit/kit/cmd/kitgen", dir, "default"),
			}, in)
		})
	}

	for _, dir := range cases {
		testcase(dir)
	}
}

func TestTemplatesBuild(t *testing.T) {
	build := exec.Command("go", "build", "./...")
	build.Dir = "templates"
	out, err := build.CombinedOutput()
	if err != nil {
		t.Fatal(err, "\n", string(out))
	}
}

func splitLines(txt []byte) []string {
	s := bufio.NewScanner(bytes.NewReader(txt))
	var ss []string
	for s.Scan() {
		ss = append(ss, s.Text())
	}
	return ss
}
