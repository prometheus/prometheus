all: bench svg

bench: build 
	@echo ">> running benchmark"
	@./tindex bench write testdata

build:
	@go build .

svg:
	@echo ">> create svgs"
	@go tool pprof -svg ./tindex benchout/cpu.prof > benchout/cpuprof.svg
	@go tool pprof -svg ./tindex benchout/mem.prof > benchout/memprof.svg
	@go tool pprof -svg ./tindex benchout/block.prof > benchout/blockprof.svg
