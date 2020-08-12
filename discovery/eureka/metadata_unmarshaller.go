// Copyright 2020 The Prometheus Authors
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

package eureka

import (
	"encoding/xml"
)

type MetaData struct {
	Map   map[string]string
	Class string
}

type Vraw struct {
	Content []byte `xml:",innerxml"`
	Class   string `xml:"class,attr"`
}

func (s *MetaData) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	s.Map = make(map[string]string)

	for {
		token, err := d.Token()
		if err != nil {
			return err
		}

		switch el := token.(type) {
		case xml.StartElement:
			item := new(string)
			err = d.DecodeElement(item, &el)
			if err != nil {
				return err
			}
			s.Map[el.Name.Local] = *item
		case xml.EndElement:
			if el == start.End() {
				return nil
			}
		}
	}
}
