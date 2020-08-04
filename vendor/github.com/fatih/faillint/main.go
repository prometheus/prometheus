package main

import (
	"github.com/fatih/faillint/faillint"
	"golang.org/x/tools/go/analysis/singlechecker"
)

func main() {
	singlechecker.Main(faillint.NewAnalyzer())
}
