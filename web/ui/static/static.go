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
//go:generate vfsgendev -source="github.com/prometheus/prometheus/web/ui/static".Assets

// Package static provides static assets via a virtual filesystem.
package static

import (
	"go/build"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/shurcooL/httpfs/filter"
)

func importPathToDir(importPath string) string {
	p, err := build.Import(importPath, "", build.FindOnly)
	if err != nil {
		log.Fatalln(err)
	}
	return p.Dir
}

// Assets contains the project's static assets.
var Assets http.FileSystem = filter.Keep(
	http.Dir(importPathToDir("github.com/prometheus/prometheus/web/ui/static")),
	func(path string, fi os.FileInfo) bool {
		switch {
		case fi.IsDir():
			return true
		case strings.HasPrefix(path, "/css") || strings.HasPrefix(path, "/img") || strings.HasPrefix(path, "/js") || strings.HasPrefix(path, "/vendor"):
			return !strings.HasSuffix(path, "map.js") &&
				!strings.HasSuffix(path, "/bootstrap.js") &&
				!strings.HasSuffix(path, "/bootstrap-theme.css") &&
				!strings.HasSuffix(path, "/bootstrap.css")
		}
		return false
	},
)
