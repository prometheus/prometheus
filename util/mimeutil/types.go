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

package mimeutil

// GoogleProtobufMimeType defines the MIME type used to detect
// Protocol Buffers scrape protocol used by Prometheus.
// Note that official MIME type for Protobuf will be "application/protobuf",
// see https://protobuf.dev/reference/protobuf/mime-types/ and
// https://www.ietf.org/archive/id/draft-murray-dispatch-mime-protobuf-01.txt
// thus ProtobufMimeType name is reserved for later use.
const GoogleProtobufMimeType = "application/vnd.google.protobuf"
