// Copyright 2016-2017 VMware, Inc. All Rights Reserved.
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

package bdoor

import "unsafe"

type UInt32 struct {
	High uint16
	Low  uint16
}

func (u *UInt32) Word() uint32 {
	return uint32(u.High)<<16 + uint32(u.Low)
}

func (u *UInt32) SetWord(w uint32) {
	u.High = uint16(w >> 16)
	u.Low = uint16(w)
}

func (u *UInt32) AsUInt32() *UInt32 {
	return u
}

func (u *UInt32) Value() uint32 {
	return u.Word()
}

func (u *UInt32) SetValue(val uint32) {
	u.SetWord(val)
}

func (u *UInt32) SetPointer(p unsafe.Pointer) {
	u.SetWord(uint32(uintptr(p)))
}

type UInt64 struct {
	High UInt32
	Low  UInt32
}

func (u *UInt64) Quad() uint64 {
	return uint64(u.High.Word())<<32 + uint64(u.Low.Word())
}

func (u *UInt64) SetQuad(w uint64) {
	u.High.SetWord(uint32(w >> 32))
	u.Low.SetWord(uint32(w))
}

func (u *UInt64) AsUInt32() *UInt32 {
	return &u.Low
}

func (u *UInt64) Value() uint64 {
	return u.Quad()
}

func (u *UInt64) SetValue(val uint64) {
	u.SetQuad(val)
}

func (u *UInt64) SetPointer(p unsafe.Pointer) {
	u.SetQuad(uint64(uintptr(p)))
}
