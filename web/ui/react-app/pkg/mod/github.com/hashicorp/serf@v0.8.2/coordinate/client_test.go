package coordinate

import (
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestClient_NewClient(t *testing.T) {
	config := DefaultConfig()

	config.Dimensionality = 0
	client, err := NewClient(config)
	if err == nil || !strings.Contains(err.Error(), "dimensionality") {
		t.Fatal(err)
	}

	config.Dimensionality = 7
	client, err = NewClient(config)
	if err != nil {
		t.Fatal(err)
	}

	origin := NewCoordinate(config)
	if !reflect.DeepEqual(client.GetCoordinate(), origin) {
		t.Fatalf("fresh client should be located at the origin")
	}
}

func TestClient_Update(t *testing.T) {
	config := DefaultConfig()
	config.Dimensionality = 3

	client, err := NewClient(config)
	if err != nil {
		t.Fatal(err)
	}

	// Make sure the Euclidean part of our coordinate is what we expect.
	c := client.GetCoordinate()
	verifyEqualVectors(t, c.Vec, []float64{0.0, 0.0, 0.0})

	// Place a node right above the client and observe an RTT longer than the
	// client expects, given its distance.
	other := NewCoordinate(config)
	other.Vec[2] = 0.001
	rtt := time.Duration(2.0 * other.Vec[2] * secondsToNanoseconds)
	c, err = client.Update("node", other, rtt)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	// The client should have scooted down to get away from it.
	if !(c.Vec[2] < 0.0) {
		t.Fatalf("client z coordinate %9.6f should be < 0.0", c.Vec[2])
	}

	// Set the coordinate to a known state.
	c.Vec[2] = 99.0
	client.SetCoordinate(c)
	c = client.GetCoordinate()
	verifyEqualFloats(t, c.Vec[2], 99.0)
}

func TestClient_InvalidInPingValues(t *testing.T) {
	config := DefaultConfig()
	config.Dimensionality = 3

	client, err := NewClient(config)
	if err != nil {
		t.Fatal(err)
	}

	// Place another node
	other := NewCoordinate(config)
	other.Vec[2] = 0.001
	dist := client.DistanceTo(other)

	// Update with a series of invalid ping periods, should return an error and estimated rtt remains unchanged
	pings := []int{1<<63 - 1, -35, 11}

	for _, ping := range pings {
		expectedErr := fmt.Errorf("round trip time not in valid range, duration %v is not a positive value less than %v", ping, 10*time.Second)
		_, err = client.Update("node", other, time.Duration(ping*secondsToNanoseconds))
		if err == nil {
			t.Fatalf("Unexpected error, wanted %v but got %v", expectedErr, err)
		}

		dist_new := client.DistanceTo(other)
		if dist_new != dist {
			t.Fatalf("distance estimate %v not equal to %v", dist_new, dist)
		}
	}

}

func TestClient_DistanceTo(t *testing.T) {
	config := DefaultConfig()
	config.Dimensionality = 3
	config.HeightMin = 0

	client, err := NewClient(config)
	if err != nil {
		t.Fatal(err)
	}

	// Fiddle a raw coordinate to put it a specific number of seconds away.
	other := NewCoordinate(config)
	other.Vec[2] = 12.345
	expected := time.Duration(other.Vec[2] * secondsToNanoseconds)
	dist := client.DistanceTo(other)
	if dist != expected {
		t.Fatalf("distance doesn't match %9.6f != %9.6f", dist.Seconds(), expected.Seconds())
	}
}

func TestClient_latencyFilter(t *testing.T) {
	config := DefaultConfig()
	config.LatencyFilterSize = 3

	client, err := NewClient(config)
	if err != nil {
		t.Fatal(err)
	}

	// Make sure we get the median, and that things age properly.
	verifyEqualFloats(t, client.latencyFilter("alice", 0.201), 0.201)
	verifyEqualFloats(t, client.latencyFilter("alice", 0.200), 0.201)
	verifyEqualFloats(t, client.latencyFilter("alice", 0.207), 0.201)

	// This glitch will get median-ed out and never seen by Vivaldi.
	verifyEqualFloats(t, client.latencyFilter("alice", 1.9), 0.207)
	verifyEqualFloats(t, client.latencyFilter("alice", 0.203), 0.207)
	verifyEqualFloats(t, client.latencyFilter("alice", 0.199), 0.203)
	verifyEqualFloats(t, client.latencyFilter("alice", 0.211), 0.203)

	// Make sure different nodes are not coupled.
	verifyEqualFloats(t, client.latencyFilter("bob", 0.310), 0.310)

	// Make sure we don't leak coordinates for nodes that leave.
	client.ForgetNode("alice")
	verifyEqualFloats(t, client.latencyFilter("alice", 0.888), 0.888)
}

func TestClient_NaN_Defense(t *testing.T) {
	config := DefaultConfig()
	config.Dimensionality = 3

	client, err := NewClient(config)
	if err != nil {
		t.Fatal(err)
	}

	// Block a bad coordinate from coming in.
	other := NewCoordinate(config)
	other.Vec[0] = math.NaN()
	if other.IsValid() {
		t.Fatalf("bad: %#v", *other)
	}
	rtt := 250 * time.Millisecond
	c, err := client.Update("node", other, rtt)
	if err == nil || !strings.Contains(err.Error(), "coordinate is invalid") {
		t.Fatalf("err: %v", err)
	}
	if c := client.GetCoordinate(); !c.IsValid() {
		t.Fatalf("bad: %#v", *c)
	}

	// Block setting an invalid coordinate directly.
	err = client.SetCoordinate(other)
	if err == nil || !strings.Contains(err.Error(), "coordinate is invalid") {
		t.Fatalf("err: %v", err)
	}
	if c := client.GetCoordinate(); !c.IsValid() {
		t.Fatalf("bad: %#v", *c)
	}

	// Block an incompatible coordinate.
	other.Vec = make([]float64, 2*len(other.Vec))
	c, err = client.Update("node", other, rtt)
	if err == nil || !strings.Contains(err.Error(), "dimensions aren't compatible") {
		t.Fatalf("err: %v", err)
	}
	if c := client.GetCoordinate(); !c.IsValid() {
		t.Fatalf("bad: %#v", *c)
	}

	// Block setting an incompatible coordinate directly.
	err = client.SetCoordinate(other)
	if err == nil || !strings.Contains(err.Error(), "dimensions aren't compatible") {
		t.Fatalf("err: %v", err)
	}
	if c := client.GetCoordinate(); !c.IsValid() {
		t.Fatalf("bad: %#v", *c)
	}

	// Poison the internal state and make sure we reset on an update.
	client.coord.Vec[0] = math.NaN()
	other = NewCoordinate(config)
	c, err = client.Update("node", other, rtt)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !c.IsValid() {
		t.Fatalf("bad: %#v", *c)
	}
	if got, want := client.Stats().Resets, 1; got != want {
		t.Fatalf("got %d want %d", got, want)
	}
}
