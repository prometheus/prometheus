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

include ../../Makefile.INCLUDE

all: populate

populate: gperftools-$(GPERFTOOLS_VERSION).tar.gz leveldb-$(LEVELDB_VERSION).tar.gz protobuf-$(PROTOCOL_BUFFERS_VERSION).tar.bz2 protobuf-c-$(PROTOBUF_C_VERSION).tar.gz snappy-$(SNAPPY_VERSION).tar.gz

gperftools-$(GPERFTOOLS_VERSION).tar.gz: wget-stamp
	$(WGET) https://gperftools.googlecode.com/files/gperftools-$(GPERFTOOLS_VERSION).tar.gz

leveldb-$(LEVELDB_VERSION).tar.gz: wget-stamp
	$(WGET) http://leveldb.googlecode.com/files/leveldb-$(LEVELDB_VERSION).tar.gz

protobuf-$(PROTOCOL_BUFFERS_VERSION).tar.bz2: wget-stamp
	$(WGET) http://protobuf.googlecode.com/files/$@

protobuf-c-$(PROTOBUF_C_VERSION).tar.gz: wget-stamp
	$(WGET) https://protobuf-c.googlecode.com/files/$@

snappy-$(SNAPPY_VERSION).tar.gz: wget-stamp
	$(WGET) http://snappy.googlecode.com/files/snappy-$(SNAPPY_VERSION).tar.gz

wget-implementation-Darwin-stamp:
	[ -x "$$(which wget)" ] || $(BREW_INSTALL) wget
	touch $@

wget-implementation-Linux-stamp:
	[ -x "$$(which wget)" ] || $(APT_GET_INSTALL) wget
	touch $@

wget-stamp: wget-implementation-$(UNAME)-stamp
	[ -x "$$(which wget)" ] || { echo "wget not found." ; false ; }
	touch $@

clean:
	-[ -n "$(REALLY_CLEAN)" ] && rm -rf *.bz2
	-[ -n "$(REALLY_CLEAN)" ] && rm -rf *.gz
	rm -rf *-stamp

.PHONY: clean populate
