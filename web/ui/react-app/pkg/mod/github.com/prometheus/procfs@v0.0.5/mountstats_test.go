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

package procfs

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestMountStats(t *testing.T) {
	tests := []struct {
		name    string
		s       string
		mounts  []*Mount
		invalid bool
	}{
		{
			name: "no devices",
			s:    `hello`,
		},
		{
			name:    "device has too few fields",
			s:       `device foo`,
			invalid: true,
		},
		{
			name:    "device incorrect format",
			s:       `device rootfs BAD on / with fstype rootfs`,
			invalid: true,
		},
		{
			name:    "device incorrect format",
			s:       `device rootfs mounted BAD / with fstype rootfs`,
			invalid: true,
		},
		{
			name:    "device incorrect format",
			s:       `device rootfs mounted on / BAD fstype rootfs`,
			invalid: true,
		},
		{
			name:    "device incorrect format",
			s:       `device rootfs mounted on / with BAD rootfs`,
			invalid: true,
		},
		{
			name:    "device rootfs cannot have stats",
			s:       `device rootfs mounted on / with fstype rootfs stats`,
			invalid: true,
		},
		{
			name:    "NFSv4 device with too little info",
			s:       "device 192.168.1.1:/srv mounted on /mnt/nfs with fstype nfs4 statvers=1.1\nhello",
			invalid: true,
		},
		{
			name:    "NFSv4 device with bad bytes",
			s:       "device 192.168.1.1:/srv mounted on /mnt/nfs with fstype nfs4 statvers=1.1\nbytes: 0",
			invalid: true,
		},
		{
			name:    "NFSv4 device with bad events",
			s:       "device 192.168.1.1:/srv mounted on /mnt/nfs with fstype nfs4 statvers=1.1\nevents: 0",
			invalid: true,
		},
		{
			name:    "NFSv4 device with bad per-op stats",
			s:       "device 192.168.1.1:/srv mounted on /mnt/nfs with fstype nfs4 statvers=1.1\nper-op statistics\nFOO 0",
			invalid: true,
		},
		{
			name:    "NFSv4 device with bad transport stats",
			s:       "device 192.168.1.1:/srv mounted on /mnt/nfs with fstype nfs4 statvers=1.1\nxprt: tcp",
			invalid: true,
		},
		{
			name:    "NFSv4 device with bad transport version",
			s:       "device 192.168.1.1:/srv mounted on /mnt/nfs with fstype nfs4 statvers=foo\nxprt: tcp 0",
			invalid: true,
		},
		{
			name:    "NFSv4 device with bad transport stats version 1.0",
			s:       "device 192.168.1.1:/srv mounted on /mnt/nfs with fstype nfs4 statvers=1.0\nxprt: tcp 0 0 0 0 0 0 0 0 0 0 0 0 0",
			invalid: true,
		},
		{
			name:    "NFSv4 device with bad transport stats version 1.1",
			s:       "device 192.168.1.1:/srv mounted on /mnt/nfs with fstype nfs4 statvers=1.1\nxprt: tcp 0 0 0 0 0 0 0 0 0 0",
			invalid: true,
		},
		{
			name:    "NFSv3 device with bad transport protocol",
			s:       "device 192.168.1.1:/srv mounted on /mnt/nfs with fstype nfs4 statvers=1.1\nxprt: tcpx 0 0 0 0 0 0 0 0 0 0",
			invalid: true,
		},
		{
			name: "NFSv3 device using TCP with transport stats version 1.0 OK",
			s:    "device 192.168.1.1:/srv mounted on /mnt/nfs with fstype nfs statvers=1.0\nxprt: tcp 1 2 3 4 5 6 7 8 9 10",
			mounts: []*Mount{{
				Device: "192.168.1.1:/srv",
				Mount:  "/mnt/nfs",
				Type:   "nfs",
				Stats: &MountStatsNFS{
					StatVersion: "1.0",
					Transport: NFSTransportStats{
						Protocol:                 "tcp",
						Port:                     1,
						Bind:                     2,
						Connect:                  3,
						ConnectIdleTime:          4,
						IdleTimeSeconds:          5,
						Sends:                    6,
						Receives:                 7,
						BadTransactionIDs:        8,
						CumulativeActiveRequests: 9,
						CumulativeBacklog:        10,
						MaximumRPCSlotsUsed:      0, // these three are not
						CumulativeSendingQueue:   0, // present in statvers=1.0
						CumulativePendingQueue:   0, //
					},
				},
			}},
		},
		{
			name: "NFSv3 device using UDP with transport stats version 1.0 OK",
			s:    "device 192.168.1.1:/srv mounted on /mnt/nfs with fstype nfs statvers=1.0\nxprt: udp 1 2 3 4 5 6 7",
			mounts: []*Mount{{
				Device: "192.168.1.1:/srv",
				Mount:  "/mnt/nfs",
				Type:   "nfs",
				Stats: &MountStatsNFS{
					StatVersion: "1.0",
					Transport: NFSTransportStats{
						Protocol:                 "udp",
						Port:                     1,
						Bind:                     2,
						Connect:                  0,
						ConnectIdleTime:          0,
						IdleTimeSeconds:          0,
						Sends:                    3,
						Receives:                 4,
						BadTransactionIDs:        5,
						CumulativeActiveRequests: 6,
						CumulativeBacklog:        7,
						MaximumRPCSlotsUsed:      0, // these three are not
						CumulativeSendingQueue:   0, // present in statvers=1.0
						CumulativePendingQueue:   0, //
					},
				},
			}},
		},
		{
			name: "NFSv3 device using TCP with transport stats version 1.1 OK",
			s:    "device 192.168.1.1:/srv mounted on /mnt/nfs with fstype nfs statvers=1.1\nxprt: tcp 1 2 3 4 5 6 7 8 9 10 11 12 13",
			mounts: []*Mount{{
				Device: "192.168.1.1:/srv",
				Mount:  "/mnt/nfs",
				Type:   "nfs",
				Stats: &MountStatsNFS{
					StatVersion: "1.1",
					Transport: NFSTransportStats{
						Protocol:                 "tcp",
						Port:                     1,
						Bind:                     2,
						Connect:                  3,
						ConnectIdleTime:          4,
						IdleTimeSeconds:          5,
						Sends:                    6,
						Receives:                 7,
						BadTransactionIDs:        8,
						CumulativeActiveRequests: 9,
						CumulativeBacklog:        10,
						MaximumRPCSlotsUsed:      11,
						CumulativeSendingQueue:   12,
						CumulativePendingQueue:   13,
					},
				},
			}},
		},
		{
			name: "NFSv3 device using UDP with transport stats version 1.1 OK",
			s:    "device 192.168.1.1:/srv mounted on /mnt/nfs with fstype nfs statvers=1.1\nxprt: udp 1 2 3 4 5 6 7 8 9 10",
			mounts: []*Mount{{
				Device: "192.168.1.1:/srv",
				Mount:  "/mnt/nfs",
				Type:   "nfs",
				Stats: &MountStatsNFS{
					StatVersion: "1.1",
					Transport: NFSTransportStats{
						Protocol:                 "udp",
						Port:                     1,
						Bind:                     2,
						Connect:                  0, // these three are not
						ConnectIdleTime:          0, // present for UDP
						IdleTimeSeconds:          0, //
						Sends:                    3,
						Receives:                 4,
						BadTransactionIDs:        5,
						CumulativeActiveRequests: 6,
						CumulativeBacklog:        7,
						MaximumRPCSlotsUsed:      8,
						CumulativeSendingQueue:   9,
						CumulativePendingQueue:   10,
					},
				},
			}},
		},
		{
			name: "NFSv3 device with mountaddr OK",
			s:    "device 192.168.1.1:/srv mounted on /mnt/nfs with fstype nfs statvers=1.1\nopts: rw,vers=3,mountaddr=192.168.1.1,proto=udp\n",
			mounts: []*Mount{{
				Device: "192.168.1.1:/srv",
				Mount:  "/mnt/nfs",
				Type:   "nfs",
				Stats: &MountStatsNFS{
					StatVersion: "1.1",
					Opts:        map[string]string{"rw": "", "vers": "3", "mountaddr": "192.168.1.1", "proto": "udp"},
				},
			}},
		},
		{
			name: "device rootfs OK",
			s:    `device rootfs mounted on / with fstype rootfs`,
			mounts: []*Mount{{
				Device: "rootfs",
				Mount:  "/",
				Type:   "rootfs",
			}},
		},
		{
			name: "NFSv3 device with minimal stats OK",
			s:    `device 192.168.1.1:/srv mounted on /mnt/nfs with fstype nfs statvers=1.1`,
			mounts: []*Mount{{
				Device: "192.168.1.1:/srv",
				Mount:  "/mnt/nfs",
				Type:   "nfs",
				Stats: &MountStatsNFS{
					StatVersion: "1.1",
				},
			}},
		},
		{
			name: "fixtures/proc OK",
			mounts: []*Mount{
				{
					Device: "rootfs",
					Mount:  "/",
					Type:   "rootfs",
				},
				{
					Device: "sysfs",
					Mount:  "/sys",
					Type:   "sysfs",
				},
				{
					Device: "proc",
					Mount:  "/proc",
					Type:   "proc",
				},
				{
					Device: "/dev/sda1",
					Mount:  "/",
					Type:   "ext4",
				},
				{
					Device: "192.168.1.1:/srv/test",
					Mount:  "/mnt/nfs/test",
					Type:   "nfs4",
					Stats: &MountStatsNFS{
						StatVersion: "1.1",
						Opts: map[string]string{"rw": "", "vers": "4.0",
							"rsize": "1048576", "wsize": "1048576", "namlen": "255", "acregmin": "3",
							"acregmax": "60", "acdirmin": "30", "acdirmax": "60", "hard": "",
							"proto": "tcp", "port": "0", "timeo": "600", "retrans": "2",
							"sec": "sys", "mountaddr": "192.168.1.1", "clientaddr": "192.168.1.5",
							"local_lock": "none",
						},
						Age: 13968 * time.Second,
						Bytes: NFSBytesStats{
							Read:      1207640230,
							ReadTotal: 1210214218,
							ReadPages: 295483,
						},
						Events: NFSEventsStats{
							InodeRevalidate: 52,
							DnodeRevalidate: 226,
							VFSOpen:         1,
							VFSLookup:       13,
							VFSAccess:       398,
							VFSReadPages:    331,
							VFSWritePages:   47,
							VFSFlush:        77,
							VFSFileRelease:  77,
						},
						Operations: []NFSOperationStats{
							{
								Operation: "NULL",
							},
							{
								Operation:                           "READ",
								Requests:                            1298,
								Transmissions:                       1298,
								BytesSent:                           207680,
								BytesReceived:                       1210292152,
								CumulativeQueueMilliseconds:         6,
								CumulativeTotalResponseMilliseconds: 79386,
								CumulativeTotalRequestMilliseconds:  79407,
							},
							{
								Operation: "WRITE",
							},
							{
								Operation:                           "ACCESS",
								Requests:                            2927395007,
								Transmissions:                       2927394995,
								BytesSent:                           526931094212,
								BytesReceived:                       362996810236,
								CumulativeQueueMilliseconds:         18446743919241604546,
								CumulativeTotalResponseMilliseconds: 1667369447,
								CumulativeTotalRequestMilliseconds:  1953587717,
							},
						},
						Transport: NFSTransportStats{
							Protocol:                 "tcp",
							Port:                     832,
							Connect:                  1,
							IdleTimeSeconds:          11,
							Sends:                    6428,
							Receives:                 6428,
							CumulativeActiveRequests: 12154,
							MaximumRPCSlotsUsed:      24,
							CumulativeSendingQueue:   26,
							CumulativePendingQueue:   5726,
						},
					},
				},
			},
		},
	}

	for i, tt := range tests {
		t.Logf("[%02d] test %q", i, tt.name)

		var mounts []*Mount
		var err error

		if tt.s != "" {
			mounts, err = parseMountStats(strings.NewReader(tt.s))
		} else {
			proc, e := getProcFixtures(t).Proc(26231)
			if e != nil {
				t.Fatalf("failed to create proc: %v", err)
			}

			mounts, err = proc.MountStats()
		}

		if tt.invalid && err == nil {
			t.Error("expected an error, but none occurred")
		}
		if !tt.invalid && err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if want, have := tt.mounts, mounts; !reflect.DeepEqual(want, have) {
			t.Errorf("mounts:\nwant:\n%v\nhave:\n%v", mountsStr(want), mountsStr(have))
		}
	}
}

func mountsStr(mounts []*Mount) string {
	var out string
	for i, m := range mounts {
		out += fmt.Sprintf("[%d] %q on %q (%q)", i, m.Device, m.Mount, m.Type)

		stats, ok := m.Stats.(*MountStatsNFS)
		if !ok {
			out += "\n"
			continue
		}

		out += fmt.Sprintf("\n\t- opts: %s", stats.Opts)
		out += fmt.Sprintf("\n\t- v%s, age: %s", stats.StatVersion, stats.Age)
		out += fmt.Sprintf("\n\t- bytes: %v", stats.Bytes)
		out += fmt.Sprintf("\n\t- events: %v", stats.Events)
		out += fmt.Sprintf("\n\t- transport: %v", stats.Transport)
		out += fmt.Sprintf("\n\t- per-operation stats:")

		for _, o := range stats.Operations {
			out += fmt.Sprintf("\n\t\t- %v", o)
		}

		out += "\n"
	}

	return out
}
