export PATH := $(PATH):/Users/mtp/Development/go/bin

all: data.proto

data.proto:
	protoc --go_out=generated/ data.proto
clean:
	rm -rf generated/*

.PHONY: data.proto
