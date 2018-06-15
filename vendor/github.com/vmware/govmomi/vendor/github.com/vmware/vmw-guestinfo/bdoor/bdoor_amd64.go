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

const (
	BackdoorMagic = uint64(0x564D5868)
)

type BackdoorProto struct {
	// typedef union {
	//   struct {
	//      DECLARE_REG_NAMED_STRUCT(ax);
	//      size_t size; /* Register bx. */
	//      DECLARE_REG_NAMED_STRUCT(cx);
	//      DECLARE_REG_NAMED_STRUCT(dx);
	//      DECLARE_REG_NAMED_STRUCT(si);
	//      DECLARE_REG_NAMED_STRUCT(di);
	//   } in;
	//   struct {
	//      DECLARE_REG_NAMED_STRUCT(ax);
	//      DECLARE_REG_NAMED_STRUCT(bx);
	//      DECLARE_REG_NAMED_STRUCT(cx);
	//      DECLARE_REG_NAMED_STRUCT(dx);
	//      DECLARE_REG_NAMED_STRUCT(si);
	//      DECLARE_REG_NAMED_STRUCT(di);
	//   } out;
	// } proto;

	AX, BX, CX, DX, SI, DI, BP UInt64
	size                       uint32
}

func bdoor_inout(ax, bx, cx, dx, si, di, bp uint64) (retax, retbx, retcx, retdx, retsi, retdi, retbp uint64)
func bdoor_hbout(ax, bx, cx, dx, si, di, bp uint64) (retax, retbx, retcx, retdx, retsi, retdi, retbp uint64)
func bdoor_hbin(ax, bx, cx, dx, si, di, bp uint64) (retax, retbx, retcx, retdx, retsi, retdi, retbp uint64)
func bdoor_inout_test(ax, bx, cx, dx, si, di, bp uint64) (retax, retbx, retcx, retdx, retsi, retdi, retbp uint64)
