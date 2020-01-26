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
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"

	"github.com/prometheus/procfs/internal/fs"
)

// Info contains identifying information for a block device such as a disk drive
type Info struct {
	MajorNumber uint32
	MinorNumber uint32
	DeviceName  string
}

// IOStats models the iostats data described in the kernel documentation
// https://www.kernel.org/doc/Documentation/iostats.txt,
// https://www.kernel.org/doc/Documentation/block/stat.txt,
// and https://www.kernel.org/doc/Documentation/ABI/testing/procfs-diskstats
type IOStats struct {
	// ReadIOs is the number of reads completed successfully.
	ReadIOs uint64
	// ReadMerges is the number of reads merged.  Reads and writes
	// which are adjacent to each other may be merged for efficiency.
	ReadMerges uint64
	// ReadSectors is the total number of sectors read successfully.
	ReadSectors uint64
	// ReadTicks is the total number of milliseconds spent by all reads.
	ReadTicks uint64
	// WriteIOs is the total number of writes completed successfully.
	WriteIOs uint64
	// WriteMerges is the number of reads merged.
	WriteMerges uint64
	// WriteSectors is the total number of sectors written successfully.
	WriteSectors uint64
	// WriteTicks is the total number of milliseconds spent by all writes.
	WriteTicks uint64
	// IOsInProgress is number of I/Os currently in progress.
	IOsInProgress uint64
	// IOsTotalTicks is the number of milliseconds spent doing I/Os.
	// This field increases so long as IosInProgress is nonzero.
	IOsTotalTicks uint64
	// WeightedIOTicks is the weighted number of milliseconds spent doing I/Os.
	// This can also be used to estimate average queue wait time for requests.
	WeightedIOTicks uint64
	// DiscardIOs is the total number of discards completed successfully.
	DiscardIOs uint64
	// DiscardMerges is the number of discards merged.
	DiscardMerges uint64
	// DiscardSectors is the total number of sectors discarded successfully.
	DiscardSectors uint64
	// DiscardTicks is the total number of milliseconds spent by all discards.
	DiscardTicks uint64
}

// Diskstats combines the device Info and IOStats
type Diskstats struct {
	Info
	IOStats
	// IoStatsCount contains the number of io stats read.  For kernel versions
	// 4.18+, there should be 18 fields read.  For earlier kernel versions this
	// will be 14 because the discard values are not available.
	IoStatsCount int
}

const (
	procDiskstatsPath   = "diskstats"
	procDiskstatsFormat = "%d %d %s %d %d %d %d %d %d %d %d %d %d %d %d %d %d %d"
	sysBlockPath        = "block"
	sysBlockStatFormat  = "%d %d %d %d %d %d %d %d %d %d %d %d %d %d %d"
)

// FS represents the pseudo-filesystems proc and sys, which provides an
// interface to kernel data structures.
type FS struct {
	proc *fs.FS
	sys  *fs.FS
}

// NewDefaultFS returns a new blockdevice fs using the default mountPoints for proc and sys.
// It will error if either of these mount points can't be read.
func NewDefaultFS() (FS, error) {
	return NewFS(fs.DefaultProcMountPoint, fs.DefaultSysMountPoint)
}

// NewFS returns a new blockdevice fs using the given mountPoints for proc and sys.
// It will error if either of these mount points can't be read.
func NewFS(procMountPoint string, sysMountPoint string) (FS, error) {
	if strings.TrimSpace(procMountPoint) == "" {
		procMountPoint = fs.DefaultProcMountPoint
	}
	procfs, err := fs.NewFS(procMountPoint)
	if err != nil {
		return FS{}, err
	}
	if strings.TrimSpace(sysMountPoint) == "" {
		sysMountPoint = fs.DefaultSysMountPoint
	}
	sysfs, err := fs.NewFS(sysMountPoint)
	if err != nil {
		return FS{}, err
	}
	return FS{&procfs, &sysfs}, nil
}

// ProcDiskstats reads the diskstats file and returns
// an array of Diskstats (one per line/device)
func (fs FS) ProcDiskstats() ([]Diskstats, error) {
	file, err := os.Open(fs.proc.Path(procDiskstatsPath))
	if err != nil {
		return nil, err
	}
	defer file.Close()

	diskstats := []Diskstats{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		d := &Diskstats{}
		d.IoStatsCount, err = fmt.Sscanf(scanner.Text(), procDiskstatsFormat,
			&d.MajorNumber,
			&d.MinorNumber,
			&d.DeviceName,
			&d.ReadIOs,
			&d.ReadMerges,
			&d.ReadSectors,
			&d.ReadTicks,
			&d.WriteIOs,
			&d.WriteMerges,
			&d.WriteSectors,
			&d.WriteTicks,
			&d.IOsInProgress,
			&d.IOsTotalTicks,
			&d.WeightedIOTicks,
			&d.DiscardIOs,
			&d.DiscardMerges,
			&d.DiscardSectors,
			&d.DiscardTicks,
		)
		// The io.EOF error can be safely ignored because it just means we read fewer than
		// the full 18 fields.
		if err != nil && err != io.EOF {
			return diskstats, err
		}
		if d.IoStatsCount == 14 || d.IoStatsCount == 18 {
			diskstats = append(diskstats, *d)
		}
	}
	return diskstats, scanner.Err()
}

// SysBlockDevices lists the device names from /sys/block/<dev>
func (fs FS) SysBlockDevices() ([]string, error) {
	deviceDirs, err := ioutil.ReadDir(fs.sys.Path(sysBlockPath))
	if err != nil {
		return nil, err
	}
	devices := []string{}
	for _, deviceDir := range deviceDirs {
		if deviceDir.IsDir() {
			devices = append(devices, deviceDir.Name())
		}
	}
	return devices, nil
}

// SysBlockDeviceStat returns stats for the block device read from /sys/block/<device>/stat.
// The number of stats read will be 15 if the discard stats are available (kernel 4.18+)
// and 11 if they are not available.
func (fs FS) SysBlockDeviceStat(device string) (IOStats, int, error) {
	stat := IOStats{}
	bytes, err := ioutil.ReadFile(fs.sys.Path(sysBlockPath, device, "stat"))
	if err != nil {
		return stat, 0, err
	}
	count, err := fmt.Sscanf(strings.TrimSpace(string(bytes)), sysBlockStatFormat,
		&stat.ReadIOs,
		&stat.ReadMerges,
		&stat.ReadSectors,
		&stat.ReadTicks,
		&stat.WriteIOs,
		&stat.WriteMerges,
		&stat.WriteSectors,
		&stat.WriteTicks,
		&stat.IOsInProgress,
		&stat.IOsTotalTicks,
		&stat.WeightedIOTicks,
		&stat.DiscardIOs,
		&stat.DiscardMerges,
		&stat.DiscardSectors,
		&stat.DiscardTicks,
	)
	// An io.EOF error is ignored because it just means we read fewer than the full 15 fields.
	if err == io.EOF {
		return stat, count, nil
	}
	return stat, count, err
}
