// Copyright 2018 The Prometheus Authors
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

//go:build ignore
// +build ignore

package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
)

var (
	OUTPUT_FILE = "assets.go"

	PREFIX = "//go:embed "
)

var HEADER = []byte(`package ui

import (
    "io"
    "io/fs"
    "time"
)

type FSEntry struct {
    n string
    v []byte
    offset int
}

func (f *FSEntry) Name() string {
    return f.n
}
func (f *FSEntry) Size() int64 {
    return int64(len(f.v))
}
func (f *FSEntry) Mode() fs.FileMode { 
    return fs.ModeDevice
}
func (f *FSEntry) ModTime() time.Time {
    return time.Time{}
}
func (f *FSEntry) IsDir() bool { return false }
func (f *FSEntry) Sys() interface{}    { return nil }    // TODO: update to any from interface{}


func (f *FSEntry) Stat() (fs.FileInfo, error) {
    return f, nil
}
func (f *FSEntry) Read(buf []byte) (int, error) {
    if len(buf) > len(f.v) -f.offset{
        buf = buf[0:len(f.v[f.offset:])]
    }
    n := copy(buf, f.v[f.offset:])
    if n == len(f.v)-f.offset {
        return n, io.EOF
    }
    f.offset += n
    return n, nil
}
func (f *FSEntry) Close() error { return nil }

type FSMap map[string][]byte

func (f FSMap) Open(name string) (fs.File, error) {
    v, ok := f[name]
    if !ok {
        return nil, fs.ErrNotExist
    }
    
    return &FSEntry{n: name, v:v}, nil
}

var fsMap = FSMap{

`)

var TRAILER = []byte(`}`)

func check(e error) {
	if e != nil {
		log.Fatal(e)
	}
}

func main() {
	// Find the files to load from embed.go
	b, err := ioutil.ReadFile("embed.go")
	check(err)

	bLines := bytes.Split(b, []byte("\n"))

	var filesToLoad []string
	// parse the lines looking for the embed header -- to find our list
	for _, l := range bLines {
		if bytes.HasPrefix(l, []byte(PREFIX)) {
			fmt.Println(string(l))

			s := string(bytes.TrimPrefix(l, []byte(PREFIX)))
			filesToLoad = strings.Split(s, " ")
			break
		}
	}

	f, err := os.Create(OUTPUT_FILE)
	check(err)
	defer f.Close()

	_, err = f.Write(HEADER)
	check(err)

	fmt.Println("files to load", filesToLoad)

	for _, fn := range filesToLoad {
		fmt.Println(fn) // TODO: remove
		// Read file contents
		b, err := ioutil.ReadFile(fn)
		check(err)

		_, err = fmt.Fprintf(f, "\t\"%s\": []byte(%q),\n", fn, b)
		check(err)
	}

	_, err = f.Write(TRAILER)
	check(err)

}
