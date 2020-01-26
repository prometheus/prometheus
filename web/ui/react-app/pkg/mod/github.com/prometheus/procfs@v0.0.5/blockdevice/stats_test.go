// Copyright 2018 The Prometheus Authors
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

package blockdevice

import (
	"testing"
)

const (
	failMsgFormat  = "%v, expected %v, actual %v"
	procfsFixtures = "../fixtures/proc"
	sysfsFixtures  = "../fixtures/sys"
)

func TestDiskstats(t *testing.T) {
	blockdevice, err := NewFS(procfsFixtures, sysfsFixtures)
	if err != nil {
		t.Fatalf("failed to access blockdevice fs: %v", err)
	}
	diskstats, err := blockdevice.ProcDiskstats()
	if err != nil {
		t.Fatal(err)
	}
	expectedNumOfDevices := 49
	if len(diskstats) != expectedNumOfDevices {
		t.Errorf(failMsgFormat, "Incorrect number of devices", expectedNumOfDevices, len(diskstats))
	}
	if diskstats[0].DeviceName != "ram0" {
		t.Errorf(failMsgFormat, "Incorrect device name", "ram0", diskstats[0].DeviceName)
	}
	if diskstats[1].IoStatsCount != 14 {
		t.Errorf(failMsgFormat, "Incorrect number of stats read", 14, diskstats[0].IoStatsCount)
	}
	if diskstats[24].WriteIOs != 28444756 {
		t.Errorf(failMsgFormat, "Incorrect writes completed", 28444756, diskstats[24].WriteIOs)
	}
	if diskstats[48].DiscardTicks != 11130 {
		t.Errorf(failMsgFormat, "Incorrect discard time", 11130, diskstats[48].DiscardTicks)
	}
	if diskstats[48].IoStatsCount != 18 {
		t.Errorf(failMsgFormat, "Incorrect number of stats read", 18, diskstats[48].IoStatsCount)
	}
}

func TestBlockDevice(t *testing.T) {
	blockdevice, err := NewFS("../fixtures/proc", "../fixtures/sys")
	if err != nil {
		t.Fatalf("failed to access blockdevice fs: %v", err)
	}
	devices, err := blockdevice.SysBlockDevices()
	if err != nil {
		t.Fatal(err)
	}
	expectedNumOfDevices := 2
	if len(devices) != expectedNumOfDevices {
		t.Fatalf(failMsgFormat, "Incorrect number of devices", expectedNumOfDevices, len(devices))
	}
	if devices[0] != "dm-0" {
		t.Errorf(failMsgFormat, "Incorrect device name", "dm-0", devices[0])
	}
	device0stats, count, err := blockdevice.SysBlockDeviceStat(devices[0])
	if err != nil {
		t.Fatal(err)
	}
	if count != 11 {
		t.Errorf(failMsgFormat, "Incorrect number of stats read", 11, count)
	}
	if device0stats.ReadIOs != 6447303 {
		t.Errorf(failMsgFormat, "Incorrect read I/Os", 6447303, device0stats.ReadIOs)
	}
	if device0stats.WeightedIOTicks != 6088971 {
		t.Errorf(failMsgFormat, "Incorrect time in queue", 6088971, device0stats.WeightedIOTicks)
	}
	device1stats, count, err := blockdevice.SysBlockDeviceStat(devices[1])
	if count != 15 {
		t.Errorf(failMsgFormat, "Incorrect number of stats read", 15, count)
	}
	if err != nil {
		t.Fatal(err)
	}
	if device1stats.WriteSectors != 286915323 {
		t.Errorf(failMsgFormat, "Incorrect write merges", 286915323, device1stats.WriteSectors)
	}
	if device1stats.DiscardTicks != 12 {
		t.Errorf(failMsgFormat, "Incorrect discard ticks", 12, device1stats.DiscardTicks)
	}
}
