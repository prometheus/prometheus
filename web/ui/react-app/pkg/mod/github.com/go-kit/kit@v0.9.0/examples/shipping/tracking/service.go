// Package tracking provides the use-case of tracking a cargo. Used by views
// facing the end-user.
package tracking

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-kit/kit/examples/shipping/cargo"
)

// ErrInvalidArgument is returned when one or more arguments are invalid.
var ErrInvalidArgument = errors.New("invalid argument")

// Service is the interface that provides the basic Track method.
type Service interface {
	// Track returns a cargo matching a tracking ID.
	Track(id string) (Cargo, error)
}

type service struct {
	cargos         cargo.Repository
	handlingEvents cargo.HandlingEventRepository
}

func (s *service) Track(id string) (Cargo, error) {
	if id == "" {
		return Cargo{}, ErrInvalidArgument
	}
	c, err := s.cargos.Find(cargo.TrackingID(id))
	if err != nil {
		return Cargo{}, err
	}
	return assemble(c, s.handlingEvents), nil
}

// NewService returns a new instance of the default Service.
func NewService(cargos cargo.Repository, events cargo.HandlingEventRepository) Service {
	return &service{
		cargos:         cargos,
		handlingEvents: events,
	}
}

// Cargo is a read model for tracking views.
type Cargo struct {
	TrackingID           string    `json:"tracking_id"`
	StatusText           string    `json:"status_text"`
	Origin               string    `json:"origin"`
	Destination          string    `json:"destination"`
	ETA                  time.Time `json:"eta"`
	NextExpectedActivity string    `json:"next_expected_activity"`
	ArrivalDeadline      time.Time `json:"arrival_deadline"`
	Events               []Event   `json:"events"`
}

// Leg is a read model for booking views.
type Leg struct {
	VoyageNumber string    `json:"voyage_number"`
	From         string    `json:"from"`
	To           string    `json:"to"`
	LoadTime     time.Time `json:"load_time"`
	UnloadTime   time.Time `json:"unload_time"`
}

// Event is a read model for tracking views.
type Event struct {
	Description string `json:"description"`
	Expected    bool   `json:"expected"`
}

func assemble(c *cargo.Cargo, events cargo.HandlingEventRepository) Cargo {
	return Cargo{
		TrackingID:           string(c.TrackingID),
		Origin:               string(c.Origin),
		Destination:          string(c.RouteSpecification.Destination),
		ETA:                  c.Delivery.ETA,
		NextExpectedActivity: nextExpectedActivity(c),
		ArrivalDeadline:      c.RouteSpecification.ArrivalDeadline,
		StatusText:           assembleStatusText(c),
		Events:               assembleEvents(c, events),
	}
}

func assembleLegs(c cargo.Cargo) []Leg {
	var legs []Leg
	for _, l := range c.Itinerary.Legs {
		legs = append(legs, Leg{
			VoyageNumber: string(l.VoyageNumber),
			From:         string(l.LoadLocation),
			To:           string(l.UnloadLocation),
			LoadTime:     l.LoadTime,
			UnloadTime:   l.UnloadTime,
		})
	}
	return legs
}

func nextExpectedActivity(c *cargo.Cargo) string {
	a := c.Delivery.NextExpectedActivity
	prefix := "Next expected activity is to"

	switch a.Type {
	case cargo.Load:
		return fmt.Sprintf("%s %s cargo onto voyage %s in %s.", prefix, strings.ToLower(a.Type.String()), a.VoyageNumber, a.Location)
	case cargo.Unload:
		return fmt.Sprintf("%s %s cargo off of voyage %s in %s.", prefix, strings.ToLower(a.Type.String()), a.VoyageNumber, a.Location)
	case cargo.NotHandled:
		return "There are currently no expected activities for this cargo."
	}

	return fmt.Sprintf("%s %s cargo in %s.", prefix, strings.ToLower(a.Type.String()), a.Location)
}

func assembleStatusText(c *cargo.Cargo) string {
	switch c.Delivery.TransportStatus {
	case cargo.NotReceived:
		return "Not received"
	case cargo.InPort:
		return fmt.Sprintf("In port %s", c.Delivery.LastKnownLocation)
	case cargo.OnboardCarrier:
		return fmt.Sprintf("Onboard voyage %s", c.Delivery.CurrentVoyage)
	case cargo.Claimed:
		return "Claimed"
	default:
		return "Unknown"
	}
}

func assembleEvents(c *cargo.Cargo, handlingEvents cargo.HandlingEventRepository) []Event {
	h := handlingEvents.QueryHandlingHistory(c.TrackingID)

	var events []Event
	for _, e := range h.HandlingEvents {
		var description string

		switch e.Activity.Type {
		case cargo.NotHandled:
			description = "Cargo has not yet been received."
		case cargo.Receive:
			description = fmt.Sprintf("Received in %s, at %s", e.Activity.Location, time.Now().Format(time.RFC3339))
		case cargo.Load:
			description = fmt.Sprintf("Loaded onto voyage %s in %s, at %s.", e.Activity.VoyageNumber, e.Activity.Location, time.Now().Format(time.RFC3339))
		case cargo.Unload:
			description = fmt.Sprintf("Unloaded off voyage %s in %s, at %s.", e.Activity.VoyageNumber, e.Activity.Location, time.Now().Format(time.RFC3339))
		case cargo.Claim:
			description = fmt.Sprintf("Claimed in %s, at %s.", e.Activity.Location, time.Now().Format(time.RFC3339))
		case cargo.Customs:
			description = fmt.Sprintf("Cleared customs in %s, at %s.", e.Activity.Location, time.Now().Format(time.RFC3339))
		default:
			description = "[Unknown status]"
		}

		events = append(events, Event{
			Description: description,
			Expected:    c.Itinerary.IsExpected(e),
		})
	}

	return events
}
