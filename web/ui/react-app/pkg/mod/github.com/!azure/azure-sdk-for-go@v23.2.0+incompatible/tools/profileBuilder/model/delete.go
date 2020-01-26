// +build go1.9

// Copyright 2018 Microsoft Corporation
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

package model

import (
	"io/ioutil"
	"os"
	"path"
	"strings"
)

// DeleteChildDirs deletes all child directories of the directory specified by
// `path`.
// If it fails to delete a child, it halts execution and returns an err immediately.
func DeleteChildDirs(dir string) (err error) {
	var children []os.FileInfo

	children, err = ioutil.ReadDir(dir)
	if err != nil {
		return
	}

	for _, child := range children {
		if child.IsDir() {
			childPath := strings.Replace(path.Join(dir, child.Name()), `\`, `/`, -1)
			err = os.RemoveAll(childPath)
			if err != nil {
				return
			}
		}
	}

	return
}
