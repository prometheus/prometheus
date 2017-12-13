// Copyright 2014 The Cockroach Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
// implied. See the License for the specific language governing
// permissions and limitations under the License.
//
// Author: Spencer Kimball (spencer.kimball@gmail.com)

package httputil

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"strconv"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"
	"github.com/pkg/errors"
)

const (
	// AcceptHeader is the canonical header name for accept.
	AcceptHeader = "Accept"
	// AcceptEncodingHeader is the canonical header name for accept encoding.
	AcceptEncodingHeader = "Accept-Encoding"
	// ContentEncodingHeader is the canonical header name for content type.
	ContentEncodingHeader = "Content-Encoding"
	// ContentTypeHeader is the canonical header name for content type.
	ContentTypeHeader = "Content-Type"
	// JSONContentType is the JSON content type.
	JSONContentType = "application/json"
	// AltJSONContentType is the alternate JSON content type.
	AltJSONContentType = "application/x-json"
	// ProtoContentType is the protobuf content type.
	ProtoContentType = "application/x-protobuf"
	// AltProtoContentType is the alternate protobuf content type.
	AltProtoContentType = "application/x-google-protobuf"
	// PlaintextContentType is the plaintext content type.
	PlaintextContentType = "text/plain"
	// GzipEncoding is the gzip encoding.
	GzipEncoding = "gzip"
)

// GetJSON uses the supplied client to GET the URL specified by the parameters
// and unmarshals the result into response.
func GetJSON(httpClient http.Client, path string, response proto.Message) error {
	req, err := http.NewRequest("GET", path, nil)
	if err != nil {
		return err
	}
	return doJSONRequest(httpClient, req, response)
}

// PostJSON uses the supplied client to POST request to the URL specified by
// the parameters and unmarshals the result into response.
func PostJSON(httpClient http.Client, path string, request, response proto.Message) error {
	// Hack to avoid upsetting TestProtoMarshal().
	marshalFn := (&jsonpb.Marshaler{}).Marshal

	var buf bytes.Buffer
	if err := marshalFn(&buf, request); err != nil {
		return err
	}
	req, err := http.NewRequest("POST", path, &buf)
	if err != nil {
		return err
	}
	return doJSONRequest(httpClient, req, response)
}

func doJSONRequest(httpClient http.Client, req *http.Request, response proto.Message) error {
	if timeout := httpClient.Timeout; timeout > 0 {
		req.Header.Set("Grpc-Timeout", strconv.FormatInt(timeout.Nanoseconds(), 10)+"n")
	}
	req.Header.Set(AcceptHeader, JSONContentType)
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if contentType := resp.Header.Get(ContentTypeHeader); !(resp.StatusCode == http.StatusOK && contentType == JSONContentType) {
		b, err := ioutil.ReadAll(resp.Body)
		return errors.Errorf("status: %s, content-type: %s, body: %s, error: %v", resp.Status, contentType, b, err)
	}
	return jsonpb.Unmarshal(resp.Body, response)
}
