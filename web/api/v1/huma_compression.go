// Copyright The Prometheus Authors
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

package v1

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

// compressedResponseWriter wraps http.ResponseWriter to add compression.
type compressedResponseWriter struct {
	http.ResponseWriter
	writer io.Writer
}

// Write writes data to the compression writer.
func (c *compressedResponseWriter) Write(p []byte) (int, error) {
	return c.writer.Write(p)
}

// Flush flushes both the compression writer and underlying writer.
// This is critical for streaming responses like SSE.
func (c *compressedResponseWriter) Flush() {
	// Flush the compression writer first.
	switch w := c.writer.(type) {
	case *gzip.Writer:
		w.Flush()
	case *zlib.Writer:
		w.Flush()
	}

	// Then flush the underlying HTTP writer.
	if flusher, ok := c.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// Unwrap returns the underlying ResponseWriter.
// This is needed to allow access to interfaces like http.Flusher
// that may be implemented by the underlying writer, which is required
// for Server-Sent Events (SSE) and other streaming responses.
func (c *compressedResponseWriter) Unwrap() http.ResponseWriter {
	return c.ResponseWriter
}

// Close flushes and closes the compression writer.
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

// newCompressedResponseWriter creates a new compressed response writer based on Accept-Encoding.
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

// CompressionHandler wraps an HTTP handler to add compression support.
type CompressionHandler struct {
	Handler http.Handler
}

// ServeHTTP adds compression to the original handler's ServeHTTP method.
func (c CompressionHandler) ServeHTTP(writer http.ResponseWriter, req *http.Request) {
	compWriter := newCompressedResponseWriter(writer, req)
	c.Handler.ServeHTTP(compWriter, req)
	compWriter.Close()
}
