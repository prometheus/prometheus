// Copyright 2024 The Prometheus Authors
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
// Provenance-includes-location: https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/95e8f8fdc2a9dc87230406c9a3cf02be4fd68bea/pkg/translator/prometheus/normalize_label.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: Copyright The OpenTelemetry Authors.

package prometheus

import (
	"strings"
	"unicode"

	"go.opentelemetry.io/collector/featuregate"
)

var dropSanitizationGate = featuregate.GlobalRegistry().MustRegister(
	"pkg.translator.prometheus.PermissiveLabelSanitization",
	featuregate.StageAlpha,
	featuregate.WithRegisterDescription("Controls whether to change labels starting with '_' to 'key_'."),
	featuregate.WithRegisterReferenceURL("https://github.com/open-telemetry/opentelemetry-collector-contrib/issues/8950"),
)

// Normalizes the specified label to follow Prometheus label names standard
//
// See rules at https://prometheus.io/docs/concepts/data_model/#metric-names-and-labels
//
// Labels that start with non-letter rune will be prefixed with "key_"
//
// Exception is made for double-underscores which are allowed
func NormalizeLabel(label string) string {

	// Trivial case
	if len(label) == 0 {
		return label
	}

	// Replace all non-alphanumeric runes with underscores
	label = strings.Map(sanitizeRune, label)

	// If label starts with a number, prepend with "key_"
	if unicode.IsDigit(rune(label[0])) {
		label = "key_" + label
	} else if strings.HasPrefix(label, "_") && !strings.HasPrefix(label, "__") && !dropSanitizationGate.IsEnabled() {
		label = "key" + label
	}

	return label
}

// Return '_' for anything non-alphanumeric
func sanitizeRune(r rune) rune {
	if unicode.IsLetter(r) || unicode.IsDigit(r) {
		return r
	}
	return '_'
}
