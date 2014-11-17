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

all: parser.y.go lexer.l.go

parser.y.go: parser.y
	$(GOCC) tool yacc -o parser.y.go -v "" parser.y

lexer.l.go: parser.y.go lexer.l
	# This is golex from https://github.com/cznic/golex.
	$(GO_GET) github.com/cznic/golex
	golex -o="lexer.l.go" lexer.l

clean:
	rm lexer.l.go parser.y.go
