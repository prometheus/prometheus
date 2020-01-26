/*
 *
 * Copyright 2017 gRPC authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

/*
To format the benchmark result:
  go run benchmark/benchresult/main.go resultfile

To see the performance change based on a old result:
  go run benchmark/benchresult/main.go resultfile_old resultfile
It will print the comparison result of intersection benchmarks between two files.

*/
package main

import (
	"encoding/gob"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"google.golang.org/grpc/benchmark/stats"
)

func createMap(fileName string) map[string]stats.BenchResults {
	f, err := os.Open(fileName)
	if err != nil {
		log.Fatalf("Read file %s error: %s\n", fileName, err)
	}
	defer f.Close()
	var data []stats.BenchResults
	decoder := gob.NewDecoder(f)
	if err = decoder.Decode(&data); err != nil {
		log.Fatalf("Decode file %s error: %s\n", fileName, err)
	}
	m := make(map[string]stats.BenchResults)
	for _, d := range data {
		m[d.RunMode+"-"+d.Features.String()] = d
	}
	return m
}

func intChange(title string, val1, val2 uint64) string {
	return fmt.Sprintf("%20s %12d %12d %8.2f%%\n", title, val1, val2, float64(int64(val2)-int64(val1))*100/float64(val1))
}

func floatChange(title string, val1, val2 float64) string {
	return fmt.Sprintf("%20s %12.2f %12.2f %8.2f%%\n", title, val1, val2, float64(int64(val2)-int64(val1))*100/float64(val1))
}
func timeChange(title string, val1, val2 time.Duration) string {
	return fmt.Sprintf("%20s %12s %12s %8.2f%%\n", title, val1.String(),
		val2.String(), float64(val2-val1)*100/float64(val1))
}

func compareTwoMap(m1, m2 map[string]stats.BenchResults) {
	for k2, v2 := range m2 {
		if v1, ok := m1[k2]; ok {
			changes := k2 + "\n"
			changes += fmt.Sprintf("%20s %12s %12s %8s\n", "Title", "Before", "After", "Percentage")
			changes += intChange("TotalOps", v1.Data.TotalOps, v2.Data.TotalOps)
			changes += intChange("SendOps", v1.Data.SendOps, v2.Data.SendOps)
			changes += intChange("RecvOps", v1.Data.RecvOps, v2.Data.RecvOps)
			changes += intChange("Bytes/op", v1.Data.AllocedBytes, v2.Data.AllocedBytes)
			changes += intChange("Allocs/op", v1.Data.Allocs, v2.Data.Allocs)
			changes += floatChange("ReqT/op", v1.Data.ReqT, v2.Data.ReqT)
			changes += floatChange("RespT/op", v1.Data.RespT, v2.Data.RespT)
			changes += timeChange("50th-Lat", v1.Data.Fiftieth, v2.Data.Fiftieth)
			changes += timeChange("90th-Lat", v1.Data.Ninetieth, v2.Data.Ninetieth)
			changes += timeChange("99th-Lat", v1.Data.NinetyNinth, v2.Data.NinetyNinth)
			changes += timeChange("Avg-Lat", v1.Data.Average, v2.Data.Average)
			fmt.Printf("%s\n", changes)
		}
	}
}

func compareBenchmark(file1, file2 string) {
	compareTwoMap(createMap(file1), createMap(file2))
}

func printline(benchName, total, send, recv, allocB, allocN, reqT, respT, ltc50, ltc90, l99, lAvg interface{}) {
	fmt.Printf("%-80v%12v%12v%12v%12v%12v%18v%18v%12v%12v%12v%12v\n",
		benchName, total, send, recv, allocB, allocN, reqT, respT, ltc50, ltc90, l99, lAvg)
}

func formatBenchmark(fileName string) {
	f, err := os.Open(fileName)
	if err != nil {
		log.Fatalf("Read file %s error: %s\n", fileName, err)
	}
	defer f.Close()
	var results []stats.BenchResults
	decoder := gob.NewDecoder(f)
	if err = decoder.Decode(&results); err != nil {
		log.Fatalf("Decode file %s error: %s\n", fileName, err)
	}
	if len(results) == 0 {
		log.Fatalf("No benchmark results in file %s\n", fileName)
	}

	fmt.Println("\nShared features:\n" + strings.Repeat("-", 20))
	fmt.Print(results[0].Features.SharedFeatures(results[0].SharedFeatures))
	fmt.Println(strings.Repeat("-", 35))

	wantFeatures := results[0].SharedFeatures
	for i := 0; i < len(results[0].SharedFeatures); i++ {
		wantFeatures[i] = !wantFeatures[i]
	}

	printline("Name", "TotalOps", "SendOps", "RecvOps", "Alloc (B)", "Alloc (#)",
		"RequestT", "ResponseT", "L-50", "L-90", "L-99", "L-Avg")
	for _, r := range results {
		d := r.Data
		printline(r.RunMode+r.Features.PrintableName(wantFeatures), d.TotalOps, d.SendOps, d.RecvOps,
			d.AllocedBytes, d.Allocs, d.ReqT, d.RespT, d.Fiftieth, d.Ninetieth, d.NinetyNinth, d.Average)
	}
}

func main() {
	if len(os.Args) == 2 {
		formatBenchmark(os.Args[1])
	} else {
		compareBenchmark(os.Args[1], os.Args[2])
	}
}
