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

// +build dev

package ui

import (
	"net/http"
	"os"
	"strings"

	"github.com/shurcooL/httpfs/filter"
	"github.com/shurcooL/httpfs/union"
)

var static http.FileSystem = filter.Keep(
	http.Dir("./static"),
	func(path string, fi os.FileInfo) bool {
		return fi.IsDir() ||
			(!strings.HasSuffix(path, "map.js") &&
				!strings.HasSuffix(path, "/bootstrap.js") &&
				!strings.HasSuffix(path, "/bootstrap-theme.css") &&
				!strings.HasSuffix(path, "/bootstrap.css"))
	},
)

var templates http.FileSystem = filter.Keep(
	http.Dir("./templates"),
	func(path string, fi os.FileInfo) bool {
		return fi.IsDir() || strings.HasSuffix(path, ".html")
	},
)

// Assets contains the project's assets.
var Assets http.FileSystem = union.New(map[string]http.FileSystem{
	"/templates": templates,
	"/static":    static,
})
