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
	// rowKeyDelimiter is used to separate formatted versions of a metric's row
	// key.
	rowKeyDelimiter = "-"
)

// Provides a compact representation of a Metric.
type Fingerprint interface {
	// Transforms the fingerprint into a database row key.
	ToRowKey() string
	Hash() uint64
	FirstCharacterOfFirstLabelName() string
	LabelMatterLength() uint
	LastCharacterOfLastLabelValue() string
	ToDTO() *dto.Fingerprint
	Less(Fingerprint) bool
	Equal(Fingerprint) bool
}

// Builds a Fingerprint from a row key.
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

// Builds a Fingerprint from a datastore entry.
func NewFingerprintFromDTO(f *dto.Fingerprint) Fingerprint {
	return NewFingerprintFromRowKey(*f.Signature)
}

// Decomposes a Metric into a Fingerprint.
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

		if i == 0 {
			firstCharacterOfFirstLabelName = labelName[0:1]
		}
		if i == labelLength-1 {
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
	return strings.Join([]string{fmt.Sprintf("%020d", f.hash), f.firstCharacterOfFirstLabelName, fmt.Sprint(f.labelMatterLength), f.lastCharacterOfLastLabelValue}, rowKeyDelimiter)
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

func (f fingerprint) Less(o Fingerprint) bool {
	if f.Hash() < o.Hash() {
		return true
	}
	if f.FirstCharacterOfFirstLabelName() < o.FirstCharacterOfFirstLabelName() {
		return true
	}
	if f.LabelMatterLength() < o.LabelMatterLength() {
		return true
	}
	if f.LastCharacterOfLastLabelValue() < o.LastCharacterOfLastLabelValue() {
		return true
	}

	return false
}

func (f fingerprint) Equal(o Fingerprint) (equal bool) {
	equal = f.Hash() == o.Hash()
	if !equal {
		return
	}

	equal = f.FirstCharacterOfFirstLabelName() == o.FirstCharacterOfFirstLabelName()
	if !equal {
		return
	}

	equal = f.LabelMatterLength() == o.LabelMatterLength()
	if !equal {
		return
	}

	equal = f.LastCharacterOfLastLabelValue() == o.LastCharacterOfLastLabelValue()

	return
}

// Represents a collection of Fingerprint subject to a given natural sorting
// scheme.
type Fingerprints []Fingerprint

func (f Fingerprints) Len() int {
	return len(f)
}

func (f Fingerprints) Less(i, j int) (less bool) {
	return f[i].Less(f[j])
}

func (f Fingerprints) Swap(i, j int) {
	f[i], f[j] = f[j], f[i]
}
