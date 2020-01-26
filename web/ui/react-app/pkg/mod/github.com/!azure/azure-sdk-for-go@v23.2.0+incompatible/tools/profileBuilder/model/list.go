// +build go1.9

// Copyright 2018 Microsoft Corporation and contributors
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
	"fmt"
	"io"
	"os"
	"path"

	"github.com/marstr/collection"
)

// ListStrategy allows a mechanism for a list of packages that should be included in a profile.
type ListStrategy struct {
	io.Reader
}

// Enumerate reads a new line delimited list of packages names relative to $GOPATH
func (list ListStrategy) Enumerate(cancel <-chan struct{}) collection.Enumerator {
	results := make(chan interface{})

	go func() {
		defer close(results)

		var currentLine string

		for {
			_, err := fmt.Fscanln(list, &currentLine)
			if err != nil {
				return
			}

			currentLine = path.Join(os.Getenv("GOPATH"), "src", currentLine)

			select {
			case results <- currentLine:
				// Intentionally Left Blank
			case <-cancel:
				return
			}
		}
	}()

	return results
}
