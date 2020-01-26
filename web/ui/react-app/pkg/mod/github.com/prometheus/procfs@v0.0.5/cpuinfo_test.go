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

import "testing"

func TestCPUInfo(t *testing.T) {
	cpuinfo, err := getProcFixtures(t).CPUInfo()
	if err != nil {
		t.Fatal(err)
	}

	if cpuinfo == nil {
		t.Fatal("cpuinfo is nil")
	}

	if want, have := 8, len(cpuinfo); want != have {
		t.Errorf("want number of processors %v, have %v", want, have)
	}

	if want, have := uint(7), cpuinfo[7].Processor; want != have {
		t.Errorf("want processor %v, have %v", want, have)
	}
	if want, have := "GenuineIntel", cpuinfo[0].VendorID; want != have {
		t.Errorf("want vendor %v, have %v", want, have)
	}
	if want, have := "6", cpuinfo[1].CPUFamily; want != have {
		t.Errorf("want family %v, have %v", want, have)
	}
	if want, have := "142", cpuinfo[2].Model; want != have {
		t.Errorf("want model %v, have %v", want, have)
	}
	if want, have := "Intel(R) Core(TM) i7-8650U CPU @ 1.90GHz", cpuinfo[3].ModelName; want != have {
		t.Errorf("want model %v, have %v", want, have)
	}
	if want, have := uint(8), cpuinfo[4].Siblings; want != have {
		t.Errorf("want siblings %v, have %v", want, have)
	}
	if want, have := "1", cpuinfo[5].CoreID; want != have {
		t.Errorf("want core id %v, have %v", want, have)
	}
	if want, have := uint(4), cpuinfo[6].CPUCores; want != have {
		t.Errorf("want cpu cores %v, have %v", want, have)
	}
	if want, have := "vme", cpuinfo[7].Flags[1]; want != have {
		t.Errorf("want flag %v, have %v", want, have)
	}

}
