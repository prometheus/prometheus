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

package httputil

import (
	"compress/zlib"
	"io"
	"net/http"
	"strings"

	"github.com/klauspost/compress/gzhttp"
)

const (
	acceptEncodingHeader  = "Accept-Encoding"
	contentEncodingHeader = "Content-Encoding"
	gzipEncoding          = "gzip"
	deflateEncoding       = "deflate"
)

// Wrapper around http.Handler which adds suitable response compression based
// on the client's Accept-Encoding headers.
type deflatedResponseWriter struct {
	http.ResponseWriter
	writer io.Writer
}

// Writes HTTP response content data.
func (c *deflatedResponseWriter) Write(p []byte) (int, error) {
	return c.writer.Write(p)
}

// Closes the deflatedResponseWriter and ensures to flush all data before.
func (c *deflatedResponseWriter) Close() {
	if zlibWriter, ok := c.writer.(*zlib.Writer); ok {
		zlibWriter.Flush()
	}
	if closer, ok := c.writer.(io.Closer); ok {
		defer closer.Close()
	}
}

// Constructs a new deflatedResponseWriter based on client request headers.
func newDeflateResponseWriter(writer http.ResponseWriter) *deflatedResponseWriter {
	writer.Header().Set(contentEncodingHeader, deflateEncoding)
	return &deflatedResponseWriter{
		ResponseWriter: writer,
		writer:         zlib.NewWriter(writer),
	}
}

// CompressionHandler is a wrapper around http.Handler which adds suitable
// response compression based on the client's Accept-Encoding headers.
type CompressionHandler struct {
	Handler http.Handler
}

// ServeHTTP adds compression to the original http.Handler's ServeHTTP() method.
func (c CompressionHandler) ServeHTTP(writer http.ResponseWriter, req *http.Request) {
	encodings := strings.Split(req.Header.Get(acceptEncodingHeader), ",")
	for _, encoding := range encodings {
		switch strings.TrimSpace(encoding) {
		case gzipEncoding:
			gzhttp.GzipHandler(c.Handler).ServeHTTP(writer, req)
			return
		case deflateEncoding:
			compWriter := newDeflateResponseWriter(writer)
			c.Handler.ServeHTTP(compWriter, req)
			compWriter.Close()
			return
		default:
			c.Handler.ServeHTTP(writer, req)
			return
		}
	}
}
