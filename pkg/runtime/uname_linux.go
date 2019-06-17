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

package runtime

import (
	"syscall"
)

// Uname returns the uname of the host machine.
func Uname() string {
	buf := syscall.Utsname{}
	err := syscall.Uname(&buf)
	if err != nil {
		panic("syscall.Uname failed: " + err.Error())
	}

	str := "(" + charsToString(buf.Sysname[:])
	str += " " + charsToString(buf.Release[:])
	str += " " + charsToString(buf.Version[:])
	str += " " + charsToString(buf.Machine[:])
	str += " " + charsToString(buf.Nodename[:])
	str += " " + charsToString(buf.Domainname[:]) + ")"
	return str
}
