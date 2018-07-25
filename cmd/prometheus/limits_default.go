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

// +build !windows

package main

import (
	"fmt"
	"log"
	"syscall"
)

var unlimited int64 = syscall.RLIM_INFINITY

func limitToString(v uint64) string {
	if v == uint64(unlimited) {
		return "unlimited"
	}
	return fmt.Sprintf("%d", v)
}

func getLimits(resource int) string {
	rlimit := syscall.Rlimit{}
	err := syscall.Getrlimit(resource, &rlimit)
	if err != nil {
		log.Fatal("Error!")
	}
	return fmt.Sprintf("(soft=%s, hard=%s)", limitToString(rlimit.Cur), limitToString(rlimit.Max))
}

// FdLimits returns the soft and hard limits for file descriptors.
func FdLimits() string {
	return getLimits(syscall.RLIMIT_NOFILE)
}

// VmLimits returns the soft and hard limits for virtual memory.
func VmLimits() string {
	return getLimits(syscall.RLIMIT_AS)
}
