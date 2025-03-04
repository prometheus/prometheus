// Copyright 2025 The Prometheus Authors

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

package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
)

func main() {
	numSeries := flag.Int("series", 10, "Number of series to generate")
	outputDir := flag.String("output", "outputs", "Directory to store the generated block, by default will be outputs which is git ignored, if you'd wish to store it in a different directory, please update the flag and set it to testdata")
	dimensions := flag.Int("dimensions", 1, "Number of additional label dimensions to add to each series")
	cardinality := flag.Int("cardinality", 5, "Cardinality for each dimension (number of unique values per dimension)")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	now := time.Date(2024, time.December, 10, 10, 0, 0, 0, time.UTC)

	printSeparator("STEP 1: CREATING TSDB BLOCK")

	fmt.Printf("Creating a TSDB block with %d series in directory '%s'\n", *numSeries, *outputDir)
	fmt.Printf("Adding %d dimensions with cardinality %d\n", *dimensions, *cardinality)

	blockPath, err := createTSDBBlock(*numSeries, *outputDir, *dimensions, *cardinality, now.Unix()*1000, logger)
	if err != nil {
		fmt.Printf("Failed to create TSDB block: %v\n", err)
		return
	}
	fmt.Printf("TSDB block created successfully: %s\n", blockPath)

	printSeparator("STEP 2: CONVERTING TO COLUMNAR BLOCK")

	err = convertToColumnarBlock(blockPath, logger)
	if err != nil {
		fmt.Printf("Failed to convert to columnar block: %v\n", err)
		return
	}
	fmt.Println("Conversion to columnar block completed successfully")

	printSeparator("COMPLETED")
}

func printSeparator(title string) {
	fmt.Println()
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("  %s\n", title)
	fmt.Println(strings.Repeat("=", 80))
	fmt.Println()
}
