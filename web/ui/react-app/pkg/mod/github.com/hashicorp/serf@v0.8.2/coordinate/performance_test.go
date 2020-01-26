package coordinate

import (
	"math"
	"testing"
	"time"
)

func TestPerformance_Line(t *testing.T) {
	const spacing = 10 * time.Millisecond
	const nodes, cycles = 10, 1000
	config := DefaultConfig()
	clients, err := GenerateClients(nodes, config)
	if err != nil {
		t.Fatal(err)
	}
	truth := GenerateLine(nodes, spacing)
	Simulate(clients, truth, cycles)
	stats := Evaluate(clients, truth)
	if stats.ErrorAvg > 0.0018 || stats.ErrorMax > 0.0092 {
		t.Fatalf("performance stats are out of spec: %v", stats)
	}
}

func TestPerformance_Grid(t *testing.T) {
	const spacing = 10 * time.Millisecond
	const nodes, cycles = 25, 1000
	config := DefaultConfig()
	clients, err := GenerateClients(nodes, config)
	if err != nil {
		t.Fatal(err)
	}
	truth := GenerateGrid(nodes, spacing)
	Simulate(clients, truth, cycles)
	stats := Evaluate(clients, truth)
	if stats.ErrorAvg > 0.0015 || stats.ErrorMax > 0.022 {
		t.Fatalf("performance stats are out of spec: %v", stats)
	}
}

func TestPerformance_Split(t *testing.T) {
	const lan, wan = 1 * time.Millisecond, 10 * time.Millisecond
	const nodes, cycles = 25, 1000
	config := DefaultConfig()
	clients, err := GenerateClients(nodes, config)
	if err != nil {
		t.Fatal(err)
	}
	truth := GenerateSplit(nodes, lan, wan)
	Simulate(clients, truth, cycles)
	stats := Evaluate(clients, truth)
	if stats.ErrorAvg > 0.000060 || stats.ErrorMax > 0.00048 {
		t.Fatalf("performance stats are out of spec: %v", stats)
	}
}

func TestPerformance_Height(t *testing.T) {
	const radius = 100 * time.Millisecond
	const nodes, cycles = 25, 1000

	// Constrain us to two dimensions so that we can just exactly represent
	// the circle.
	config := DefaultConfig()
	config.Dimensionality = 2
	clients, err := GenerateClients(nodes, config)
	if err != nil {
		t.Fatal(err)
	}

	// Generate truth where the first coordinate is in the "middle" because
	// it's equidistant from all the nodes, but it will have an extra radius
	// added to the distance, so it should come out above all the others.
	truth := GenerateCircle(nodes, radius)
	Simulate(clients, truth, cycles)

	// Make sure the height looks reasonable with the regular nodes all in a
	// plane, and the center node up above.
	for i, _ := range clients {
		coord := clients[i].GetCoordinate()
		if i == 0 {
			if coord.Height < 0.97*radius.Seconds() {
				t.Fatalf("height is out of spec: %9.6f", coord.Height)
			}
		} else {
			if coord.Height > 0.03*radius.Seconds() {
				t.Fatalf("height is out of spec: %9.6f", coord.Height)
			}
		}
	}
	stats := Evaluate(clients, truth)
	if stats.ErrorAvg > 0.0025 || stats.ErrorMax > 0.064 {
		t.Fatalf("performance stats are out of spec: %v", stats)
	}
}

func TestPerformance_Drift(t *testing.T) {
	const dist = 500 * time.Millisecond
	const nodes = 4
	config := DefaultConfig()
	config.Dimensionality = 2
	clients, err := GenerateClients(nodes, config)
	if err != nil {
		t.Fatal(err)
	}

	// Do some icky surgery on the clients to put them into a square, up in
	// the first quadrant.
	clients[0].coord.Vec = []float64{0.0, 0.0}
	clients[1].coord.Vec = []float64{0.0, dist.Seconds()}
	clients[2].coord.Vec = []float64{dist.Seconds(), dist.Seconds()}
	clients[3].coord.Vec = []float64{dist.Seconds(), dist.Seconds()}

	// Make a corresponding truth matrix. The nodes are laid out like this
	// so the distances are all equal, except for the diagonal:
	//
	// (1)  <- dist ->  (2)
	//
	//  | <- dist        |
	//  |                |
	//  |        dist -> |
	//
	// (0)  <- dist ->  (3)
	//
	truth := make([][]time.Duration, nodes)
	for i := range truth {
		truth[i] = make([]time.Duration, nodes)
	}
	for i := 0; i < nodes; i++ {
		for j := i + 1; j < nodes; j++ {
			rtt := dist
			if (i%2 == 0) && (j%2 == 0) {
				rtt = time.Duration(math.Sqrt2 * float64(rtt))
			}
			truth[i][j], truth[j][i] = rtt, rtt
		}
	}

	calcCenterError := func() float64 {
		min, max := clients[0].GetCoordinate(), clients[0].GetCoordinate()
		for i := 1; i < nodes; i++ {
			coord := clients[i].GetCoordinate()
			for j, v := range coord.Vec {
				min.Vec[j] = math.Min(min.Vec[j], v)
				max.Vec[j] = math.Max(max.Vec[j], v)
			}
		}

		mid := make([]float64, config.Dimensionality)
		for i, _ := range mid {
			mid[i] = min.Vec[i] + (max.Vec[i]-min.Vec[i])/2
		}
		return magnitude(mid)
	}

	// Let the simulation run for a while to stabilize, then snap a baseline
	// for the center error.
	Simulate(clients, truth, 1000)
	baseline := calcCenterError()

	// Now run for a bunch more cycles and see if gravity pulls the center
	// in the right direction.
	Simulate(clients, truth, 10000)
	if error := calcCenterError(); error > 0.8*baseline {
		t.Fatalf("drift performance out of spec: %9.6f -> %9.6f", baseline, error)
	}
}

func TestPerformance_Random(t *testing.T) {
	const mean, deviation = 100 * time.Millisecond, 10 * time.Millisecond
	const nodes, cycles = 25, 1000
	config := DefaultConfig()
	clients, err := GenerateClients(nodes, config)
	if err != nil {
		t.Fatal(err)
	}
	truth := GenerateRandom(nodes, mean, deviation)
	Simulate(clients, truth, cycles)
	stats := Evaluate(clients, truth)
	if stats.ErrorAvg > 0.075 || stats.ErrorMax > 0.33 {
		t.Fatalf("performance stats are out of spec: %v", stats)
	}
}
