# Getting started

## Installation

### Go

First, create a `$HOME/mygo` directory and its src subdirectory:

```bash
mkdir -p $HOME/mygo/src # create a place to put source code
```

Next, set it as the GOPATH. You should also add the bin subdirectory to your PATH environment variable so that you can run the commands therein without specifying their full path. To do this, add the following lines to `$HOME/.profile` (or equivalent):

```bash
export GOPATH=$HOME/mygo
export PATH=$PATH:$HOME/mygo/bin
```

Now you can install Go:

```bash
brew install go
```


### Dependencies

Install leveldb and protobuf dependencies:

```bash
brew install leveldb protobuf
```


### Libraries

```bash
go get code.google.com/p/goprotobuf/{proto,protoc-gen-go}
go get github.com/jmhodges/levigo
go get code.google.com/p/gorest
go get github.com/prometheus/{prometheus,client_golang}
```


## Build

```bash
cd ${GOPATH}/src/github.com/prometheus/prometheus
make build
```

## Configure

```bash
cp prometheus.conf.example prometheus.conf
```

## Run

```bash
./prometheus
```
