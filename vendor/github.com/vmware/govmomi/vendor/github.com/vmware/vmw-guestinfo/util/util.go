// Copyright 2016 VMware, Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package util

import (
	"reflect"
	"runtime"
	"testing"
)

// Test utilities.

func AssertEqual(t *testing.T, a interface{}, b interface{}) bool {
	if !reflect.DeepEqual(a, b) {
		Fail(t)
		return false
	}

	return true
}

func AssertNoError(t *testing.T, err error) bool {
	if err != nil {
		t.Logf("error :%s", err.Error())
		Fail(t)
		return false
	}

	return true
}

func AssertNotNil(t *testing.T, a interface{}) bool {
	val := reflect.ValueOf(a)
	if val.IsNil() {
		Fail(t)
		return false
	}

	return true
}

func Fail(t *testing.T) {
	_, file, line, _ := runtime.Caller(2)
	t.Logf("FAIL on %s:%d", file, line)
	t.Fail()
}
