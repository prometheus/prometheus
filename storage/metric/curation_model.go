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

package metric

import (
	"bytes"
	"fmt"
	"time"

	"code.google.com/p/goprotobuf/proto"

	clientmodel "github.com/prometheus/client_golang/model"

	dto "github.com/prometheus/prometheus/model/generated"
)

// curationRemark provides a representation of dto.CurationValue with associated
// business logic methods attached to it to enhance code readability.
type curationRemark struct {
	LastCompletionTimestamp time.Time
}

// OlderThan answers whether this curationRemark is older than the provided
// cutOff time.
func (c *curationRemark) OlderThan(t time.Time) bool {
	return c.LastCompletionTimestamp.Before(t)
}

// Equal answers whether the two curationRemarks are equivalent.
func (c *curationRemark) Equal(o curationRemark) bool {
	return c.LastCompletionTimestamp.Equal(o.LastCompletionTimestamp)
}

func (c *curationRemark) String() string {
	return fmt.Sprintf("Last curated at %s", c.LastCompletionTimestamp)
}

// // ToDTO generates the dto.CurationValue representation of this.
// func (c *curationRemark) ToDTO() *dto.CurationValue {
// 	return &dto.CurationValue{
// 		LastCompletionTimestamp: proto.Int64(c.LastCompletionTimestamp.Unix()),
// 	}
// }

// NewcurationRemarkFromDTO builds curationRemark from the provided
// dto.CurationValue object.
// func NewcurationRemarkFromDTO(d *dto.CurationValue) curationRemark {
// 	return curationRemark{
// 		LastCompletionTimestamp: time.Unix(*d.LastCompletionTimestamp, 0).UTC(),
// 	}
// }

// curationKey provides a representation of dto.CurationKey with associated
// business logic methods attached to it to enhance code readability.
type curationKey struct {
	Fingerprint              *clientmodel.Fingerprint
	ProcessorMessageRaw      []byte
	ProcessorMessageTypeName string
	IgnoreYoungerThan        time.Duration
}

// Equal answers whether the two curationKeys are equivalent.
func (c *curationKey) Equal(o *curationKey) bool {
	switch {
	case !c.Fingerprint.Equal(o.Fingerprint):
		return false
	case bytes.Compare(c.ProcessorMessageRaw, o.ProcessorMessageRaw) != 0:
		return false
	case c.ProcessorMessageTypeName != o.ProcessorMessageTypeName:
		return false
	case c.IgnoreYoungerThan != o.IgnoreYoungerThan:
		return false
	}

	return true
}

// // ToDTO generates a dto.CurationKey representation of this.
// func (c curationKey) ToDTO() *dto.CurationKey {
// 	return &dto.curationKey{
// 		Fingerprint:              c.Fingerprint.ToDTO(),
// 		ProcessorMessageRaw:      c.ProcessorMessageRaw,
// 		ProcessorMessageTypeName: proto.String(c.ProcessorMessageTypeName),
// 		IgnoreYoungerThan:        proto.Int64(int64(c.IgnoreYoungerThan)),
// 	}
// }

// // NewCurationKeyFromDTO builds CurationKey from the provided dto.CurationKey.
// func NewCurationKeyFromDTO(d *dto.CurationKey) CurationKey {
// 	return CurationKey{
// 		Fingerprint:              NewFingerprintFromDTO(d.Fingerprint),
// 		ProcessorMessageRaw:      d.ProcessorMessageRaw,
// 		ProcessorMessageTypeName: *d.ProcessorMessageTypeName,
// 		IgnoreYoungerThan:        time.Duration(*d.IgnoreYoungerThan),
// 	}
// }
