build:
	@go build .
	
bench: build 
	@echo ">> running benchmark"
	@./tsdb bench write --metrics=$(NUM_METRICS) testdata.1m
	@go tool pprof -svg ./tsdb benchout/cpu.prof > benchout/cpuprof.svg
	@go tool pprof --inuse_space -svg ./tsdb benchout/mem.prof > benchout/memprof.inuse.svg
	@go tool pprof --alloc_space -svg ./tsdb benchout/mem.prof > benchout/memprof.alloc.svg
	@go tool pprof -svg ./tsdb benchout/block.prof > benchout/blockprof.svg
	@go tool pprof -svg ./tsdb benchout/mutex.prof > benchout/mutexprof.svg
