// Package inspection provides means to inspect cargos.
package inspection

import (
	"github.com/go-kit/kit/examples/shipping/cargo"
)

// EventHandler provides means of subscribing to inspection events.
type EventHandler interface {
	CargoWasMisdirected(*cargo.Cargo)
	CargoHasArrived(*cargo.Cargo)
}

// Service provides cargo inspection operations.
type Service interface {
	// InspectCargo inspects cargo and send relevant notifications to
	// interested parties, for example if a cargo has been misdirected, or
	// unloaded at the final destination.
	InspectCargo(id cargo.TrackingID)
}

type service struct {
	cargos  cargo.Repository
	events  cargo.HandlingEventRepository
	handler EventHandler
}

// TODO: Should be transactional
func (s *service) InspectCargo(id cargo.TrackingID) {
	c, err := s.cargos.Find(id)
	if err != nil {
		return
	}

	h := s.events.QueryHandlingHistory(id)

	c.DeriveDeliveryProgress(h)

	if c.Delivery.IsMisdirected {
		s.handler.CargoWasMisdirected(c)
	}

	if c.Delivery.IsUnloadedAtDestination {
		s.handler.CargoHasArrived(c)
	}

	s.cargos.Store(c)
}

// NewService creates a inspection service with necessary dependencies.
func NewService(cargos cargo.Repository, events cargo.HandlingEventRepository, handler EventHandler) Service {
	return &service{cargos, events, handler}
}
