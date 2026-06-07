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

package runtime

import "golang.org/x/sys/unix"

// Uname returns the uname of the host machine.
func Uname() string {
	buf := unix.Utsname{}
	err := unix.Uname(&buf)
	if err != nil {
		panic("unix.Uname failed: " + err.Error())
	}

	str := "(" + unix.ByteSliceToString(buf.Sysname[:])
	str += " " + unix.ByteSliceToString(buf.Release[:])
	str += " " + unix.ByteSliceToString(buf.Version[:])
	str += " " + unix.ByteSliceToString(buf.Machine[:])
	str += " " + unix.ByteSliceToString(buf.Nodename[:])
	str += " " + unix.ByteSliceToString(buf.Domainname[:]) + ")"
	return str
}
