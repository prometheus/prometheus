package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
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

	printSeparator("STEP 1: CREATING TSDB BLOCK")

	fmt.Printf("Creating a TSDB block with %d series in directory '%s'\n", *numSeries, *outputDir)
	fmt.Printf("Adding %d dimensions with cardinality %d\n", *dimensions, *cardinality)

	blockPath, err := createTSDBBlock(*numSeries, *outputDir, *dimensions, *cardinality, logger)
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
