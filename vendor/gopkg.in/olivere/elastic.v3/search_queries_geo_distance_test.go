// Copyright 2012-2015 Oliver Eilhard. All rights reserved.
// Use of this source code is governed by a MIT-license.
// See http://olivere.mit-license.org/license.txt for details.

package elastic

import (
	"encoding/json"
	"testing"
)

func TestGeoDistanceQuery(t *testing.T) {
	q := NewGeoDistanceQuery("pin.location")
	q = q.Lat(40)
	q = q.Lon(-70)
	q = q.Distance("200km")
	q = q.DistanceType("plane")
	q = q.OptimizeBbox("memory")
	src, err := q.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"geo_distance":{"distance":"200km","distance_type":"plane","optimize_bbox":"memory","pin.location":{"lat":40,"lon":-70}}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}

func TestGeoDistanceQueryWithGeoPoint(t *testing.T) {
	q := NewGeoDistanceQuery("pin.location")
	q = q.GeoPoint(GeoPointFromLatLon(40, -70))
	q = q.Distance("200km")
	src, err := q.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"geo_distance":{"distance":"200km","pin.location":{"lat":40,"lon":-70}}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}

func TestGeoDistanceQueryWithGeoHash(t *testing.T) {
	q := NewGeoDistanceQuery("pin.location")
	q = q.GeoHash("drm3btev3e86")
	q = q.Distance("12km")
	src, err := q.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"geo_distance":{"distance":"12km","pin.location":"drm3btev3e86"}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}
