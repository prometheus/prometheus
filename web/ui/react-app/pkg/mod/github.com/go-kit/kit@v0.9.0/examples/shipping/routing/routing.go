// Package routing provides the routing domain service. It does not actually
// implement the routing service but merely acts as a proxy for a separate
// bounded context.
package routing

import (
	"github.com/go-kit/kit/examples/shipping/cargo"
)

// Service provides access to an external routing service.
type Service interface {
	// FetchRoutesForSpecification finds all possible routes that satisfy a
	// given specification.
	FetchRoutesForSpecification(rs cargo.RouteSpecification) []cargo.Itinerary
}
