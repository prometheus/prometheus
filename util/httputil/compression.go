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
	"io"
	"net/http"
	"strings"

	"github.com/klauspost/compress/gzip"
	"github.com/klauspost/compress/zlib"
)

const (
	acceptEncodingHeader  = "Accept-Encoding"
	contentEncodingHeader = "Content-Encoding"
	gzipEncoding          = "gzip"
	deflateEncoding       = "deflate"
)

// Wrapper around http.Handler which adds suitable response compression based
// on the client's Accept-Encoding headers.
type compressedResponseWriter struct {
	http.ResponseWriter
	writer io.Writer
}

// Writes HTTP response content data.
func (c *compressedResponseWriter) Write(p []byte) (int, error) {
	return c.writer.Write(p)
}

// Closes the compressedResponseWriter and ensures to flush all data before.
func (c *compressedResponseWriter) Close() {
	if zlibWriter, ok := c.writer.(*zlib.Writer); ok {
		zlibWriter.Flush()
	}
	if gzipWriter, ok := c.writer.(*gzip.Writer); ok {
		gzipWriter.Flush()
	}
	if closer, ok := c.writer.(io.Closer); ok {
		defer closer.Close()
	}
}

// Constructs a new compressedResponseWriter based on client request headers.
func newCompressedResponseWriter(writer http.ResponseWriter, req *http.Request) *compressedResponseWriter {
	raw := req.Header.Get(acceptEncodingHeader)
	var (
		encoding   string
		commaFound bool
	)
	for {
		encoding, raw, commaFound = strings.Cut(raw, ",")
		switch strings.TrimSpace(encoding) {
		case gzipEncoding:
			writer.Header().Set(contentEncodingHeader, gzipEncoding)
			return &compressedResponseWriter{
				ResponseWriter: writer,
				writer:         gzip.NewWriter(writer),
			}
		case deflateEncoding:
			writer.Header().Set(contentEncodingHeader, deflateEncoding)
			return &compressedResponseWriter{
				ResponseWriter: writer,
				writer:         zlib.NewWriter(writer),
			}
		}
		if !commaFound {
			break
		}
	}
	return &compressedResponseWriter{
		ResponseWriter: writer,
		writer:         writer,
	}
}

// CompressionHandler is a wrapper around http.Handler which adds suitable
// response compression based on the client's Accept-Encoding headers.
type CompressionHandler struct {
	Handler http.Handler
}

// ServeHTTP adds compression to the original http.Handler's ServeHTTP() method.
func (c CompressionHandler) ServeHTTP(writer http.ResponseWriter, req *http.Request) {
	compWriter := newCompressedResponseWriter(writer, req)
	c.Handler.ServeHTTP(compWriter, req)
	compWriter.Close()
}
