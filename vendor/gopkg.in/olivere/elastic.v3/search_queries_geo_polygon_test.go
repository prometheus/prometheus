// Copyright 2012-2015 Oliver Eilhard. All rights reserved.
// Use of this source code is governed by a MIT-license.
// See http://olivere.mit-license.org/license.txt for details.

package elastic

import (
	"encoding/json"
	"testing"
)

func TestGeoPolygonQuery(t *testing.T) {
	q := NewGeoPolygonQuery("person.location")
	q = q.AddPoint(40, -70)
	q = q.AddPoint(30, -80)
	point, err := GeoPointFromString("20,-90")
	if err != nil {
		t.Fatalf("GeoPointFromString failed: %v", err)
	}
	q = q.AddGeoPoint(point)
	src, err := q.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"geo_polygon":{"person.location":{"points":[{"lat":40,"lon":-70},{"lat":30,"lon":-80},{"lat":20,"lon":-90}]}}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}

func TestGeoPolygonQueryFromGeoPoints(t *testing.T) {
	q := NewGeoPolygonQuery("person.location")
	q = q.AddGeoPoint(&GeoPoint{Lat: 40, Lon: -70})
	q = q.AddGeoPoint(GeoPointFromLatLon(30, -80))
	point, err := GeoPointFromString("20,-90")
	if err != nil {
		t.Fatalf("GeoPointFromString failed: %v", err)
	}
	q = q.AddGeoPoint(point)
	src, err := q.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"geo_polygon":{"person.location":{"points":[{"lat":40,"lon":-70},{"lat":30,"lon":-80},{"lat":20,"lon":-90}]}}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}
