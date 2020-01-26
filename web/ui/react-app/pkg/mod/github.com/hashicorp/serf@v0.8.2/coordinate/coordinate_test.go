package coordinate

import (
	"math"
	"reflect"
	"testing"
	"time"
)

// verifyDimensionPanic will run the supplied func and make sure it panics with
// the expected error type.
func verifyDimensionPanic(t *testing.T, f func()) {
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(DimensionalityConflictError); !ok {
				t.Fatalf("panic isn't the right type")
			}
		} else {
			t.Fatalf("didn't get expected panic")
		}
	}()
	f()
}

func TestCoordinate_NewCoordinate(t *testing.T) {
	config := DefaultConfig()
	c := NewCoordinate(config)
	if uint(len(c.Vec)) != config.Dimensionality {
		t.Fatalf("dimensionality not set correctly %d != %d",
			len(c.Vec), config.Dimensionality)
	}
}

func TestCoordinate_Clone(t *testing.T) {
	c := NewCoordinate(DefaultConfig())
	c.Vec[0], c.Vec[1], c.Vec[2] = 1.0, 2.0, 3.0
	c.Error = 5.0
	c.Adjustment = 10.0
	c.Height = 4.2

	other := c.Clone()
	if !reflect.DeepEqual(c, other) {
		t.Fatalf("coordinate clone didn't make a proper copy")
	}

	other.Vec[0] = c.Vec[0] + 0.5
	if reflect.DeepEqual(c, other) {
		t.Fatalf("cloned coordinate is still pointing at its ancestor")
	}
}

func TestCoordinate_IsValid(t *testing.T) {
	c := NewCoordinate(DefaultConfig())

	var fields []*float64
	for i := range c.Vec {
		fields = append(fields, &c.Vec[i])
	}
	fields = append(fields, &c.Error)
	fields = append(fields, &c.Adjustment)
	fields = append(fields, &c.Height)

	for i, field := range fields {
		if !c.IsValid() {
			t.Fatalf("field %d should be valid", i)
		}

		*field = math.NaN()
		if c.IsValid() {
			t.Fatalf("field %d should not be valid (NaN)", i)
		}

		*field = 0.0
		if !c.IsValid() {
			t.Fatalf("field %d should be valid", i)
		}

		*field = math.Inf(0)
		if c.IsValid() {
			t.Fatalf("field %d should not be valid (Inf)", i)
		}

		*field = 0.0
		if !c.IsValid() {
			t.Fatalf("field %d should be valid", i)
		}
	}
}

func TestCoordinate_IsCompatibleWith(t *testing.T) {
	config := DefaultConfig()

	config.Dimensionality = 3
	c1 := NewCoordinate(config)
	c2 := NewCoordinate(config)

	config.Dimensionality = 2
	alien := NewCoordinate(config)

	if !c1.IsCompatibleWith(c1) || !c2.IsCompatibleWith(c2) ||
		!alien.IsCompatibleWith(alien) {
		t.Fatalf("coordinates should be compatible with themselves")
	}

	if !c1.IsCompatibleWith(c2) || !c2.IsCompatibleWith(c1) {
		t.Fatalf("coordinates should be compatible with each other")
	}

	if c1.IsCompatibleWith(alien) || c2.IsCompatibleWith(alien) ||
		alien.IsCompatibleWith(c1) || alien.IsCompatibleWith(c2) {
		t.Fatalf("alien should not be compatible with the other coordinates")
	}
}

