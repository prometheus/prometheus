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

type PBEncoder struct {
	message proto.Message
}

func (p PBEncoder) MustEncode() []byte {
	raw, err := proto.Marshal(p.message)
	if err != nil {
		panic(err)
	}

	return raw
}

func (p PBEncoder) String() string {
	return fmt.Sprintf("PBEncoder of %T", p.message)
}

// BUG(matt): Replace almost all calls to this with mechanisms that wrap the
// underlying protocol buffers with business logic types that simply encode
// themselves.  If all points are done, then we'll no longer need this type.
func NewPBEncoder(m proto.Message) PBEncoder {
	return PBEncoder{message: m}
}
