package agent

import (
	"runtime"
	"strconv"
)

// runtimeStats is used to return various runtime information
func runtimeStats() map[string]string {
	return map[string]string{
		"os":         runtime.GOOS,
		"arch":       runtime.GOARCH,
		"version":    runtime.Version(),
		"max_procs":  strconv.FormatInt(int64(runtime.GOMAXPROCS(0)), 10),
		"goroutines": strconv.FormatInt(int64(runtime.NumGoroutine()), 10),
		"cpu_count":  strconv.FormatInt(int64(runtime.NumCPU()), 10),
	}
}
