// Copyright 2016 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package v1

import "github.com/munnerz/goautoneg"

// A Codec performs encoding of API responses.
type Codec interface {
	// ContentType returns the MIME time that this Codec emits.
	ContentType() MIMEType

	// CanEncode determines if this Codec can encode resp.
	CanEncode(resp *Response) bool

	// Encode encodes resp, ready for transmission to an API consumer.
	Encode(resp *Response) ([]byte, error)
}

type MIMEType struct {
	Type    string
	SubType string
}

func (m MIMEType) String() string {
	return m.Type + "/" + m.SubType
}

func (m MIMEType) Satisfies(accept goautoneg.Accept) bool {
	if accept.Type == "*" && accept.SubType == "*" {
		return true
	}

	if accept.Type == m.Type && accept.SubType == "*" {
		return true
	}

	if accept.Type == m.Type && accept.SubType == m.SubType {
		return true
	}

	return false
}
