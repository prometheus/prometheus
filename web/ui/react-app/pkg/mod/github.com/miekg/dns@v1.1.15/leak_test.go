package dns

import (
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
)

// copied from net/http/main_test.go

func interestingGoroutines() (gs []string) {
	buf := make([]byte, 2<<20)
	buf = buf[:runtime.Stack(buf, true)]
	for _, g := range strings.Split(string(buf), "\n\n") {
		sl := strings.SplitN(g, "\n", 2)
		if len(sl) != 2 {
			continue
		}
		stack := strings.TrimSpace(sl[1])
		if stack == "" ||
			strings.Contains(stack, "testing.(*M).before.func1") ||
			strings.Contains(stack, "os/signal.signal_recv") ||
			strings.Contains(stack, "created by net.startServer") ||
			strings.Contains(stack, "created by testing.RunTests") ||
			strings.Contains(stack, "closeWriteAndWait") ||
			strings.Contains(stack, "testing.Main(") ||
			strings.Contains(stack, "testing.(*T).Run(") ||
			// These only show up with GOTRACEBACK=2; Issue 5005 (comment 28)
			strings.Contains(stack, "runtime.goexit") ||
			strings.Contains(stack, "created by runtime.gc") ||
			strings.Contains(stack, "dns.interestingGoroutines") ||
			strings.Contains(stack, "runtime.MHeap_Scavenger") {
			continue
		}
		gs = append(gs, stack)
	}
	sort.Strings(gs)
	return
}

func goroutineLeaked() error {
	if testing.Short() {
		// Don't worry about goroutine leaks in -short mode or in
		// benchmark mode. Too distracting when there are false positives.
		return nil
	}

	var stackCount map[string]int
	for i := 0; i < 5; i++ {
		n := 0
		stackCount = make(map[string]int)
		gs := interestingGoroutines()
		for _, g := range gs {
			stackCount[g]++
			n++
		}
		if n == 0 {
			return nil
		}
		// Wait for goroutines to schedule and die off:
		time.Sleep(100 * time.Millisecond)
	}
	for stack, count := range stackCount {
		fmt.Fprintf(os.Stderr, "%d instances of:\n%s\n", count, stack)
	}
	return fmt.Errorf("too many goroutines running after dns test(s)")
}
