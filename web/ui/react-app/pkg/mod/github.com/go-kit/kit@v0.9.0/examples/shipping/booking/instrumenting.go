package booking

import (
	"time"

	"github.com/go-kit/kit/metrics"

	"github.com/go-kit/kit/examples/shipping/cargo"
	"github.com/go-kit/kit/examples/shipping/location"
)

type instrumentingService struct {
	requestCount   metrics.Counter
	requestLatency metrics.Histogram
	Service
}

// NewInstrumentingService returns an instance of an instrumenting Service.
func NewInstrumentingService(counter metrics.Counter, latency metrics.Histogram, s Service) Service {
	return &instrumentingService{
		requestCount:   counter,
		requestLatency: latency,
		Service:        s,
	}
}

func (s *instrumentingService) BookNewCargo(origin, destination location.UNLocode, deadline time.Time) (cargo.TrackingID, error) {
	defer func(begin time.Time) {
		s.requestCount.With("method", "book").Add(1)
		s.requestLatency.With("method", "book").Observe(time.Since(begin).Seconds())
	}(time.Now())

	return s.Service.BookNewCargo(origin, destination, deadline)
}

func (s *instrumentingService) LoadCargo(id cargo.TrackingID) (c Cargo, err error) {
	defer func(begin time.Time) {
		s.requestCount.With("method", "load").Add(1)
		s.requestLatency.With("method", "load").Observe(time.Since(begin).Seconds())
	}(time.Now())

	return s.Service.LoadCargo(id)
}

func (s *instrumentingService) RequestPossibleRoutesForCargo(id cargo.TrackingID) []cargo.Itinerary {
	defer func(begin time.Time) {
		s.requestCount.With("method", "request_routes").Add(1)
		s.requestLatency.With("method", "request_routes").Observe(time.Since(begin).Seconds())
	}(time.Now())

	return s.Service.RequestPossibleRoutesForCargo(id)
}

func (s *instrumentingService) AssignCargoToRoute(id cargo.TrackingID, itinerary cargo.Itinerary) (err error) {
	defer func(begin time.Time) {
		s.requestCount.With("method", "assign_to_route").Add(1)
		s.requestLatency.With("method", "assign_to_route").Observe(time.Since(begin).Seconds())
	}(time.Now())

	return s.Service.AssignCargoToRoute(id, itinerary)
}

func (s *instrumentingService) ChangeDestination(id cargo.TrackingID, l location.UNLocode) (err error) {
	defer func(begin time.Time) {
		s.requestCount.With("method", "change_destination").Add(1)
		s.requestLatency.With("method", "change_destination").Observe(time.Since(begin).Seconds())
	}(time.Now())

	return s.Service.ChangeDestination(id, l)
}

func (s *instrumentingService) Cargos() []Cargo {
	defer func(begin time.Time) {
		s.requestCount.With("method", "list_cargos").Add(1)
		s.requestLatency.With("method", "list_cargos").Observe(time.Since(begin).Seconds())
	}(time.Now())

	return s.Service.Cargos()
}

func (s *instrumentingService) Locations() []Location {
	defer func(begin time.Time) {
		s.requestCount.With("method", "list_locations").Add(1)
		s.requestLatency.With("method", "list_locations").Observe(time.Since(begin).Seconds())
	}(time.Now())

	return s.Service.Locations()
}
