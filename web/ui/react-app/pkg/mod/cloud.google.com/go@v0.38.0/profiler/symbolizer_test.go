// Copyright 2017 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package profiler

import (
	"bytes"
	"testing"

	"cloud.google.com/go/internal/testutil"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/pprof/profile"
)

type fakeFunc struct {
	name   string
	file   string
	lineno int
}

func (f *fakeFunc) Name() string {
	return f.name
}
func (f *fakeFunc) FileLine(_ uintptr) (string, int) {
	return f.file, f.lineno
}

var cmpOpt = cmpopts.IgnoreUnexported(profile.Profile{}, profile.Function{},
	profile.Line{}, profile.Location{}, profile.Sample{}, profile.ValueType{})

// TestRuntimeFunctionTrimming tests if symbolize trims runtime functions as intended.
func TestRuntimeFunctionTrimming(t *testing.T) {
	fakeFuncMap := map[uintptr]*fakeFunc{
		0x10: {"runtime.goexit", "runtime.go", 10},
		0x20: {"runtime.other", "runtime.go", 20},
		0x30: {"foo", "foo.go", 30},
		0x40: {"bar", "bar.go", 40},
	}
	backupFuncForPC := funcForPC
	funcForPC = func(pc uintptr) function {
		return fakeFuncMap[pc]
	}
	defer func() {
		funcForPC = backupFuncForPC
	}()
	testLoc := []*profile.Location{
		{ID: 1, Address: 0x10},
		{ID: 2, Address: 0x20},
		{ID: 3, Address: 0x30},
		{ID: 4, Address: 0x40},
	}
	testProfile := &profile.Profile{
		Sample: []*profile.Sample{
			{Location: []*profile.Location{testLoc[0], testLoc[1], testLoc[3], testLoc[2]}},
			{Location: []*profile.Location{testLoc[1], testLoc[3], testLoc[2]}},
			{Location: []*profile.Location{testLoc[3], testLoc[2], testLoc[1]}},
			{Location: []*profile.Location{testLoc[3], testLoc[2], testLoc[0]}},
			{Location: []*profile.Location{testLoc[0], testLoc[1], testLoc[3], testLoc[0]}},
			{Location: []*profile.Location{testLoc[1], testLoc[0]}},
		},
		Location: testLoc,
	}
	testProfiles := make([]*profile.Profile, 2)
	testProfiles[0] = testProfile.Copy()
	testProfiles[1] = testProfile.Copy()
	// Test case for CPU profile.
	testProfiles[0].PeriodType = &profile.ValueType{Type: "cpu", Unit: "nanoseconds"}
	// Test case for heap profile.
	testProfiles[1].PeriodType = &profile.ValueType{Type: "space", Unit: "bytes"}
	wantFunc := []*profile.Function{
		{ID: 1, Name: "runtime.goexit", SystemName: "runtime.goexit", Filename: "runtime.go"},
		{ID: 2, Name: "runtime.other", SystemName: "runtime.other", Filename: "runtime.go"},
		{ID: 3, Name: "foo", SystemName: "foo", Filename: "foo.go"},
		{ID: 4, Name: "bar", SystemName: "bar", Filename: "bar.go"},
	}
	wantLoc := []*profile.Location{
		{ID: 1, Address: 0x10, Line: []profile.Line{{Function: wantFunc[0], Line: 10}}},
		{ID: 2, Address: 0x20, Line: []profile.Line{{Function: wantFunc[1], Line: 20}}},
		{ID: 3, Address: 0x30, Line: []profile.Line{{Function: wantFunc[2], Line: 30}}},
		{ID: 4, Address: 0x40, Line: []profile.Line{{Function: wantFunc[3], Line: 40}}},
	}
	wantProfiles := []*profile.Profile{
		{
			PeriodType: &profile.ValueType{Type: "cpu", Unit: "nanoseconds"},
			Sample: []*profile.Sample{
				{Location: []*profile.Location{wantLoc[1], wantLoc[3], wantLoc[2]}},
				{Location: []*profile.Location{wantLoc[1], wantLoc[3], wantLoc[2]}},
				{Location: []*profile.Location{wantLoc[3], wantLoc[2], wantLoc[1]}},
				{Location: []*profile.Location{wantLoc[3], wantLoc[2]}},
				{Location: []*profile.Location{wantLoc[1], wantLoc[3]}},
				{Location: []*profile.Location{wantLoc[1]}},
			},
			Location: wantLoc,
			Function: wantFunc,
		},
		{
			PeriodType: &profile.ValueType{Type: "space", Unit: "bytes"},
			Sample: []*profile.Sample{
				{Location: []*profile.Location{wantLoc[3], wantLoc[2]}},
				{Location: []*profile.Location{wantLoc[3], wantLoc[2]}},
				{Location: []*profile.Location{wantLoc[3], wantLoc[2], wantLoc[1]}},
				{Location: []*profile.Location{wantLoc[3], wantLoc[2]}},
				{Location: []*profile.Location{wantLoc[3]}},
				{Location: []*profile.Location{wantLoc[0]}},
			},
			Location: wantLoc,
			Function: wantFunc,
		},
	}
	for i := 0; i < 2; i++ {
		symbolize(testProfiles[i])
		if !testutil.Equal(testProfiles[i], wantProfiles[i], cmpOpt) {
			t.Errorf("incorrect trimming (testcase = %d): got {%v}, want {%v}", i, testProfiles[i], wantProfiles[i])
		}
	}
}

