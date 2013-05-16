// Copyright 2013 Prometheus Team
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

package format

import (
	"fmt"
	"mime"
	"net/http"
)

var (
	DefaultRegistry Registry = &registry{}
)

// Registry is responsible for applying a determination strategy to the given
// inputs to determine what Processor can handle this type of input.
type Registry interface {
	// ProcessorForRequestHeader interprets a HTTP request header to determine
	// what Processor should be used for the given input.
	ProcessorForRequestHeader(header http.Header) (Processor, error)
}

type registry struct {
}

func (r *registry) ProcessorForRequestHeader(header http.Header) (Processor, error) {
	if header == nil {
		return nil, fmt.Errorf("Received illegal and nil header.")
	}

	mediatype, params, err := mime.ParseMediaType(header.Get("Content-Type"))

	if err != nil {
		return nil, fmt.Errorf("Invalid Content-Type header %q: %s", header.Get("Content-Type"), err)
	}

	if mediatype != "application/json" {
		return nil, fmt.Errorf("Unsupported media type %q, expected %q", mediatype, "application/json")
	}

	var prometheusApiVersion string

	if params["schema"] == "prometheus/telemetry" && params["version"] != "" {
		prometheusApiVersion = params["version"]
	} else {
		prometheusApiVersion = header.Get("X-Prometheus-API-Version")
	}

	switch prometheusApiVersion {
	case "0.0.2":
		return Processor002, nil
	case "0.0.1":
		return Processor001, nil
	default:
		return nil, fmt.Errorf("Unrecognized API version %s", prometheusApiVersion)
	}
}
