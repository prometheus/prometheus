// Copyright 2019 The Prometheus Authors
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
	"reflect"
	"testing"
)

func TestMountInfo(t *testing.T) {
	tests := []struct {
		name    string
		s       string
		mount   *MountInfo
		invalid bool
	}{
		{
			name:    "Regular sysfs mounted at /sys",
			s:       "16 21 0:16 / /sys rw,nosuid,nodev,noexec,relatime shared:7 - sysfs sysfs rw",
			invalid: false,
			mount: &MountInfo{
				MountId:        16,
				ParentId:       21,
				MajorMinorVer:  "0:16",
				Root:           "/",
				MountPoint:     "/sys",
				Options:        map[string]string{"rw": "", "nosuid": "", "nodev": "", "noexec": "", "relatime": ""},
				OptionalFields: map[string]string{"shared": "7"},
				FSType:         "sysfs",
				Source:         "sysfs",
				SuperOptions:   map[string]string{"rw": ""},
			},
		},
		{
			name:    "Not enough information",
			s:       "hello",
			invalid: true,
		},
		{
			name: "Tmpfs mounted at /run",
			s:    "225 20 0:39 / /run/user/112 rw,nosuid,nodev,relatime shared:177 - tmpfs tmpfs rw,size=405096k,mode=700,uid=112,gid=116",
			mount: &MountInfo{
				MountId:        225,
				ParentId:       20,
				MajorMinorVer:  "0:39",
				Root:           "/",
				MountPoint:     "/run/user/112",
				Options:        map[string]string{"rw": "", "nosuid": "", "nodev": "", "relatime": ""},
				OptionalFields: map[string]string{"shared": "177"},
				FSType:         "tmpfs",
				Source:         "tmpfs",
				SuperOptions:   map[string]string{"rw": "", "size": "405096k", "mode": "700", "uid": "112", "gid": "116"},
			},
			invalid: false,
		},
		{
			name: "Tmpfs mounted at /run, but no optional values",
			s:    "225 20 0:39 / /run/user/112 rw,nosuid,nodev,relatime  - tmpfs tmpfs rw,size=405096k,mode=700,uid=112,gid=116",
			mount: &MountInfo{
				MountId:        225,
				ParentId:       20,
				MajorMinorVer:  "0:39",
				Root:           "/",
				MountPoint:     "/run/user/112",
				Options:        map[string]string{"rw": "", "nosuid": "", "nodev": "", "relatime": ""},
				OptionalFields: nil,
				FSType:         "tmpfs",
				Source:         "tmpfs",
				SuperOptions:   map[string]string{"rw": "", "size": "405096k", "mode": "700", "uid": "112", "gid": "116"},
			},
			invalid: false,
		},
		{
			name: "Tmpfs mounted at /run, with multiple optional values",
			s:    "225 20 0:39 / /run/user/112 rw,nosuid,nodev,relatime shared:177 master:8 - tmpfs tmpfs rw,size=405096k,mode=700,uid=112,gid=116",
			mount: &MountInfo{
				MountId:        225,
				ParentId:       20,
				MajorMinorVer:  "0:39",
				Root:           "/",
				MountPoint:     "/run/user/112",
				Options:        map[string]string{"rw": "", "nosuid": "", "nodev": "", "relatime": ""},
				OptionalFields: map[string]string{"shared": "177", "master": "8"},
				FSType:         "tmpfs",
				Source:         "tmpfs",
				SuperOptions:   map[string]string{"rw": "", "size": "405096k", "mode": "700", "uid": "112", "gid": "116"},
			},
			invalid: false,
		},
		{
			name: "Tmpfs mounted at /run, with a mixture of valid and invalid optional values",
			s:    "225 20 0:39 / /run/user/112 rw,nosuid,nodev,relatime shared:177 master:8 foo:bar - tmpfs tmpfs rw,size=405096k,mode=700,uid=112,gid=116",
			mount: &MountInfo{
				MountId:        225,
				ParentId:       20,
				MajorMinorVer:  "0:39",
				Root:           "/",
				MountPoint:     "/run/user/112",
				Options:        map[string]string{"rw": "", "nosuid": "", "nodev": "", "relatime": ""},
				OptionalFields: map[string]string{"shared": "177", "master": "8"},
				FSType:         "tmpfs",
				Source:         "tmpfs",
				SuperOptions:   map[string]string{"rw": "", "size": "405096k", "mode": "700", "uid": "112", "gid": "116"},
			},
			invalid: false,
		},
	}

	for i, test := range tests {
		t.Logf("[%02d] test %q", i, test.name)

		mount, err := parseMountInfoString(test.s)

		if test.invalid && err == nil {
			t.Error("expected an error, but none occurred")
		}
		if !test.invalid && err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if want, have := test.mount, mount; !reflect.DeepEqual(want, have) {
			t.Errorf("mounts:\nwant:\n%+v\nhave:\n%+v", want, have)
		}
	}
}