func TestCoordinate_ApplyForce(t *testing.T) {
	config := DefaultConfig()
	config.Dimensionality = 3
	config.HeightMin = 0

	origin := NewCoordinate(config)

	// This proves that we normalize, get the direction right, and apply the
	// force multiplier correctly.
	above := NewCoordinate(config)
	above.Vec = []float64{0.0, 0.0, 2.9}
	c := origin.ApplyForce(config, 5.3, above)
	verifyEqualVectors(t, c.Vec, []float64{0.0, 0.0, -5.3})

	// Scoot a point not starting at the origin to make sure there's nothing
	// special there.
	right := NewCoordinate(config)
	right.Vec = []float64{3.4, 0.0, -5.3}
	c = c.ApplyForce(config, 2.0, right)
	verifyEqualVectors(t, c.Vec, []float64{-2.0, 0.0, -5.3})

	// If the points are right on top of each other, then we should end up
	// in a random direction, one unit away. This makes sure the unit vector
	// build up doesn't divide by zero.
	c = origin.ApplyForce(config, 1.0, origin)
	verifyEqualFloats(t, origin.DistanceTo(c).Seconds(), 1.0)

	// Enable a minimum height and make sure that gets factored in properly.
	config.HeightMin = 10.0e-6
	origin = NewCoordinate(config)
	c = origin.ApplyForce(config, 5.3, above)
	verifyEqualVectors(t, c.Vec, []float64{0.0, 0.0, -5.3})
	verifyEqualFloats(t, c.Height, config.HeightMin+5.3*config.HeightMin/2.9)

	// Make sure the height minimum is enforced.
	c = origin.ApplyForce(config, -5.3, above)
	verifyEqualVectors(t, c.Vec, []float64{0.0, 0.0, 5.3})
	verifyEqualFloats(t, c.Height, config.HeightMin)

	// Shenanigans should get called if the dimensions don't match.
	bad := c.Clone()
	bad.Vec = make([]float64, len(bad.Vec)+1)
	verifyDimensionPanic(t, func() { c.ApplyForce(config, 1.0, bad) })
}

func TestCoordinate_DistanceTo(t *testing.T) {
	config := DefaultConfig()
	config.Dimensionality = 3
	config.HeightMin = 0

	c1, c2 := NewCoordinate(config), NewCoordinate(config)
	c1.Vec = []float64{-0.5, 1.3, 2.4}
	c2.Vec = []float64{1.2, -2.3, 3.4}

	verifyEqualFloats(t, c1.DistanceTo(c1).Seconds(), 0.0)
	verifyEqualFloats(t, c1.DistanceTo(c2).Seconds(), c2.DistanceTo(c1).Seconds())
	verifyEqualFloats(t, c1.DistanceTo(c2).Seconds(), 4.104875150354758)

	// Make sure negative adjustment factors are ignored.
	c1.Adjustment = -1.0e6
	verifyEqualFloats(t, c1.DistanceTo(c2).Seconds(), 4.104875150354758)

	// Make sure positive adjustment factors affect the distance.
	c1.Adjustment = 0.1
	c2.Adjustment = 0.2
	verifyEqualFloats(t, c1.DistanceTo(c2).Seconds(), 4.104875150354758+0.3)

	// Make sure the heights affect the distance.
	c1.Height = 0.7
	c2.Height = 0.1
	verifyEqualFloats(t, c1.DistanceTo(c2).Seconds(), 4.104875150354758+0.3+0.8)

	// Shenanigans should get called if the dimensions don't match.
	bad := c1.Clone()
	bad.Vec = make([]float64, len(bad.Vec)+1)
	verifyDimensionPanic(t, func() { _ = c1.DistanceTo(bad) })
}

// dist is a self-contained example that appears in documentation.
func dist(a *Coordinate, b *Coordinate) time.Duration {
	// Coordinates will always have the same dimensionality, so this is
	// just a sanity check.
	if len(a.Vec) != len(b.Vec) {
		panic("dimensions aren't compatible")
	}

	// Calculate the Euclidean distance plus the heights.
	sumsq := 0.0
	for i := 0; i < len(a.Vec); i++ {
		diff := a.Vec[i] - b.Vec[i]
		sumsq += diff * diff
	}
	rtt := math.Sqrt(sumsq) + a.Height + b.Height

	// Apply the adjustment components, guarding against negatives.
	adjusted := rtt + a.Adjustment + b.Adjustment
	if adjusted > 0.0 {
		rtt = adjusted
	}

	// Go's times are natively nanoseconds, so we convert from seconds.
	const secondsToNanoseconds = 1.0e9
	return time.Duration(rtt * secondsToNanoseconds)
}

