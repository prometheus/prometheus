// Copyright 2016 The Cockroach Authors.
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
// Author: Tamir Duberstein (tamird@gmail.com)

package protoutil

import (
	"io"
	"io/ioutil"

	"github.com/gogo/protobuf/proto"
	gwruntime "github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/pkg/errors"

	"github.com/cockroachdb/cockroach/pkg/util/httputil"
)

var _ gwruntime.Marshaler = (*ProtoPb)(nil)

// ProtoPb is a gwruntime.Marshaler that uses github.com/gogo/protobuf/proto.
type ProtoPb struct{}

// ContentType implements gwruntime.Marshaler.
func (*ProtoPb) ContentType() string {
	return httputil.ProtoContentType
}

// Marshal implements gwruntime.Marshaler.
func (*ProtoPb) Marshal(v interface{}) ([]byte, error) {
	if p, ok := v.(proto.Message); ok {
		return Marshal(p)
	}
	return nil, errors.Errorf("unexpected type %T does not implement %s", v, typeProtoMessage)
}

// Unmarshal implements gwruntime.Marshaler.
func (*ProtoPb) Unmarshal(data []byte, v interface{}) error {
	if p, ok := v.(proto.Message); ok {
		return proto.Unmarshal(data, p)
	}
	return errors.Errorf("unexpected type %T does not implement %s", v, typeProtoMessage)
}

type protoDecoder struct {
	r io.Reader
}

// NewDecoder implements gwruntime.Marshaler.
func (*ProtoPb) NewDecoder(r io.Reader) gwruntime.Decoder {
	return &protoDecoder{r: r}
}

// Decode implements gwruntime.Marshaler.
func (d *protoDecoder) Decode(v interface{}) error {
	if p, ok := v.(proto.Message); ok {
		bytes, err := ioutil.ReadAll(d.r)
		if err == nil {
			err = proto.Unmarshal(bytes, p)
		}
		return err
	}
	return errors.Errorf("unexpected type %T does not implement %s", v, typeProtoMessage)
}

type protoEncoder struct {
	w io.Writer
}

// NewEncoder implements gwruntime.Marshaler.
func (*ProtoPb) NewEncoder(w io.Writer) gwruntime.Encoder {
	return &protoEncoder{w: w}
}

// Encode implements gwruntime.Marshaler.
func (e *protoEncoder) Encode(v interface{}) error {
	if p, ok := v.(proto.Message); ok {
		bytes, err := Marshal(p)
		if err == nil {
			_, err = e.w.Write(bytes)
		}
		return err
	}
	return errors.Errorf("unexpected type %T does not implement %s", v, typeProtoMessage)
}
