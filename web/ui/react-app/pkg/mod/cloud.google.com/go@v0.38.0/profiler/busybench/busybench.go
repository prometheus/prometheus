// Copyright 2017 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Busybench is a tool that runs a benchmark with the profiler enabled.
package main

import (
	"bytes"
	"compress/gzip"
	"flag"
	"log"
	"math/rand"
	"runtime"
	"sync"
	"time"

	"cloud.google.com/go/profiler"
)

var (
	service        = flag.String("service", "", "service name")
	mutexProfiling = flag.Bool("mutex_profiling", false, "enable mutex profiling")
	duration       = flag.Int("duration", 150, "duration of the benchmark in seconds")
	apiAddr        = flag.String("api_address", "", "API address of the profiler (e.g. 'cloudprofiler.googleapis.com:443')")
	projectID      = flag.String("project_id", "", "cloud project ID")
)

// busywork continuously generates 1MiB of random data and compresses it
// throwing away the result.
func busywork(mu *sync.Mutex) {
	ticker := time.NewTicker(time.Duration(*duration) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			return
		default:
			mu.Lock()
			busyworkOnce()
			mu.Unlock()
		}
	}
}

func busyworkOnce() {
	data := make([]byte, 1024*1024)
	rand.Read(data)

	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	if _, err := gz.Write(data); err != nil {
		log.Printf("Failed to write to gzip stream: %v", err)
		return
	}
	// Sleep to increase contention.
	time.Sleep(time.Millisecond)
	if err := gz.Flush(); err != nil {
		log.Printf("Failed to flush to gzip stream: %v", err)
		return
	}
	// Sleep to increase contention.
	time.Sleep(time.Millisecond)
	if err := gz.Close(); err != nil {
		log.Printf("Failed to close gzip stream: %v", err)
	}
	// Throw away the result.
}

func main() {
	flag.Parse()
	defer log.Printf("busybench finished profiling.")

	if *service == "" {
		log.Print("Service name must be configured using --service flag.")
		return
	}
	if err := profiler.Start(profiler.Config{Service: *service,
		MutexProfiling: *mutexProfiling,
		DebugLogging:   true,
		APIAddr:        *apiAddr,
		ProjectID:      *projectID}); err != nil {
		log.Printf("Failed to start the profiler: %v", err)
		return
	}
	mu := new(sync.Mutex)
	var wg sync.WaitGroup
	wg.Add(10)
	// Increse number of concurrent threads to increase contention.
	// Save current value.
	np := runtime.GOMAXPROCS(10)
	for i := 0; i < 10; i++ {
		go func() {
			defer wg.Done()
			busywork(mu)
		}()
	}
	wg.Wait()
	// Reset concurrent thread count.
	runtime.GOMAXPROCS(np)
}
