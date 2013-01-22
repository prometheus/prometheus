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

func (r *registry) ProcessorForRequestHeader(header http.Header) (processor Processor, err error) {
	if header == nil {
		err = fmt.Errorf("Received illegal and nil header.")
		return
	}

	prometheusApiVersion := header.Get("X-Prometheus-API-Version")

	switch prometheusApiVersion {
	case "0.0.1":
		processor = Processor001
		return
	default:
		err = fmt.Errorf("Unrecognized API version %s", prometheusApiVersion)
		return
	}

	return
}
