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
	dto "github.com/prometheus/prometheus/model/generated"
	"time"
)

// CurationRemark provides a representation of dto.CurationValue with associated
// business logic methods attached to it to enhance code readability.
type CurationRemark struct {
	LastCompletionTimestamp time.Time
}

// OlderThanLimit answers whether this CurationRemark is older than the provided
// cutOff time.
func (c CurationRemark) OlderThanLimit(cutOff time.Time) bool {
	return c.LastCompletionTimestamp.Before(cutOff)
}

// NewCurationRemarkFromDTO builds CurationRemark from the provided
// dto.CurationValue object.
func NewCurationRemarkFromDTO(d *dto.CurationValue) CurationRemark {
	return CurationRemark{
		LastCompletionTimestamp: time.Unix(*d.LastCompletionTimestamp, 0),
	}
}
