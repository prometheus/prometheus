/*
package dump is a utility for printing memory allocations info using printf.
This allows instrumentation of code for memory allocation analysis with very
little effort.

- MemStats: Uses `runtime.ReadMemStats`. The numbers shown include "garbage" not
yet collected by GC according to Go docs.

- MemProf: Uses code from pprof package to show stats as would be
shown by pprof tool. The pprof itself uses some probabilistic approach.
Hopefully this is more accurate than `MemStats` and do not include "garbage".

- WriteHeapDump():  Write pprof heap profiles at any point in code.

Sample usage:

    func Example() []int64 {
    	// Write pprof heap profile at the start and end of function
    	dump.WriteHeapDump(fmt.Sprintf("heap-example-before"))
    	defer dump.WriteHeapDump(fmt.Sprintf("heap-example-after"))

    	// Take a snapshot at the start of a function
    	// Capture and print deltas at the end
    	memProf := dump.NewMemProf("example")
    	defer memProf.PrintDiff()

    	// Similar for memStats
    	memStats := dump.NewMemStats("example")
    	defer memStats.PrintDiff()

    	// allocate memory
    	allocateMem := func () []int64 {
    		return make([]int64, 100000)
    	}

    	var result []int64
    	for i := 0; i < 1000; i++{
    		result = append(result, allocateMem()...)
    	}

    	return result
    }

This will produce results similar to this:

	WRITTEN HEAP DUMP TO /Users/philip/thanos/github.com/ppanyukov/go-dump/heap-example-before.pb.gz
	MEM STATS DIFF:   	example 	example - AFTER 	-> Delta
		HeapAlloc  : 	219.76K 	963.52M 		-> 963.30M
		HeapObjects: 	203 		408 			-> 205

	MEM PROF DIFF:    	example 	example - AFTER 	-> Delta
		InUseBytes  : 	0 		816.39M 		-> 816.39M
		InUseObjects: 	0 		1 			-> 1
		AllocBytes  : 	1.26M 		4.69G 			-> 4.69G
		AllocObjects: 	6 		808 			-> 802

	WRITTEN HEAP DUMP TO /Users/philip/thanos/github.com/ppanyukov/go-dump/heap-example-after.pb.gz

The size of 1,000,000 int64 values is `800MB`, and can be seen `MemProf` reports this
quite close to this number. The MemStats are slightly off with its `963.30M` as this
number includes objects not yet collected by GC.

To use pprof tool to see allocations use this command:

	go tool pprof \
		-sample_index=inuse_space \
		-edgefraction=0 \
		-functions \
		-http=:8081 \
		-base /Users/philip/thanos/github.com/ppanyukov/go-dump/heap-example-before.pb.gz \
 		/Users/philip/thanos/github.com/ppanyukov/go-dump/heap-example-after.pb.gz

This would report figures like so:

	Showing nodes accounting for 778.57MB, 100% of 778.57MB total

Also close to `800MB`.

*/
package dump


