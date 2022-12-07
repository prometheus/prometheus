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
	"compress/gzip"
	"compress/zlib"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
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
	switch selectCoding(req.Header.Get(acceptEncodingHeader), []string{gzipEncoding, deflateEncoding}) {
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
	default:
		return &compressedResponseWriter{
			ResponseWriter: writer,
			writer:         writer,
		}
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

// selectCoding selects one of the available content-codings based on the value of the Accept-Encoding header.
// In case of errors or no available coding being accepted, empty string is returned.
func selectCoding(acceptEncodingHeader string, availableCodings []string) string {
	const (
		wildcard = "*"
		identity = "identity"
	)

	if len(availableCodings) == 0 || acceptEncodingHeader == "" {
		// Shortcut for no header or no available codings provided.
		return ""
	}

	acceptedWeights, ok := parseAcceptedCodings(acceptEncodingHeader)
	if !ok {
		// Can't parse Accept-Encoding header.
		return ""
	}

	var (
		bestCoding string
		bestWeight float64
	)
	ww := acceptedWeights[wildcard]
	for _, c := range availableCodings {
		if w, ok := acceptedWeights[c]; ok && w > 0 {
			// If coding is accepted and it has a positive weight, check if it's our best option so far.
			if w > bestWeight {
				bestCoding, bestWeight = c, w
			}
		} else if !ok && ww > 0 {
			// If coding is not explicitly accepted, but there's a positive wildcard weight, consider that.
			if ww > bestWeight {
				bestCoding, bestWeight = c, ww
			}
		}
	}
	// Check if identity was a better option.
	if w := acceptedWeights[identity]; w > bestWeight {
		return ""
	}

	return bestCoding
}

// parseAcceptedCodings parses the Accept-Encoding header into a map of codings with their qvalue weights.
// If bool param is false, then parsing failed.
func parseAcceptedCodings(acceptEncodingHeader string) (map[string]float64, bool) {
	c := make(map[string]float64)
	for _, ss := range strings.Split(acceptEncodingHeader, ",") {
		coding, qvalue := parseCoding(ss)
		if coding == "" {
			return nil, false
		}
		c[coding] = qvalue
	}
	return c, true
}

// parseCoding parses a content coding and it's weight,
// as defined by https://datatracker.ietf.org/doc/html/rfc7231#section-5.3.4
// If any issues are found while parsing the coding (especially the weight), an empty coding name is returned.
func parseCoding(s string) (string, float64) {
	parts := strings.SplitN(strings.ToLower(s), ";", 2)
	c := strings.TrimSpace(parts[0])
	w := 1.0

	if len(parts) == 2 {
		var err error
		w, err = strconv.ParseFloat(strings.TrimPrefix(strings.TrimSpace(parts[1]), "q="), 64)
		if err != nil {
			return "", 0
		}
		w = math.Max(math.Min(1, w), 0)
	}

	return c, w
}
