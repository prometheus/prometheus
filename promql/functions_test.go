package promql

import "testing"

func BenchmarkDoubleSmooth4Week5Min(b *testing.B) {
	input := `
clear
load 5m
    http_requests{path="/foo"}    0+10x8064

eval instant at 4w holt_winters(http_requests[4w], 0.3, 0.3)
    {path="/foo"} 20160
`

	bench := NewBenchmark(b, input)
	bench.Run()

}

func BenchmarkDoubleSmooth1Week5Min(b *testing.B) {
	input := `
clear
load 5m
    http_requests{path="/foo"}    0+10x2016

eval instant at 1w holt_winters(http_requests[1w], 0.3, 0.3)
    {path="/foo"} 20160
`

	bench := NewBenchmark(b, input)
	bench.Run()
}

func BenchmarkDoubleSmooth1Day1Min(b *testing.B) {
	input := `

clear
load 1m
    http_requests{path="/foo"}    0+10x1440

eval instant at 1d holt_winters(http_requests[1d], 0.3, 0.3)
    {path="/foo"} 20160
`

	bench := NewBenchmark(b, input)
	bench.Run()
}
