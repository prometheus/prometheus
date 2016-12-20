build:
	@go build .
	
bench: build 
	@echo ">> running benchmark"
	@./tsdb bench write --out=benchout/ --metrics=$(NUM_METRICS) testdata.100k
	@go tool pprof -svg ./tsdb benchout/cpu.prof > benchout/cpuprof.svg
	@go tool pprof -svg ./tsdb benchout/mem.prof > benchout/memprof.svg
	@go tool pprof -svg ./tsdb benchout/block.prof > benchout/blockprof.svg
