// Copyright 2025 The Prometheus Authors
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

package namevalidationutil

import (
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
)

func TestCheckNameValidationScheme(t *testing.T) {
	require.NoError(t, CheckNameValidationScheme(model.UTF8Validation))
	require.NoError(t, CheckNameValidationScheme(model.LegacyValidation))
	require.EqualError(t, CheckNameValidationScheme(model.UnsetValidation), "unset nameValidationScheme")
	require.PanicsWithError(t, "unhandled ValidationScheme: 20", func() {
		CheckNameValidationScheme(20)
	})
}
