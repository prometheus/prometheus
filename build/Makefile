# Copyright 2013 Prometheus Team
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

.SUFFIXES:

include ../Makefile.INCLUDE

all: dependencies-stamp

bison-stamp: bison-implementation-$(UNAME)-stamp
	[ -x "$$(which bison)" ] || { echo "bison not found." ; false ; }
	touch $@

bison-implementation-Darwin-stamp:
	[ -x "$$(which bison)" ] || $(BREW_INSTALL) bison
	touch $@

bison-implementation-Linux-stamp:
	[ -x "$$(which bison)" ] || $(APT_GET_INSTALL) bison
	touch $@

cache-stamp:
	$(MAKE) -C cache
	touch $@

cc-stamp: cc-implementation-$(UNAME)-stamp
	[ -x "$$(which cc)" ] || { echo "cc not found." ; false ; }
	touch $@

cc-implementation-Darwin-stamp:
	[ -x "$$(which cc)" ] || { echo "Install XCode?" ; false ; }
	touch $@

cc-implementation-Linux-stamp:
	[ -x "$$(which cc)" ] || $(APT_GET_INSTALL) build-essential
	touch $@

dependencies-stamp: cache-stamp go-stamp goprotobuf-stamp gorest-stamp goskiplist-stamp instrumentation-stamp levigo-stamp
	touch $@

go-stamp: gvm-stamp
	[ -x "$$(which go)" ] || { echo "go not found." ; false ; }
	touch $@

goprotobuf-protoc-gen-go-stamp: protoc-stamp
	$(GO_GET) code.google.com/p/goprotobuf/protoc-gen-go $(THIRD_PARTY_BUILD_OUTPUT)
	touch $@

goprotobuf-stamp: go-stamp mercurial-stamp
	$(GO_GET) code.google.com/p/goprotobuf/proto $(THIRD_PARTY_BUILD_OUTPUT)
	touch $@

gorest-stamp: go-stamp
	$(GO_GET) code.google.com/p/gorest $(THIRD_PARTY_BUILD_OUTPUT)
	touch $@

goskiplist-stamp: go-stamp
	$(GO_GET) github.com/ryszard/goskiplist/skiplist $(THIRD_PARTY_BUILD_OUTPUT)
	touch $@

gvm-stamp: bison-stamp cc-stamp mercurial-stamp
	[ -x "$$(which gvm)" ] || { echo "gvm not found; install manually from https://github.com/moovweb/gvm." ; false ; }
	-gvm install go$(GO_VERSION)
	#
	# ********** **********
	# gvm installed go$(GO_VERSION) if it did not exist already.
  #
  # Prometheus expects go$(GO_VERSION) to be used for building.
	# This can be achieved by running ``gvm use go$(GO_VERSION)`` before any
	# other Prometheus make tools.
	#
	# Once this build finishes, $(GOPATH) will be mutated and
	# contain some new packages.  Here is an example workflow:
	#
	# $ gvm install go$(GO_VERSION)
	# $ gvm use go$(GO_VERSION)
	# $ gvm pkgset create prometheus
	# $ gvm pkgset use prometheus
	#
	# ********** **********
	#
	-[ -z "${CI}" ] && read -p "Press [ENTER] upon reading." _
	touch $@

instrumentation-stamp: go-stamp
	$(GO_GET) github.com/prometheus/client_golang/prometheus $(THIRD_PARTY_BUILD_OUTPUT)
	$(GO_GET) github.com/prometheus/client_golang/prometheus/exp $(THIRD_PARTY_BUILD_OUTPUT)
	touch $@

leveldb-stamp: cache-stamp cache/leveldb-$(LEVELDB_VERSION).tar.gz cc-stamp rsync-stamp snappy-stamp
	tar xzvf cache/leveldb-$(LEVELDB_VERSION).tar.gz -C dirty $(THIRD_PARTY_BUILD_OUTPUT)
	cd dirty/leveldb-$(LEVELDB_VERSION) && CFLAGS="$(CFLAGS) -lsnappy" CXXFLAGS="$(CXXFLAGS) -lsnappy $(LDFLAGS)" LDFLAGS="-lsnappy $(LDFLAGS)" bash -x ./build_detect_platform build_config.mk ./
	# The test that LevelDB uses to test for Snappy is naive and
	# does not respect LDFLAGS.  :-(
	CFLAGS="$(CFLAGS) -lsnappy" CXXFLAGS="$(CXXFLAGS) -lsnappy $(LDFLAGS)" LDFLAGS="-lsnappy $(LDFLAGS)" $(MAKE) -C dirty/leveldb-$(LEVELDB_VERSION) $(THIRD_PARTY_BUILD_OUTPUT)
	rsync -av "dirty/leveldb-$(LEVELDB_VERSION)/include/" "$(PREFIX)/include/" $(THIRD_PARTY_BUILD_OUTPUT)
	-[ "$(UNAME)" = "Linux" ] && { rsync -av "dirty/leveldb-$(LEVELDB_VERSION)/"*.*so* "$(PREFIX)/lib/" ; } $(THIRD_PARTY_BUILD_OUTPUT) $(THIRD_PARTY_BUILD_OUTPUT)
	-[ "$(UNAME)" = "Darwin" ] && { rsync -av "dirty/leveldb-$(LEVELDB_VERSION)/"*.*dylib* "$(PREFIX)/lib/" ; } $(THIRD_PARTY_BUILD_OUTPUT)
	rsync -av "dirty/leveldb-$(LEVELDB_VERSION)/"*.a "$(PREFIX)/lib/" $(THIRD_PARTY_BUILD_OUTPUT)
	touch $@