// TestParseAndSymbolize tests if parseAndSymbolize parses and symbolizes
// profiles as intended.
func TestParseAndSymbolize(t *testing.T) {
	fakeFuncMap := map[uintptr]*fakeFunc{
		0x10: {"foo", "foo.go", 10},
		0x20: {"bar", "bar.go", 20},
	}
	backupFuncForPC := funcForPC
	funcForPC = func(pc uintptr) function {
		return fakeFuncMap[pc]
	}
	defer func() {
		funcForPC = backupFuncForPC
	}()

	testLoc := []*profile.Location{
		{ID: 1, Address: 0x10},
		{ID: 2, Address: 0x20},
	}
	testProfile := &profile.Profile{
		SampleType: []*profile.ValueType{
			{Type: "cpu", Unit: "nanoseconds"},
		},
		PeriodType: &profile.ValueType{Type: "cpu", Unit: "nanoseconds"},
		Sample: []*profile.Sample{
			{Location: []*profile.Location{testLoc[0], testLoc[1]}, Value: []int64{1}},
			{Location: []*profile.Location{testLoc[1]}, Value: []int64{1}},
		},
		Location: testLoc,
	}
	testProfiles := make([]*profile.Profile, 2)
	testProfiles[0] = testProfile.Copy()
	testProfiles[1] = testProfile.Copy()

	wantFunc := []*profile.Function{
		{ID: 1, Name: "foo", SystemName: "foo", Filename: "foo.go"},
		{ID: 2, Name: "bar", SystemName: "bar", Filename: "bar.go"},
	}
	wantLoc := []*profile.Location{
		{ID: 1, Address: 0x10, Line: []profile.Line{{Function: wantFunc[0], Line: 10}}},
		{ID: 2, Address: 0x20, Line: []profile.Line{{Function: wantFunc[1], Line: 20}}},
	}
	wantProfile := &profile.Profile{
		SampleType: []*profile.ValueType{
			{Type: "cpu", Unit: "nanoseconds"},
		},
		PeriodType: &profile.ValueType{Type: "cpu", Unit: "nanoseconds"},
		Sample: []*profile.Sample{
			{Location: []*profile.Location{wantLoc[0], wantLoc[1]}, Value: []int64{1}},
			{Location: []*profile.Location{wantLoc[1]}, Value: []int64{1}},
		},
		Location: wantLoc,
		Function: wantFunc,
	}

	// Profile already symbolized.
	testProfiles[1].Location = []*profile.Location{
		{ID: 1, Address: 0x10, Line: []profile.Line{{Function: wantFunc[0], Line: 10}}},
		{ID: 2, Address: 0x20, Line: []profile.Line{{Function: wantFunc[1], Line: 20}}},
	}
	testProfiles[1].Function = []*profile.Function{
		{ID: 1, Name: "foo", SystemName: "foo", Filename: "foo.go"},
		{ID: 2, Name: "bar", SystemName: "bar", Filename: "bar.go"},
	}
	for i := 0; i < 2; i++ {
		var prof bytes.Buffer
		testProfiles[i].Write(&prof)

		parseAndSymbolize(&prof)
		gotProfile, err := profile.ParseData(prof.Bytes())
		if err != nil {
			t.Errorf("parsing symbolized profile (testcase = %d) got err: %v, want no error", i, err)
		}
		if !testutil.Equal(gotProfile, wantProfile, cmpOpt) {
			t.Errorf("incorrect symbolization (testcase = %d): got {%v}, want {%v}", i, gotProfile, wantProfile)
		}
	}
}

func TestIsSymbolizedGoVersion(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  bool
	}{
		{"go1.9beta2", true},
		{"go1.9", true},
		{"go1.9.1", true},
		{"go1.10", true},
		{"go1.10.1", true},
		{"go2.0", true},
		{"go3.1", true},
		{"go1.8", false},
		{"go1.8.1", false},
		{"go1.7", false},
		{"devel ", false},
	} {
		if got := isSymbolizedGoVersion(tc.input); got != tc.want {
			t.Errorf("isSymbolizedGoVersion(%v) got %v, want %v", tc.input, got, tc.want)
		}
	}
}
