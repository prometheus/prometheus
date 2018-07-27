// +build dev

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
