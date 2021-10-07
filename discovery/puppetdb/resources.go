// Copyright 2021 The Prometheus Authors
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

package puppetdb

import (
	"strconv"
	"strings"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/util/strutil"
)

type Resource struct {
	Certname    string     `json:"certname"`
	Resource    string     `json:"resource"`
	Type        string     `json:"type"`
	Title       string     `json:"title"`
	Exported    bool       `json:"exported"`
	Tags        []string   `json:"tags"`
	File        string     `json:"file"`
	Environment string     `json:"environment"`
	Parameters  Parameters `json:"parameters"`
}

type Parameters map[string]interface{}

func (p *Parameters) toLabels() model.LabelSet {
	labels := model.LabelSet{}

	for k, v := range *p {
		var labelValue string
		switch value := v.(type) {
		case string:
			labelValue = value
		case bool:
			labelValue = strconv.FormatBool(value)
		case []string:
			labelValue = separator + strings.Join(value, separator) + separator
		case []interface{}:
			if len(value) == 0 {
				continue
			}
			values := make([]string, len(value))
			for i, v := range value {
				switch value := v.(type) {
				case string:
					values[i] = value
				case bool:
					values[i] = strconv.FormatBool(value)
				case []string:
					values[i] = separator + strings.Join(value, separator) + separator
				}
			}
			labelValue = strings.Join(values, separator)
		case map[string]interface{}:
			subParameter := Parameters(value)
			prefix := strutil.SanitizeLabelName(k + "_")
			for subk, subv := range subParameter.toLabels() {
				labels[model.LabelName(prefix)+subk] = subv
			}
		default:
			continue
		}
		if labelValue == "" {
			continue
		}
		name := strutil.SanitizeLabelName(k)
		labels[model.LabelName(name)] = model.LabelValue(labelValue)
	}
	return labels
}
