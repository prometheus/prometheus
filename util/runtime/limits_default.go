// Copyright 2017 The Prometheus Authors
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

//go:build !windows
// +build !windows

package runtime

import (
	"fmt"
	"syscall"
)

// syscall.RLIM_INFINITY is a constant and its default type is int.
// It needs to be converted to an int64 variable to be compared with uint64 values.
// See https://golang.org/ref/spec#Conversions
var unlimited int64 = syscall.RLIM_INFINITY

func limitToString(v uint64, unit string) string {
	if v == uint64(unlimited) {
		return "unlimited"
	}
	return fmt.Sprintf("%d%s", v, unit)
}

func getLimits(resource int, unit string) string {
	rlimit := syscall.Rlimit{}
	err := syscall.Getrlimit(resource, &rlimit)
	if err != nil {
		panic("syscall.Getrlimit failed: " + err.Error())
	}
	return fmt.Sprintf("(soft=%s, hard=%s)", limitToString(uint64(rlimit.Cur), unit), limitToString(uint64(rlimit.Max), unit))
}

// FdLimits returns the soft and hard limits for file descriptors.
func FdLimits() string {
	return getLimits(syscall.RLIMIT_NOFILE, "")
}
