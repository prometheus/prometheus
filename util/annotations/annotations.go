// Copyright 2023 The Prometheus Authors
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

package annotations

// Annotations is a general wrapper for text that is shown in the
// Prometheus/Grafana UI along with the query results.
// Currently there are only 2 types, warnings and info.
// For now, info are visually identical with warnings as we have not updated
// the API spec or the frontend to show a different kind of warning. But we
// make the distinction here to prepare for adding them in future.
type Annotations struct {
	Warnings Warnings
	Info     Info
}

func (a *Annotations) AddWarning(err error) {
	a.Warnings = append(a.Warnings, err)
}

func CreateAnnotationsWithWarning(err error) Annotations {
	a := Annotations{}
	a.AddWarning(err)
	return a
}

func (a *Annotations) AddInfo(txt string) {
	a.Info = append(a.Info, txt)
}

func (a *Annotations) Merge(aa Annotations) {
	a.Warnings = append(a.Warnings, aa.Warnings...)
	a.Info = append(a.Info, aa.Info...)
}
