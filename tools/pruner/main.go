// Copyright 2013 Prometheus Team
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

// Pruner is responsible for cleaning all Prometheus disk databases, which
// minimally includes 1. applying pending commit logs, 2. compacting SSTables,
// 3. purging stale SSTables, and 4. removing old tombstones.
package main

import (
	"flag"
	"github.com/prometheus/prometheus/storage/metric/tiered"
	"time"

	"github.com/golang/glog"
)

var (
	storageRoot = flag.String("storage.root", "", "The path to the storage root for Prometheus.")
)

func main() {
	flag.Parse()

	if storageRoot == nil || *storageRoot == "" {
		glog.Fatal("Must provide a path...")
	}

	persistences, err := tiered.NewLevelDBPersistence(*storageRoot)
	if err != nil {
		glog.Fatal(err)
	}
	defer persistences.Close()

	start := time.Now()
	glog.Info("Starting compaction...")
	size, _ := persistences.Sizes()
	glog.Info("Original Size: ", size)
	persistences.Prune()
	glog.Info("Finished in ", time.Since(start))
	size, _ = persistences.Sizes()
	glog.Info("New Size: ", size)
}
