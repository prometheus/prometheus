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
	BackdoorPort       = uint16(0x5658)
	BackdoorHighBWPort = uint16(0x5659)

	CommandGetVersion = uint32(10)

	CommandMessage       = uint16(0x1e)
	CommandHighBWMessage = uint16(0)
	CommandFlagCookie    = uint32(0x80000000)
)

func (p *BackdoorProto) InOut() *BackdoorProto {
	p.DX.AsUInt32().Low = BackdoorPort
	p.AX.SetValue(BackdoorMagic)

	retax, retbx, retcx, retdx, retsi, retdi, retbp := bdoor_inout(
		p.AX.Value(),
		p.BX.Value(),
		p.CX.Value(),
		p.DX.Value(),
		p.SI.Value(),
		p.DI.Value(),
		p.BP.Value(),
	)

	ret := &BackdoorProto{}
	ret.AX.SetValue(retax)
	ret.BX.SetValue(retbx)
	ret.CX.SetValue(retcx)
	ret.DX.SetValue(retdx)
	ret.SI.SetValue(retsi)
	ret.DI.SetValue(retdi)
	ret.BP.SetValue(retbp)

	return ret
}

func (p *BackdoorProto) HighBandwidthOut() *BackdoorProto {
	p.DX.AsUInt32().Low = BackdoorHighBWPort
	p.AX.SetValue(BackdoorMagic)

	retax, retbx, retcx, retdx, retsi, retdi, retbp := bdoor_hbout(
		p.AX.Value(),
		p.BX.Value(),
		p.CX.Value(),
		p.DX.Value(),
		p.SI.Value(),
		p.DI.Value(),
		p.BP.Value(),
	)

	ret := &BackdoorProto{}
	ret.AX.SetValue(retax)
	ret.BX.SetValue(retbx)
	ret.CX.SetValue(retcx)
	ret.DX.SetValue(retdx)
	ret.SI.SetValue(retsi)
	ret.DI.SetValue(retdi)
	ret.BP.SetValue(retbp)

	return ret
}

func (p *BackdoorProto) HighBandwidthIn() *BackdoorProto {
	p.DX.AsUInt32().Low = BackdoorHighBWPort
	p.AX.SetValue(BackdoorMagic)

	retax, retbx, retcx, retdx, retsi, retdi, retbp := bdoor_hbin(
		p.AX.Value(),
		p.BX.Value(),
		p.CX.Value(),
		p.DX.Value(),
		p.SI.Value(),
		p.DI.Value(),
		p.BP.Value(),
	)

	ret := &BackdoorProto{}
	ret.AX.SetValue(retax)
	ret.BX.SetValue(retbx)
	ret.CX.SetValue(retcx)
	ret.DX.SetValue(retdx)
	ret.SI.SetValue(retsi)
	ret.DI.SetValue(retdi)
	ret.BP.SetValue(retbp)

	return ret
}
