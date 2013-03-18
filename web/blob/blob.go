package blob

import (
	"bytes"
	"compress/gzip"
	"io"
	"log"
	"net/http"
)

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

	file, err := GetFile("static", name)
	if err != nil {
		if err != io.EOF {
			log.Printf("Could not get file: %s", err)
		}
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", types["static"][name])
	w.Write(file)
}
