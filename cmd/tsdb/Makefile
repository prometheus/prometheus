build:
	@go build .
	
bench_default: build
	@echo ">> running benchmark"
	@./tsdb bench write --out=benchout/default --engine=default --metrics=$(NUM_METRICS) testdata.100k
	@go tool pprof -svg ./tsdb benchout/default/cpu.prof > benchout/default/cpuprof.svg
	@go tool pprof -svg ./tsdb benchout/default/mem.prof > benchout/default/memprof.svg
	@go tool pprof -svg ./tsdb benchout/default/block.prof > benchout/default/blockprof.svg

bench_tsdb: build 
	@echo ">> running benchmark"
	@./tsdb bench write --out=benchout/tsdb --engine=tsdb --metrics=$(NUM_METRICS) testdata.100k
	@go tool pprof -svg ./tsdb benchout/tsdb/cpu.prof > benchout/tsdb/cpuprof.svg
	@go tool pprof -svg ./tsdb benchout/tsdb/mem.prof > benchout/tsdb/memprof.svg
	@go tool pprof -svg ./tsdb benchout/tsdb/block.prof > benchout/tsdb/blockprof.svg
