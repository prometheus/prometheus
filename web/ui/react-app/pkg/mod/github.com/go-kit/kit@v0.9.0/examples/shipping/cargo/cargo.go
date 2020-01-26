// Package cargo contains the heart of the domain model.
package cargo

import (
	"errors"
	"strings"
	"time"

	"github.com/pborman/uuid"

	"github.com/go-kit/kit/examples/shipping/location"
)

// TrackingID uniquely identifies a particular cargo.
type TrackingID string

// Cargo is the central class in the domain model.
type Cargo struct {
	TrackingID         TrackingID
	Origin             location.UNLocode
	RouteSpecification RouteSpecification
	Itinerary          Itinerary
	Delivery           Delivery
}

// SpecifyNewRoute specifies a new route for this cargo.
func (c *Cargo) SpecifyNewRoute(rs RouteSpecification) {
	c.RouteSpecification = rs
	c.Delivery = c.Delivery.UpdateOnRouting(c.RouteSpecification, c.Itinerary)
}

// AssignToRoute attaches a new itinerary to this cargo.
func (c *Cargo) AssignToRoute(itinerary Itinerary) {
	c.Itinerary = itinerary
	c.Delivery = c.Delivery.UpdateOnRouting(c.RouteSpecification, c.Itinerary)
}

// DeriveDeliveryProgress updates all aspects of the cargo aggregate status
// based on the current route specification, itinerary and handling of the cargo.
func (c *Cargo) DeriveDeliveryProgress(history HandlingHistory) {
	c.Delivery = DeriveDeliveryFrom(c.RouteSpecification, c.Itinerary, history)
}

// New creates a new, unrouted cargo.
func New(id TrackingID, rs RouteSpecification) *Cargo {
	itinerary := Itinerary{}
	history := HandlingHistory{make([]HandlingEvent, 0)}

	return &Cargo{
		TrackingID:         id,
		Origin:             rs.Origin,
		RouteSpecification: rs,
		Delivery:           DeriveDeliveryFrom(rs, itinerary, history),
	}
}

// Repository provides access a cargo store.
type Repository interface {
	Store(cargo *Cargo) error
	Find(id TrackingID) (*Cargo, error)
	FindAll() []*Cargo
}

// ErrUnknown is used when a cargo could not be found.
var ErrUnknown = errors.New("unknown cargo")

// NextTrackingID generates a new tracking ID.
// TODO: Move to infrastructure(?)
func NextTrackingID() TrackingID {
	return TrackingID(strings.Split(strings.ToUpper(uuid.New()), "-")[0])
}

// RouteSpecification Contains information about a route: its origin,
// destination and arrival deadline.
type RouteSpecification struct {
	Origin          location.UNLocode
	Destination     location.UNLocode
	ArrivalDeadline time.Time
}

// IsSatisfiedBy checks whether provided itinerary satisfies this
// specification.
func (s RouteSpecification) IsSatisfiedBy(itinerary Itinerary) bool {
	return itinerary.Legs != nil &&
		s.Origin == itinerary.InitialDepartureLocation() &&
		s.Destination == itinerary.FinalArrivalLocation()
}

// RoutingStatus describes status of cargo routing.
type RoutingStatus int

// Valid routing statuses.
const (
	NotRouted RoutingStatus = iota
	Misrouted
	Routed
)

func (s RoutingStatus) String() string {
	switch s {
	case NotRouted:
		return "Not routed"
	case Misrouted:
		return "Misrouted"
	case Routed:
		return "Routed"
	}
	return ""
}

// TransportStatus describes status of cargo transportation.
type TransportStatus int

// Valid transport statuses.
const (
	NotReceived TransportStatus = iota
	InPort
	OnboardCarrier
	Claimed
	Unknown
)

func (s TransportStatus) String() string {
	switch s {
	case NotReceived:
		return "Not received"
	case InPort:
		return "In port"
	case OnboardCarrier:
		return "Onboard carrier"
	case Claimed:
		return "Claimed"
	case Unknown:
		return "Unknown"
	}
	return ""
}
