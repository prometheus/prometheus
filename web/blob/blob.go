package blob

import (
	"bytes"
	"compress/gzip"
	"io"
	"log"
	"net/http"
	"strings"
)

const (
	TemplateFiles = "templates"
	StaticFiles   = "static"
)

var mimeMap = map[string]string{
	"css": "text/css",
	"js":  "text/javascript",
}

func GetFile(bucket string, name string) ([]byte, error) {
	reader := bytes.NewReader(files[bucket][name])
	gz, err := gzip.NewReader(reader)
	if err != nil {
		return nil, err
	}

	var b bytes.Buffer
	io.Copy(&b, gz)
	gz.Close()

	return b.Bytes(), nil
}

type Handler struct{}

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := r.URL.String()
	if name == "" {
		name = "index.html"
	}

	file, err := GetFile(StaticFiles, name)
	if err != nil {
		if err != io.EOF {
			log.Printf("Could not get file: %s", err)
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
	w.Write(file)
}
