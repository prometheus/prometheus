// Copyright 2013 The Prometheus Authors
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

package static

import (
	"go/build"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/shurcooL/httpfs/filter"
	"github.com/shurcooL/httpfs/union"
)

func importPathToDir(importPath string) string {
	p, err := build.Import(importPath, "", build.FindOnly)
	if err != nil {
		log.Fatalln(err)
	}
	return p.Dir
}

func getAsset(path string) http.FileSystem {
	return filter.Keep(
		http.Dir(filepath.Join(importPathToDir("github.com/prometheus/prometheus/web/ui/static"), path)),
		func(path string, fi os.FileInfo) bool {
			return fi.IsDir() ||
				(!strings.HasSuffix(path, "map.js") &&
					!strings.HasSuffix(path, "/bootstrap.js") &&
					!strings.HasSuffix(path, "/bootstrap-theme.css") &&
					!strings.HasSuffix(path, "/bootstrap.css"))
		},
	)
}

// Assets contains the project's static assets.
var Assets http.FileSystem = union.New(map[string]http.FileSystem{
	"/css":    getAsset("css"),
	"/img":    getAsset("img"),
	"/js":     getAsset("js"),
	"/vendor": getAsset("vendor"),
})
