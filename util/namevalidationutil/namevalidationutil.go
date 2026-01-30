// Copyright The Prometheus Authors
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
	"errors"
	"fmt"

	"github.com/prometheus/common/model"
)

// CheckNameValidationScheme returns an error iff nameValidationScheme is unset.
func CheckNameValidationScheme(nameValidationScheme model.ValidationScheme) error {
	switch nameValidationScheme {
	case model.UTF8Validation, model.LegacyValidation:
	case model.UnsetValidation:
		return errors.New("unset nameValidationScheme")
	default:
		panic(fmt.Errorf("unhandled nameValidationScheme: %s", nameValidationScheme.String()))
	}
	return nil
}
