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

//go:build !windows

package runtime

import (
	"fmt"
	"math"
	"syscall"
)

// syscall.RLIM_INFINITY is a constant.
// Its type is int on most architectures but there are exceptions such as loong64.
// Uniform it to uint according to the standard.
// https://pubs.opengroup.org/onlinepubs/9699919799/basedefs/sys_resource.h.html
var unlimited uint64 = syscall.RLIM_INFINITY & math.MaxUint64

func limitToString(v uint64, unit string) string {
	if v == unlimited {
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
	// rlimit.Cur and rlimit.Max are int64 on some platforms, such as dragonfly.
	// We need to cast them explicitly to uint64.
	return fmt.Sprintf("(soft=%s, hard=%s)", limitToString(uint64(rlimit.Cur), unit), limitToString(uint64(rlimit.Max), unit)) //nolint:unconvert
}

// FdLimits returns the soft and hard limits for file descriptors.
func FdLimits() string {
	return getLimits(syscall.RLIMIT_NOFILE, "")
}
