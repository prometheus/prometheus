// Copyright 2017, 2018 Percona LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build gofuzzgen
// +build gofuzzgen

package gofuzz

import (
	"crypto/sha1"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
)

var root string

func init() {
	_, file, _, _ := runtime.Caller(0)
	root = filepath.Join(filepath.Dir(file), "..", "..", "go-fuzz")
}

// AddToCorpus adds data to named go-fuzz corpus when "gofuzzgen" build tag is used.
func AddToCorpus(name string, data []byte) {
	path := filepath.Join(root, name, "corpus")
	_ = os.MkdirAll(path, 0700)

	path = filepath.Join(path, fmt.Sprintf("test-%x", sha1.Sum(data)))
	if err := ioutil.WriteFile(path, data, 0666); err != nil {
		panic(err)
	}
}
