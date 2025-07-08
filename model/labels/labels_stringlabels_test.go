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

//go:build !slicelabels && !dedupelabels

package labels

var expectedSizeOfLabels = []uint64{ // Values must line up with testCaseLabels.
	12,
	0,
	37,
	266,
	270,
	309,
}

var expectedByteSize = []uint64{ // Values must line up with testCaseLabels.
	12,
	0,
	37,
	266,
	270,
	309,
}
