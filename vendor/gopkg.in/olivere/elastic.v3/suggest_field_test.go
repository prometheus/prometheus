// Copyright 2012-2015 Oliver Eilhard. All rights reserved.
// Use of this source code is governed by a MIT-license.
// See http://olivere.mit-license.org/license.txt for details.

package elastic

import (
	"encoding/json"
	"testing"
)

func TestSuggestField(t *testing.T) {
	field := NewSuggestField().
		Input("Welcome to Golang and Elasticsearch.", "Golang and Elasticsearch").
		Output("Golang and Elasticsearch: An introduction.").
		Weight(1).
		ContextQuery(
			NewSuggesterCategoryMapping("color").FieldName("color_field").DefaultValues("red", "green", "blue"),
			NewSuggesterGeoMapping("location").Precision("5m").Neighbors(true).DefaultLocations(GeoPointFromLatLon(52.516275, 13.377704)),
		)
	data, err := json.Marshal(field)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"context":[{"color":{"default":["red","green","blue"],"path":"color_field","type":"category"}},{"location":{"default":{"lat":52.516275,"lon":13.377704},"neighbors":true,"precision":["5m"],"type":"geo"}}],"input":["Welcome to Golang and Elasticsearch.","Golang and Elasticsearch"],"output":"Golang and Elasticsearch: An introduction.","weight":1}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}
