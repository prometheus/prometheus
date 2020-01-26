// +build go1.9

// Copyright 2018 Microsoft Corporation
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package model_test

import (
	"path/filepath"
	"testing"

	"github.com/Azure/azure-sdk-for-go/tools/profileBuilder/model"
	"github.com/marstr/collection"
)

func Test_VersionLE(t *testing.T) {
	const dateWithAlpha, dateWithBeta = "2016-02-01-alpha", "2016-02-01-beta"
	const semVer1dot2, semVer1dot3 = "2018-03-03-1.2", "2018-03-03-1.3"
	const dateAlone = "2016-12-07"

	testCases := []struct {
		left  string
		right string
		want  bool
	}{
		{dateWithAlpha, dateWithAlpha, true},
		{dateAlone, dateAlone, true},
		{"2017-12-01", "2018-03-04", true},
		{"2018-03-04", "2017-12-01", false},
		{semVer1dot2, semVer1dot3, true},
		{semVer1dot3, semVer1dot2, false},
		{semVer1dot2, semVer1dot2, true},
		{dateWithAlpha, dateWithBeta, true},
		{dateWithBeta, dateWithAlpha, false},
		{"2016-04-03-preview", "2016-04-03", true},
		{"2016-04-03", "2016-04-03-preview", false},
		{"1.0.0", "5.6", true},
		{"5.6", "1.0.0", false},
		{"5.6", "6.0", true},
		{"6.0", "5.6", false},
	}

	for _, tc := range testCases {
		t.Run("", func(t *testing.T) {
			if got, err := model.VersionLE(tc.left, tc.right); err != nil {
				t.Error(err)
			} else if got != tc.want {
				t.Logf("\n Left: %s\nRight: %s", tc.left, tc.right)
				t.Logf("\n got: %v\nwant: %v", got, tc.want)
				t.Fail()
			}
		})
	}
}

func BenchmarkLatestStrategy_Enumerate(b *testing.B) {
	subject := model.LatestStrategy{
		Root: filepath.Join("..", "..", "service"),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.Logf("Enumerated %d packages", collection.CountAll(subject))
	}
}
