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

package model

import (
	"bytes"
	"code.google.com/p/goprotobuf/proto"
	"fmt"
	dto "github.com/prometheus/prometheus/model/generated"
	"time"
)

// CurationRemark provides a representation of dto.CurationValue with associated
// business logic methods attached to it to enhance code readability.
type CurationRemark struct {
	LastCompletionTimestamp time.Time
}

// OlderThan answers whether this CurationRemark is older than the provided
// cutOff time.
func (c CurationRemark) OlderThan(t time.Time) bool {
	return c.LastCompletionTimestamp.Before(t)
}

// Equal answers whether the two CurationRemarks are equivalent.
func (c CurationRemark) Equal(o CurationRemark) bool {
	return c.LastCompletionTimestamp.Equal(o.LastCompletionTimestamp)
}

func (c CurationRemark) String() string {
	return fmt.Sprintf("Last curated at %s", c.LastCompletionTimestamp)
}

// ToDTO generates the dto.CurationValue representation of this.
func (c CurationRemark) ToDTO() *dto.CurationValue {
	return &dto.CurationValue{
		LastCompletionTimestamp: proto.Int64(c.LastCompletionTimestamp.Unix()),
	}
}

// NewCurationRemarkFromDTO builds CurationRemark from the provided
// dto.CurationValue object.
func NewCurationRemarkFromDTO(d *dto.CurationValue) CurationRemark {
	return CurationRemark{
		LastCompletionTimestamp: time.Unix(*d.LastCompletionTimestamp, 0).UTC(),
	}
}

// CurationKey provides a representation of dto.CurationKey with asociated
// business logic methods attached to it to enhance code readability.
type CurationKey struct {
	Fingerprint              Fingerprint
	ProcessorMessageRaw      []byte
	ProcessorMessageTypeName string
	IgnoreYoungerThan        time.Duration
}

// Equal answers whether the two CurationKeys are equivalent.
func (c CurationKey) Equal(o CurationKey) (equal bool) {
	switch {
	case !c.Fingerprint.Equal(o.Fingerprint):
		return
	case bytes.Compare(c.ProcessorMessageRaw, o.ProcessorMessageRaw) != 0:
		return
	case c.ProcessorMessageTypeName != o.ProcessorMessageTypeName:
		return
	case c.IgnoreYoungerThan != o.IgnoreYoungerThan:
		return
	}

	return true
}

// ToDTO generates a dto.CurationKey representation of this.
func (c CurationKey) ToDTO() *dto.CurationKey {
	return &dto.CurationKey{
		Fingerprint:              c.Fingerprint.ToDTO(),
		ProcessorMessageRaw:      c.ProcessorMessageRaw,
		ProcessorMessageTypeName: proto.String(c.ProcessorMessageTypeName),
		IgnoreYoungerThan:        proto.Int64(int64(c.IgnoreYoungerThan)),
	}
}

// NewCurationKeyFromDTO builds CurationKey from the provided dto.CurationKey.
func NewCurationKeyFromDTO(d *dto.CurationKey) CurationKey {
	return CurationKey{
		Fingerprint:              NewFingerprintFromDTO(d.Fingerprint),
		ProcessorMessageRaw:      d.ProcessorMessageRaw,
		ProcessorMessageTypeName: *d.ProcessorMessageTypeName,
		IgnoreYoungerThan:        time.Duration(*d.IgnoreYoungerThan),
	}
}
