package eureka

import (
	"encoding/xml"
	"encoding/json"
	"regexp"
)

type MetaData struct {
	Map   map[string]string
	Class string
}


type Vraw struct {
	Content []byte `xml:",innerxml"`
	Class   string `xml:"class,attr" json:"@class"`
}

func (s *MetaData) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	var attributes []xml.Attr = make([]xml.Attr, 0)
	if s.Class != "" {
		attributes = append(attributes, xml.Attr{
			Name: xml.Name{
				Local: "class",
			},
			Value: s.Class,
		})
	}
	start.Attr = attributes
	tokens := []xml.Token{start}

	for key, value := range s.Map {
		t := xml.StartElement{Name: xml.Name{"", key}}
		tokens = append(tokens, t, xml.CharData(value), xml.EndElement{t.Name})
	}

	tokens = append(tokens, xml.EndElement{
		Name: start.Name,
	})


	for _, t := range tokens {
		err := e.EncodeToken(t)
		if err != nil {
			return err
		}
	}

	// flush to ensure tokens are written
	err := e.Flush()
	if err != nil {
		return err
	}

	return nil
}

func (s *MetaData) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	s.Map = make(map[string]string)
	vraw := &Vraw{}
	d.DecodeElement(vraw, &start)
	dataInString := string(vraw.Content)
	regex, err := regexp.Compile("\\s*<([^<>]+)>([^<>]+)</[^<>]+>\\s*")
	if err != nil {
		return err
	}
	subMatches := regex.FindAllStringSubmatch(dataInString, -1)
	for _, subMatch := range subMatches {
		s.Map[subMatch[1]] = subMatch[2]
	}
	s.Class = vraw.Class
	return nil
}

func (s *MetaData) MarshalJSON() ([]byte, error) {
	mapIt := make(map[string]string)
	for key, value := range s.Map {
		mapIt[key] = value
	}
	if s.Class != "" {
		mapIt["@class"] = s.Class
	}
	return json.Marshal(mapIt)
}
func (s *MetaData) UnmarshalJSON(data []byte) error {
	dataUnmarshal := make(map[string]string)
	err := json.Unmarshal(data, dataUnmarshal)
	s.Map = dataUnmarshal
	if val, ok := s.Map["@class"]; ok {
		s.Class = val
		delete(s.Map, "@class")
	}
	return err
}