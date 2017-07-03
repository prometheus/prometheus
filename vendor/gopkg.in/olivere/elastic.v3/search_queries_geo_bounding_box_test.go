// Copyright 2012-2015 Oliver Eilhard. All rights reserved.
// Use of this source code is governed by a MIT-license.
// See http://olivere.mit-license.org/license.txt for details.

package elastic

import (
	"encoding/json"
	"testing"
)

func TestGeoBoundingBoxQueryIncomplete(t *testing.T) {
	q := NewGeoBoundingBoxQuery("pin.location")
	q = q.TopLeft(40.73, -74.1)
	// no bottom and no right here
	q = q.Type("memory")
	src, err := q.Source()
	if err == nil {
		t.Fatal("expected error")
	}
	if src != nil {
		t.Fatal("expected empty source")
	}
}

func TestGeoBoundingBoxQuery(t *testing.T) {
	q := NewGeoBoundingBoxQuery("pin.location")
	q = q.TopLeft(40.73, -74.1)
	q = q.BottomRight(40.01, -71.12)
	q = q.Type("memory")
	src, err := q.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"geo_bbox":{"pin.location":{"bottom_right":[-71.12,40.01],"top_left":[-74.1,40.73]},"type":"memory"}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}

func TestGeoBoundingBoxQueryWithGeoPoint(t *testing.T) {
	q := NewGeoBoundingBoxQuery("pin.location")
	q = q.TopLeftFromGeoPoint(GeoPointFromLatLon(40.73, -74.1))
	q = q.BottomRightFromGeoPoint(GeoPointFromLatLon(40.01, -71.12))
	src, err := q.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"geo_bbox":{"pin.location":{"bottom_right":[-71.12,40.01],"top_left":[-74.1,40.73]}}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}
