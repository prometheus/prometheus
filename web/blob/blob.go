package blob

//go:generate go-bindata -pkg blob -o files.go -ignore '(.*\.map|bootstrap\.js|bootstrap-theme\.css|bootstrap\.css)'  templates/... static/...

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/prometheus/log"

	"github.com/prometheus/prometheus/util/route"
)

// Sub-directories for templates and static content.
const (
	TemplateFiles = "templates"
	StaticFiles   = "static"
)

var mimeMap = map[string]string{
	"css":        "text/css",
	"js":         "text/javascript",
	"descriptor": "application/vnd.google.protobuf;proto=google.protobuf.FileDescriptorSet",
}

// GetFile retrieves the content of an embedded file.
func GetFile(bucket string, name string) ([]byte, error) {
	data, err := Asset(fmt.Sprintf("%s/%s", bucket, name))
	if err != nil {
		return nil, err
	}
	return data, nil
}

// Handler implements http.Handler.
type Handler struct{}

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := route.Context(r)

	name := strings.Trim(route.Param(ctx, "filepath"), "/")
	if name == "" {
		name = "index.html"
	}

	file, err := GetFile(StaticFiles, name)
	if err != nil {
		if err != io.EOF {
			log.Warn("Could not get file: ", err)
		}
		w.WriteHeader(http.StatusNotFound)
		return
	}
	contentType := http.DetectContentType(file)
	if strings.Contains(contentType, "text/plain") || strings.Contains(contentType, "application/octet-stream") {
		parts := strings.Split(name, ".")
		contentType = mimeMap[parts[len(parts)-1]]
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=259200")
	w.Write(file)
}
