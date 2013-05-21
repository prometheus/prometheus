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

// Builds a Fingerprint from a row key.
func NewFingerprintFromRowKey(rowKey string) *Fingerprint {
	components := strings.Split(rowKey, rowKeyDelimiter)
	hash, err := strconv.ParseUint(components[0], 10, 64)
	if err != nil {
		panic(err)
	}
	labelMatterLength, err := strconv.ParseUint(components[2], 10, 0)
	if err != nil {
		panic(err)
	}

	return &Fingerprint{
		hash: hash,
		firstCharacterOfFirstLabelName: components[1],
		labelMatterLength:              uint(labelMatterLength),
		lastCharacterOfLastLabelValue:  components[3],
	}
}

// Builds a Fingerprint from a datastore entry.
func NewFingerprintFromDTO(f *dto.Fingerprint) *Fingerprint {
	return NewFingerprintFromRowKey(*f.Signature)
}

// Decomposes a Metric into a Fingerprint.
func NewFingerprintFromMetric(metric Metric) *Fingerprint {
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
			lastCharacterOfLastLabelValue = string(labelValue[labelValueLength-1 : labelValueLength])
		}

		summer.Write([]byte(labelName))
		summer.Write([]byte(reservedDelimiter))
		summer.Write([]byte(labelValue))
	}

	return &Fingerprint{
		firstCharacterOfFirstLabelName: firstCharacterOfFirstLabelName,
		hash:                          binary.LittleEndian.Uint64(summer.Sum(nil)),
		labelMatterLength:             uint(labelMatterLength % 10),
		lastCharacterOfLastLabelValue: lastCharacterOfLastLabelValue,
	}
}

// A simplified representation of an entity.
type Fingerprint struct {
	// A hashed representation of the underyling entity.  For our purposes, FNV-1A
	// 64-bit is used.
	hash                           uint64
	firstCharacterOfFirstLabelName string
	labelMatterLength              uint
	lastCharacterOfLastLabelValue  string
}

func (f *Fingerprint) String() string {
	return f.ToRowKey()
}

// Transforms the Fingerprint into a database row key.
func (f *Fingerprint) ToRowKey() string {
	return strings.Join([]string{fmt.Sprintf("%020d", f.hash), f.firstCharacterOfFirstLabelName, fmt.Sprint(f.labelMatterLength), f.lastCharacterOfLastLabelValue}, rowKeyDelimiter)
}

func (f *Fingerprint) ToDTO() *dto.Fingerprint {
	return &dto.Fingerprint{
		Signature: proto.String(f.ToRowKey()),
	}
}

func (f *Fingerprint) Hash() uint64 {
	return f.hash
}

func (f *Fingerprint) FirstCharacterOfFirstLabelName() string {
	return f.firstCharacterOfFirstLabelName
}

func (f *Fingerprint) LabelMatterLength() uint {
	return f.labelMatterLength
}

func (f *Fingerprint) LastCharacterOfLastLabelValue() string {
	return f.lastCharacterOfLastLabelValue
}

func (f *Fingerprint) Less(o *Fingerprint) bool {
	if f.hash < o.hash {
		return true
	}
	if f.hash > o.hash {
		return false
	}

	if f.firstCharacterOfFirstLabelName < o.firstCharacterOfFirstLabelName {
		return true
	}
	if f.firstCharacterOfFirstLabelName > o.firstCharacterOfFirstLabelName {
		return false
	}

	if f.labelMatterLength < o.labelMatterLength {
		return true
	}
	if f.labelMatterLength > o.labelMatterLength {
		return false
	}

	if f.lastCharacterOfLastLabelValue < o.lastCharacterOfLastLabelValue {
		return true
	}
	if f.lastCharacterOfLastLabelValue > o.lastCharacterOfLastLabelValue {
		return false
	}
	return false
}

func (f *Fingerprint) Equal(o *Fingerprint) (equal bool) {
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
type Fingerprints []*Fingerprint

func (f Fingerprints) Len() int {
	return len(f)
}

func (f Fingerprints) Less(i, j int) bool {
	return f[i].Less(f[j])
}

func (f Fingerprints) Swap(i, j int) {
	f[i], f[j] = f[j], f[i]
}