func TestCoordinate_dist_Example(t *testing.T) {
	config := DefaultConfig()
	c1, c2 := NewCoordinate(config), NewCoordinate(config)
	c1.Vec = []float64{-0.5, 1.3, 2.4}
	c2.Vec = []float64{1.2, -2.3, 3.4}
	c1.Adjustment = 0.1
	c2.Adjustment = 0.2
	c1.Height = 0.7
	c2.Height = 0.1
	verifyEqualFloats(t, c1.DistanceTo(c2).Seconds(), dist(c1, c2).Seconds())
}

func TestCoordinate_rawDistanceTo(t *testing.T) {
	config := DefaultConfig()
	config.Dimensionality = 3
	config.HeightMin = 0

	c1, c2 := NewCoordinate(config), NewCoordinate(config)
	c1.Vec = []float64{-0.5, 1.3, 2.4}
	c2.Vec = []float64{1.2, -2.3, 3.4}

	verifyEqualFloats(t, c1.rawDistanceTo(c1), 0.0)
	verifyEqualFloats(t, c1.rawDistanceTo(c2), c2.rawDistanceTo(c1))
	verifyEqualFloats(t, c1.rawDistanceTo(c2), 4.104875150354758)

	// Make sure that the adjustment doesn't factor into the raw
	// distance.
	c1.Adjustment = 1.0e6
	verifyEqualFloats(t, c1.rawDistanceTo(c2), 4.104875150354758)

	// Make sure the heights affect the distance.
	c1.Height = 0.7
	c2.Height = 0.1
	verifyEqualFloats(t, c1.rawDistanceTo(c2), 4.104875150354758+0.8)
}

func TestCoordinate_add(t *testing.T) {
	vec1 := []float64{1.0, -3.0, 3.0}
	vec2 := []float64{-4.0, 5.0, 6.0}
	verifyEqualVectors(t, add(vec1, vec2), []float64{-3.0, 2.0, 9.0})

	zero := []float64{0.0, 0.0, 0.0}
	verifyEqualVectors(t, add(vec1, zero), vec1)
}

func TestCoordinate_diff(t *testing.T) {
	vec1 := []float64{1.0, -3.0, 3.0}
	vec2 := []float64{-4.0, 5.0, 6.0}
	verifyEqualVectors(t, diff(vec1, vec2), []float64{5.0, -8.0, -3.0})

	zero := []float64{0.0, 0.0, 0.0}
	verifyEqualVectors(t, diff(vec1, zero), vec1)
}

func TestCoordinate_magnitude(t *testing.T) {
	zero := []float64{0.0, 0.0, 0.0}
	verifyEqualFloats(t, magnitude(zero), 0.0)

	vec := []float64{1.0, -2.0, 3.0}
	verifyEqualFloats(t, magnitude(vec), 3.7416573867739413)
}

func TestCoordinate_unitVectorAt(t *testing.T) {
	vec1 := []float64{1.0, 2.0, 3.0}
	vec2 := []float64{0.5, 0.6, 0.7}
	u, mag := unitVectorAt(vec1, vec2)
	verifyEqualVectors(t, u, []float64{0.18257418583505536, 0.511207720338155, 0.8398412548412546})
	verifyEqualFloats(t, magnitude(u), 1.0)
	verifyEqualFloats(t, mag, magnitude(diff(vec1, vec2)))

	// If we give positions that are equal we should get a random unit vector
	// returned to us, rather than a divide by zero.
	u, mag = unitVectorAt(vec1, vec1)
	verifyEqualFloats(t, magnitude(u), 1.0)
	verifyEqualFloats(t, mag, 0.0)

	// We can't hit the final clause without heroics so I manually forced it
	// there to verify it works.
}
