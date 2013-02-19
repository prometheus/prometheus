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
	"code.google.com/p/goprotobuf/proto"
	"encoding/binary"
	"fmt"
	dto "github.com/prometheus/prometheus/model/generated"
	"hash/fnv"
	"sort"
	"strconv"
	"strings"
)

const (
	rowKeyDelimiter = "-"
)

// Provides a compact representation of a Metric.
type Fingerprint interface {
	// Get the base value for the data store row key.
	ToRowKey() string
	// Return the primitive Hash value.
	Hash() uint64
	FirstCharacterOfFirstLabelName() string
	LabelMatterLength() uint
	LastCharacterOfLastLabelValue() string
	ToDTO() *dto.Fingerprint
}

func NewFingerprintFromRowKey(rowKey string) (f Fingerprint) {
	components := strings.Split(rowKey, rowKeyDelimiter)
	hash, err := strconv.ParseUint(components[0], 10, 64)
	if err != nil {
		panic(err)
	}
	labelMatterLength, err := strconv.ParseUint(components[2], 10, 0)
	if err != nil {
		panic(err)
	}

	return fingerprint{
		hash: hash,
		firstCharacterOfFirstLabelName: components[1],
		labelMatterLength:              uint(labelMatterLength),
		lastCharacterOfLastLabelValue:  components[3],
	}
}

func NewFingerprintFromMetric(metric Metric) (f Fingerprint) {
	labelLength := len(metric)
	labelNames := make([]string, 0, labelLength)

	for labelName := range metric {
		labelNames = append(labelNames, string(labelName))
	}

	sort.Strings(labelNames)

	summer := fnv.New64a()
	firstCharacterOfFirstLabelName := ""
	lastCharacterOfLastLabelValue := ""
	labelMatterLength := 0

	for i, labelName := range labelNames {
		labelValue := metric[LabelName(labelName)]
		labelNameLength := len(labelName)
		labelValueLength := len(labelValue)
		labelMatterLength += labelNameLength + labelValueLength

		switch i {
		case 0:
			firstCharacterOfFirstLabelName = labelName[0:1]
		case labelLength - 1:
			lastCharacterOfLastLabelValue = string(labelValue[labelValueLength-2 : labelValueLength-1])
		}

		summer.Write([]byte(labelName))
		summer.Write([]byte(reservedDelimiter))
		summer.Write([]byte(labelValue))
	}

	return fingerprint{
		firstCharacterOfFirstLabelName: firstCharacterOfFirstLabelName,
		hash:                           binary.LittleEndian.Uint64(summer.Sum(nil)),
		labelMatterLength:              uint(labelMatterLength),
		lastCharacterOfLastLabelValue:  lastCharacterOfLastLabelValue,
	}
}

// A simplified representation of an entity.
type fingerprint struct {
	// A hashed representation of the underyling entity.  For our purposes, FNV-1A
	// 64-bit is used.
	hash                           uint64
	firstCharacterOfFirstLabelName string
	labelMatterLength              uint
	lastCharacterOfLastLabelValue  string
}

func (f fingerprint) ToRowKey() string {
	return strings.Join([]string{fmt.Sprint(f.hash), f.firstCharacterOfFirstLabelName, fmt.Sprint(f.labelMatterLength), f.lastCharacterOfLastLabelValue}, rowKeyDelimiter)
}

func (f fingerprint) ToDTO() *dto.Fingerprint {
	return &dto.Fingerprint{
		Signature: proto.String(f.ToRowKey()),
	}
}

func (f fingerprint) Hash() uint64 {
	return f.hash
}

func (f fingerprint) FirstCharacterOfFirstLabelName() string {
	return f.firstCharacterOfFirstLabelName
}

func (f fingerprint) LabelMatterLength() uint {
	return f.labelMatterLength
}

func (f fingerprint) LastCharacterOfLastLabelValue() string {
	return f.lastCharacterOfLastLabelValue
}

// Represents a collection of Fingerprint subject to a given natural sorting
// scheme.
type Fingerprints []Fingerprint

func (f Fingerprints) Len() int {
	return len(f)
}

func (f Fingerprints) Less(i, j int) (less bool) {
	this := f[i]
	other := f[j]

	less = sort.IntsAreSorted([]int{int(this.Hash()), int(other.Hash())})
	if !less {
		return
	}

	less = sort.StringsAreSorted([]string{this.FirstCharacterOfFirstLabelName(), other.FirstCharacterOfFirstLabelName()})
	if !less {
		return
	}

	less = sort.IntsAreSorted([]int{int(this.LabelMatterLength()), int(other.LabelMatterLength())})
	if !less {
		return
	}

	less = sort.StringsAreSorted([]string{this.LastCharacterOfLastLabelValue(), other.LastCharacterOfLastLabelValue()})

	return
}

func (f Fingerprints) Swap(i, j int) {
	f[i], f[j] = f[j], f[i]
}
