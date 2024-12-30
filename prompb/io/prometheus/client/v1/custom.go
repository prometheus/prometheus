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
	"fmt"
	"io"

	"github.com/VictoriaMetrics/easyproto"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/runtime/protoimpl"

	"github.com/prometheus/prometheus/model/labels"
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

// ParseLabels return labels
//
//	 Metric(l *labels.Labels) string to build labels from builder.
//		(Same for exemplar)
//	 For updateMetricBytes.
func (x *Metric) ParseLabels(b *labels.ScratchBuilder) error {
	if x == nil {
		return nil
	}
	if !protoimpl.X.Present(&(x.XXX_presence[0]), 0) {
		return nil
	}
	if !protoimpl.X.AtomicCheckPointerIsNil(&x.xxx_hidden_Label) {
		panic("GetLabel were called in the code, which is inefficient, no point in using ParseLabels; replace all calls with ParseLabels")
	}

	// protoimpl.X.UnmarshalField(x, 1)

	// TODO(bwplotka): This is deprecated. Switch to vtprotobuf once new API
	// will be supported https://github.com/planetscale/vtprotobuf/issues/150
	src := x.XXX_lazyUnmarshalInfo.Protobuf
	var (
		fc  easyproto.FieldContext
		err error
	)
	for len(src) > 0 {
		src, err = fc.NextField(src)
		if err != nil {
			return err
		}
		if fc.FieldNum == 1 { // Labels.
			data, ok := fc.MessageData()
			if !ok {
				return fmt.Errorf("Label parsing error, got %v", string(src))
			}
			// Another element of LabelPair.
			if err := parseLabelPair(data, b); err != nil {
				return fmt.Errorf("LabelPair parsing error, got %v", string(data))
			}
		}
	}
	return nil
}

func parseLabelPair(src []byte, b *labels.ScratchBuilder) error {
	var (
		fc          easyproto.FieldContext
		err         error
		ok          bool
		name, value string
	)
	for len(src) > 0 {
		src, err = fc.NextField(src)
		if err != nil {
			return err
		}
		switch fc.FieldNum {
		case 1: // Name.
			name, ok = fc.String() // Reused from proto bytes.
			if !ok {
				return fmt.Errorf("Name parsing error, got %v", string(src))
			}
		case 2: // Value.
			value, ok = fc.String() // Reused from proto bytes.
			if !ok {
				return fmt.Errorf("Value parsing error, got %v", string(src))
			}
		}
	}
	b.Add(name, value)
	return nil
}