levigo-stamp: cc-stamp go-stamp leveldb-stamp snappy-stamp
	CGO_CFLAGS="-I$(PREFIX)/include" CGO_LDFLAGS="-L$(PREFIX)/lib" $(GO_GET) github.com/jmhodges/levigo $(THIRD_PARTY_BUILD_OUTPUT)
	touch $@

libunwind-stamp:
	$(APT_GET_INSTALL) libunwind7
	$(APT_GET_INSTALL) libunwind7-dev
	touch $@

mercurial-implementation-Darwin-stamp:
	[ -x "$$(which hg)" ] || $(BREW_INSTALL) mercurial
	touch $@

mercurial-implementation-Linux-stamp:
	[ -x "$$(which hg)" ] || $(APT_GET_INSTALL) mercurial
	touch $@

mercurial-stamp: mercurial-implementation-$(UNAME)-stamp
	[ -x "$$(which hg)" ] || { echo "hg not found." ; false ; }
	touch $@

noop-target-stamp:
	echo "Not doing anything."
	touch $@

protoc-stamp: cache-stamp cache/protobuf-$(PROTOCOL_BUFFERS_VERSION).tar.bz2 cc-stamp
	tar xjvf cache/protobuf-$(PROTOCOL_BUFFERS_VERSION).tar.bz2 -C dirty $(THIRD_PARTY_BUILD_OUTPUT)
	cd dirty/protobuf-$(PROTOCOL_BUFFERS_VERSION) && ./configure --prefix="$(PREFIX)" $(THIRD_PARTY_BUILD_OUTPUT)
	$(MAKE) -C dirty/protobuf-$(PROTOCOL_BUFFERS_VERSION) $(THIRD_PARTY_BUILD_OUTPUT)
	$(MAKE) -C dirty/protobuf-$(PROTOCOL_BUFFERS_VERSION) install $(THIRD_PARTY_BUILD_OUTPUT)
	[ -x "$$(which protoc)" ] || { echo "protoc not found." ; false ; }
	touch $@

rsync-implementation-Darwin-stamp:
	[ -x "$$(which rsync)" ] || $(BREW_INSTALL) rsync
	touch $@

rsync-implementation-Linux-stamp:
	[ -x "$$(which rsync)" ] || $(APT_GET_INSTALL) rsync

rsync-stamp: rsync-implementation-$(UNAME)-stamp
	[ -x "$$(which rsync)" ] || { echo "rsync not found." ; false ; }
	touch $@

snappy-stamp: cache-stamp cache/snappy-$(SNAPPY_VERSION).tar.gz cc-stamp
	tar xzvf cache/snappy-$(SNAPPY_VERSION).tar.gz -C dirty $(THIRD_PARTY_BUILD_OUTPUT)
	cd dirty/snappy-$(SNAPPY_VERSION) && ./configure --prefix="$(PREFIX)" $(THIRD_PARTY_BUILD_OUTPUT)
	$(MAKE) -C dirty/snappy-$(SNAPPY_VERSION) $(THIRD_PARTY_BUILD_OUTPUT)
	$(MAKE) -C dirty/snappy-$(SNAPPY_VERSION) install $(THIRD_PARTY_BUILD_OUTPUT)
	touch $@

ifeq ($(UNAME), Linux)
stack-unwind-support-stamp: libunwind-stamp
	touch $@
else
stack-unwind-support-stamp: noop-target-stamp
	touch $@
endif

vim-implementation-Darwin-stamp:
	[ -x "$$(which vim)" ] || $(BREW_INSTALL) vim
	touch $@

vim-implementation-Linux-stamp:
	[ -x "$$(which vim)" ] || $(APT_GET_INSTALL) vim
	touch $@

vim-stamp: vim-implementation-$(UNAME)-stamp
	touch $@

clean:
	$(MAKE) -C cache clean
	$(MAKE) -C dirty clean
	$(MAKE) -C root clean
	rm -rf *-stamp


.PHONY: clean
