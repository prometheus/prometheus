// Copyright 2013 Prometheus Team
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package coding

import (
	"code.google.com/p/goprotobuf/proto"
	"fmt"
)

type ProtocolBuffer struct {
	message proto.Message
}

func (p ProtocolBuffer) Encode() (raw []byte, err error) {
	raw, err = proto.Marshal(p.message)

	// XXX: Adjust legacy users of this to not check for error.
	if err != nil {
		panic(err)
	}

	return
}

func (p ProtocolBuffer) String() string {
	return fmt.Sprintf("ProtocolBufferEncoder of %s", p.message)
}

func NewProtocolBuffer(message proto.Message) *ProtocolBuffer {
	return &ProtocolBuffer{
		message: message,
	}
}
