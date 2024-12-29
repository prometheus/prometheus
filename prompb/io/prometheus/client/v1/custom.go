// Copyright 2024 Prometheus Team
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

package clientv1

import (
	"io"

	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
)

func MetricFamilyFromTextToDelimitedProto(text []byte, w io.Writer) error {
	mf := &MetricFamily{}
	if err := prototext.Unmarshal(text, mf); err != nil {
		return err
	}
	// From proto message to binary protobuf.
	protoBuf, err := proto.Marshal(mf)
	if err != nil {
		return err
	}
	// Write first length, then binary protobuf.
	varintBuf := protowire.AppendVarint(nil, uint64(len(protoBuf)))
	if _, err := w.Write(varintBuf); err != nil {
		return err
	}
	if _, err := w.Write(protoBuf); err != nil {
		return err
	}
	return nil
}
