/*
Copyright (c) 2014 VMware, Inc. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package debug

import (
	"io"
	"os"
	"path"
)

// Provider specified the interface types must implement to be used as a
// debugging sink. Having multiple such sink implementations allows it to be
// changed externally (for example when running tests).
type Provider interface {
	NewFile(s string) io.WriteCloser
	Flush()
}

var currentProvider Provider = nil

func SetProvider(p Provider) {
	if currentProvider != nil {
		currentProvider.Flush()
	}
	currentProvider = p
}

// Enabled returns whether debugging is enabled or not.
func Enabled() bool {
	return currentProvider != nil
}

// NewFile dispatches to the current provider's NewFile function.
func NewFile(s string) io.WriteCloser {
	return currentProvider.NewFile(s)
}

// Flush dispatches to the current provider's Flush function.
func Flush() {
	currentProvider.Flush()
}

// FileProvider implements a debugging provider that creates a real file for
// every call to NewFile. It maintains a list of all files that it creates,
// such that it can close them when its Flush function is called.
type FileProvider struct {
	Path string

	files []*os.File
}

func (fp *FileProvider) NewFile(p string) io.WriteCloser {
	f, err := os.Create(path.Join(fp.Path, p))
	if err != nil {
		panic(err)
	}

	fp.files = append(fp.files, f)

	return f
}

func (fp *FileProvider) Flush() {
	for _, f := range fp.files {
		f.Close()
	}
}
