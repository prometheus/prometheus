// Copyright 2016 The Cockroach Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
// implied. See the License for the specific language governing
// permissions and limitations under the License.
//
// Author: Tamir Duberstein (tamird@gmail.com)

package protoutil

import "github.com/gogo/protobuf/proto"

// Interceptor will be called with every proto before it is marshalled.
// Interceptor is not safe to modify concurrently with calls to Marshal.
var Interceptor = func(_ proto.Message) {}

// Marshal uses proto.Marshal to encode pb into the wire format. It is used in
// some tests to intercept calls to proto.Marshal.
func Marshal(pb proto.Message) ([]byte, error) {
	Interceptor(pb)

	return proto.Marshal(pb)
}
