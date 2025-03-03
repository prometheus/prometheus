package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
)

func main() {
	// Parse command line flags
	numSeries := flag.Int("series", 100, "Number of series to generate")
	outputDir := flag.String("output", "outputs", "Directory to store the generated block, by default will be outputs which is git ignored, if you'd wish to store it in a different directory, please update the flag and set it to testdata")
	flag.Parse()

	// Create a proper structured logger
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// Step 1: Create a TSDB block
	fmt.Printf("Step 1: Creating a TSDB block with %d series in directory '%s'...\n", *numSeries, *outputDir)
	blockPath, err := createTSDBBlock(*numSeries, *outputDir, logger)
	if err != nil {
		fmt.Printf("Failed to create TSDB block: %v\n", err)
		return
	}
	fmt.Printf("TSDB block created successfully: %s\n", blockPath)

	// Step 2: Convert TSDB block to columnar block
	fmt.Println("Step 2: Converting TSDB block to columnar block...")
	err = convertToColumnarBlock(blockPath, logger)
	if err != nil {
		fmt.Printf("Failed to convert to columnar block: %v\n", err)
		return
	}
	fmt.Println("Conversion to columnar block completed successfully")
}
