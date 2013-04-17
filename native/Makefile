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

include ../Makefile.INCLUDE

MAKE_ARTIFACTS := data.pb-c.c data.pb-c.h

all: test

build:
	CGO_CFLAGS="$(CFLAGS)" CGO_LDFLAGS="$(LDFLAGS)" go build .

test: build data.pb-c.c data.pb-c.h
	CGO_CFLAGS="$(CFLAGS)" CGO_LDFLAGS="$(LDFLAGS)" go test .

data.pb-c.c:
	echo "Run make in model"
	false

data.pb-c.h:
	echo "Run make in model"
	false

clean:
	rm -rf $(MAKE_ARTIFACTS)

.PHONY: build clean test
