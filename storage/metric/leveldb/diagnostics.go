// Copyright 2012 Prometheus Team
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

package leveldb

import (
	"code.google.com/p/goprotobuf/proto"
	"errors"
	"github.com/matttproud/prometheus/coding"
	"github.com/matttproud/prometheus/model"
	dto "github.com/matttproud/prometheus/model/generated"
	"github.com/matttproud/prometheus/utility"
)

func (l *LevelDBMetricPersistence) GetAllLabelNames() ([]string, error) {
	if getAll, getAllError := l.labelNameToFingerprints.GetAll(); getAllError == nil {
		result := make([]string, 0, len(getAll))
		labelNameDTO := &dto.LabelName{}

		for _, pair := range getAll {
			if unmarshalError := proto.Unmarshal(pair.Left, labelNameDTO); unmarshalError == nil {
				result = append(result, *labelNameDTO.Name)
			} else {
				return nil, unmarshalError
			}
		}

		return result, nil

	} else {
		return nil, getAllError
	}

	return nil, errors.New("Unknown error encountered when querying label names.")
}

func (l *LevelDBMetricPersistence) GetAllLabelPairs() ([]model.LabelSet, error) {
	if getAll, getAllError := l.labelSetToFingerprints.GetAll(); getAllError == nil {
		result := make([]model.LabelSet, 0, len(getAll))
		labelPairDTO := &dto.LabelPair{}

		for _, pair := range getAll {
			if unmarshalError := proto.Unmarshal(pair.Left, labelPairDTO); unmarshalError == nil {
				n := model.LabelName(*labelPairDTO.Name)
				v := model.LabelValue(*labelPairDTO.Value)
				item := model.LabelSet{n: v}
				result = append(result, item)
			} else {
				return nil, unmarshalError
			}
		}

		return result, nil

	} else {
		return nil, getAllError
	}

	return nil, errors.New("Unknown error encountered when querying label pairs.")
}

func (l *LevelDBMetricPersistence) GetAllMetrics() ([]model.LabelSet, error) {
	if getAll, getAllError := l.labelSetToFingerprints.GetAll(); getAllError == nil {
		result := make([]model.LabelSet, 0)
		fingerprintCollection := &dto.FingerprintCollection{}

		fingerprints := make(utility.Set)

		for _, pair := range getAll {
			if unmarshalError := proto.Unmarshal(pair.Right, fingerprintCollection); unmarshalError == nil {
				for _, member := range fingerprintCollection.Member {
					if !fingerprints.Has(*member.Signature) {
						fingerprints.Add(*member.Signature)
						fingerprintEncoded := coding.NewProtocolBufferEncoder(member)
						if labelPairCollectionRaw, labelPairCollectionRawError := l.fingerprintToMetrics.Get(fingerprintEncoded); labelPairCollectionRawError == nil {

							labelPairCollectionDTO := &dto.LabelSet{}

							if labelPairCollectionDTOMarshalError := proto.Unmarshal(labelPairCollectionRaw, labelPairCollectionDTO); labelPairCollectionDTOMarshalError == nil {
								intermediate := make(model.LabelSet, 0)

								for _, member := range labelPairCollectionDTO.Member {
									n := model.LabelName(*member.Name)
									v := model.LabelValue(*member.Value)
									intermediate[n] = v
								}

								result = append(result, intermediate)
							} else {
								return nil, labelPairCollectionDTOMarshalError
							}
						} else {
							return nil, labelPairCollectionRawError
						}
					}
				}
			} else {
				return nil, unmarshalError
			}
		}
		return result, nil
	} else {
		return nil, getAllError
	}

	return nil, errors.New("Unknown error encountered when querying metrics.")
}
