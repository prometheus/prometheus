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

all: blob-stamp

blob-stamp: static/generated/protocol_buffer.descriptor
	$(MAKE) -C blob
	touch $@

clean:
	$(MAKE) -C blob clean
	-rm -f *-stamp

static/generated:
	mkdir -vp static/generated

static/generated/protocol_buffer.descriptor: static/generated ../model/generated/descriptor.blob
	cp -f ../model/generated/descriptor.blob $@

.PHONY: clean
