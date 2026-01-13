// Copyright The Prometheus Authors
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

//go:build fuzzing

//go:generate go run -tags fuzzing .

package main

import (
	"archive/zip"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/prometheus/prometheus/util/fuzzing"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Successfully generated all seed corpus ZIP files.")
}

func run() error {
	// Generate FuzzParseExpr seed corpus.
	exprs, err := fuzzing.GetCorpusForFuzzParseExpr()
	if err != nil {
		return fmt.Errorf("failed to get corpus for FuzzParseExpr: %w", err)
	}
	if err := generateZipFromStrings("fuzzParseExpr", exprs); err != nil {
		return fmt.Errorf("failed to generate FuzzParseExpr_seed_corpus.zip: %w", err)
	}
	fmt.Printf("Generated fuzzParseExpr_seed_corpus.zip with %d entries.\n", len(exprs))

	// Generate FuzzParseMetricSelector seed corpus.
	selectors := fuzzing.GetCorpusForFuzzParseMetricSelector()
	if err := generateZipFromStrings("fuzzParseMetricSelector", selectors); err != nil {
		return fmt.Errorf("failed to generate FuzzParseMetricSelector_seed_corpus.zip: %w", err)
	}
	fmt.Printf("Generated fuzzParseMetricSelector_seed_corpus.zip with %d entries.\n", len(selectors))

	// Generate FuzzParseMetricText seed corpus.
	metrics := fuzzing.GetCorpusForFuzzParseMetricText()
	if err := generateZipFromBytes("fuzzParseMetricText", metrics); err != nil {
		return fmt.Errorf("failed to generate FuzzParseMetricText_seed_corpus.zip: %w", err)
	}
	fmt.Printf("Generated fuzzParseMetricText_seed_corpus.zip with %d entries.\n", len(metrics))

	// Generate FuzzParseOpenMetric seed corpus.
	openMetrics := fuzzing.GetCorpusForFuzzParseOpenMetric()
	if err := generateZipFromBytes("fuzzParseOpenMetric", openMetrics); err != nil {
		return fmt.Errorf("failed to generate FuzzParseOpenMetric_seed_corpus.zip: %w", err)
	}
	fmt.Printf("Generated fuzzParseOpenMetric_seed_corpus.zip with %d entries.\n", len(openMetrics))

	return nil
}

// generateZipFromBytes creates a seed corpus ZIP file from a slice of byte slices.
func generateZipFromBytes(fuzzName string, corpus [][]byte) error {
	// Sort corpus deterministically.
	sorted := make([][]byte, len(corpus))
	copy(sorted, corpus)
	sort.Slice(sorted, func(i, j int) bool {
		return string(sorted[i]) < string(sorted[j])
	})

	// Create ZIP file in parent directory.
	zipPath := filepath.Join("..", fuzzName+"_seed_corpus.zip")
	zipFile, err := os.Create(zipPath)
	if err != nil {
		return fmt.Errorf("failed to create zip file: %w", err)
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	// Add each corpus entry as a file.
	for i, entry := range sorted {
		fileName := fmt.Sprintf("expr%d", i)
		writer, err := zipWriter.Create(fileName)
		if err != nil {
			return fmt.Errorf("failed to create zip entry %s: %w", fileName, err)
		}
		if _, err := writer.Write(entry); err != nil {
			return fmt.Errorf("failed to write zip entry %s: %w", fileName, err)
		}
	}

	return nil
}

// generateZipFromStrings creates a seed corpus ZIP file from a slice of strings.
func generateZipFromStrings(fuzzName string, corpus []string) error {
	// Convert []string to [][]byte and delegate to generateZipFromBytes
	byteCorpus := make([][]byte, len(corpus))
	for i, s := range corpus {
		byteCorpus[i] = []byte(s)
	}
	return generateZipFromBytes(fuzzName, byteCorpus)
}
