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

package certutil

import (
	"testing"
)

// TestIsPEMFormat tests the PEM format detection.
func TestIsPEMFormat(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected bool
	}{
		{
			name: "valid PEM",
			data: []byte(`-----BEGIN CERTIFICATE-----
MIIBkTCB+wIJAKHHCgVZU6pfMA0GCSqGSIb3DQEBCwUAMA0xCzAJBgNVBAYTAlVT
MB4XDTE5MDEwMTAwMDAwMFoXDTIwMDEwMTAwMDAwMFowDTELMAkGA1UEBhMCVVMw
-----END CERTIFICATE-----`),
			expected: true,
		},
		{
			name:     "not PEM",
			data:     []byte("random binary data"),
			expected: false,
		},
		{
			name:     "empty",
			data:     []byte(""),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isPEMFormat(tt.data)
			if result != tt.expected {
				t.Errorf("isPEMFormat() = %v, want %v", result, tt.expected)
			}
		})
	}
}
