// Copyright 2019-current Go-dump Authors
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

package dump

import (
	"fmt"
	"os"
	"path"
	"runtime"
	"runtime/pprof"
)

// heapDumpDir is the directory where the heap dump will be written. By default empty (current working dir).
// Set to whatever is required.
var heapDumpDir = func() string {
	// ignore error
	dir, _ := os.Getwd()
	return dir
}()

// WriteHeapDump is a convenience method to write a pprof heap dump which can be
// examined with pprof tool. The file `{name}.pb.gz` is written to `heapDumpDir`.
//
// Call at the start and end of the function to see how much and where things
// were allocated within that function.
func WriteHeapDump(name string) {
	var fName = fmt.Sprintf("%s.pb.gz", name)
	fName = path.Join(heapDumpDir, fName)

	f, _ := os.Create(fName)
	defer f.Close()
	runtime.GC()
	err := pprof.WriteHeapProfile(f)
	if err != nil {
		fmt.Printf("ERROR WRITING HEAP DUMP TO %s: %v\n", fName, err)
		return
	}

	fmt.Printf("WRITTEN HEAP DUMP TO %s\n", fName)
}
