// Copyright 2023 The Prometheus Authors
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

package scrape

import (
	"bytes"
	"encoding/binary"

	"github.com/gogo/protobuf/proto"

	// Intentionally using client model to simulate client in tests.
	dto "github.com/prometheus/client_model/go"
)

// Write a MetricFamily into a protobuf.
// This function is intended for testing scraping by providing protobuf serialized input.
func MetricFamilyToProtobuf(metricFamily *dto.MetricFamily) ([]byte, error) {
	buffer := &bytes.Buffer{}
	err := AddMetricFamilyToProtobuf(buffer, metricFamily)
	if err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// Append a MetricFamily protobuf representation to a buffer.
// This function is intended for testing scraping by providing protobuf serialized input.
func AddMetricFamilyToProtobuf(buffer *bytes.Buffer, metricFamily *dto.MetricFamily) error {
	protoBuf, err := proto.Marshal(metricFamily)
	if err != nil {
		return err
	}

	varintBuf := make([]byte, binary.MaxVarintLen32)
	varintLength := binary.PutUvarint(varintBuf, uint64(len(protoBuf)))

	_, err = buffer.Write(varintBuf[:varintLength])
	if err != nil {
		return err
	}
	_, err = buffer.Write(protoBuf)
	return err
}
