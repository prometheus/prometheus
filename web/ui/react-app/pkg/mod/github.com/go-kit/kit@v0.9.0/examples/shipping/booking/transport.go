package booking

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/gorilla/mux"

	kitlog "github.com/go-kit/kit/log"
	"github.com/go-kit/kit/transport"
	kithttp "github.com/go-kit/kit/transport/http"

	"github.com/go-kit/kit/examples/shipping/cargo"
	"github.com/go-kit/kit/examples/shipping/location"
)

// MakeHandler returns a handler for the booking service.
func MakeHandler(bs Service, logger kitlog.Logger) http.Handler {
	opts := []kithttp.ServerOption{
		kithttp.ServerErrorHandler(transport.NewLogErrorHandler(logger)),
		kithttp.ServerErrorEncoder(encodeError),
	}

	bookCargoHandler := kithttp.NewServer(
		makeBookCargoEndpoint(bs),
		decodeBookCargoRequest,
		encodeResponse,
		opts...,
	)
	loadCargoHandler := kithttp.NewServer(
		makeLoadCargoEndpoint(bs),
		decodeLoadCargoRequest,
		encodeResponse,
		opts...,
	)
	requestRoutesHandler := kithttp.NewServer(
		makeRequestRoutesEndpoint(bs),
		decodeRequestRoutesRequest,
		encodeResponse,
		opts...,
	)
	assignToRouteHandler := kithttp.NewServer(
		makeAssignToRouteEndpoint(bs),
		decodeAssignToRouteRequest,
		encodeResponse,
		opts...,
	)
	changeDestinationHandler := kithttp.NewServer(
		makeChangeDestinationEndpoint(bs),
		decodeChangeDestinationRequest,
		encodeResponse,
		opts...,
	)
	listCargosHandler := kithttp.NewServer(
		makeListCargosEndpoint(bs),
		decodeListCargosRequest,
		encodeResponse,
		opts...,
	)
	listLocationsHandler := kithttp.NewServer(
		makeListLocationsEndpoint(bs),
		decodeListLocationsRequest,
		encodeResponse,
		opts...,
	)

	r := mux.NewRouter()

	r.Handle("/booking/v1/cargos", bookCargoHandler).Methods("POST")
	r.Handle("/booking/v1/cargos", listCargosHandler).Methods("GET")
	r.Handle("/booking/v1/cargos/{id}", loadCargoHandler).Methods("GET")
	r.Handle("/booking/v1/cargos/{id}/request_routes", requestRoutesHandler).Methods("GET")
	r.Handle("/booking/v1/cargos/{id}/assign_to_route", assignToRouteHandler).Methods("POST")
	r.Handle("/booking/v1/cargos/{id}/change_destination", changeDestinationHandler).Methods("POST")
	r.Handle("/booking/v1/locations", listLocationsHandler).Methods("GET")

	return r
}

var errBadRoute = errors.New("bad route")

func decodeBookCargoRequest(_ context.Context, r *http.Request) (interface{}, error) {
	var body struct {
		Origin          string    `json:"origin"`
		Destination     string    `json:"destination"`
		ArrivalDeadline time.Time `json:"arrival_deadline"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, err
	}

	return bookCargoRequest{
		Origin:          location.UNLocode(body.Origin),
		Destination:     location.UNLocode(body.Destination),
		ArrivalDeadline: body.ArrivalDeadline,
	}, nil
}

func decodeLoadCargoRequest(_ context.Context, r *http.Request) (interface{}, error) {
	vars := mux.Vars(r)
	id, ok := vars["id"]
	if !ok {
		return nil, errBadRoute
	}
	return loadCargoRequest{ID: cargo.TrackingID(id)}, nil
}

func decodeRequestRoutesRequest(_ context.Context, r *http.Request) (interface{}, error) {
	vars := mux.Vars(r)
	id, ok := vars["id"]
	if !ok {
		return nil, errBadRoute
	}
	return requestRoutesRequest{ID: cargo.TrackingID(id)}, nil
}

func decodeAssignToRouteRequest(_ context.Context, r *http.Request) (interface{}, error) {
	vars := mux.Vars(r)
	id, ok := vars["id"]
	if !ok {
		return nil, errBadRoute
	}

	var itinerary cargo.Itinerary
	if err := json.NewDecoder(r.Body).Decode(&itinerary); err != nil {
		return nil, err
	}

	return assignToRouteRequest{
		ID:        cargo.TrackingID(id),
		Itinerary: itinerary,
	}, nil
}

func decodeChangeDestinationRequest(_ context.Context, r *http.Request) (interface{}, error) {
	vars := mux.Vars(r)
	id, ok := vars["id"]
	if !ok {
		return nil, errBadRoute
	}

	var body struct {
		Destination string `json:"destination"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, err
	}

	return changeDestinationRequest{
		ID:          cargo.TrackingID(id),
		Destination: location.UNLocode(body.Destination),
	}, nil
}

func decodeListCargosRequest(_ context.Context, r *http.Request) (interface{}, error) {
	return listCargosRequest{}, nil
}

func decodeListLocationsRequest(_ context.Context, r *http.Request) (interface{}, error) {
	return listLocationsRequest{}, nil
}

func encodeResponse(ctx context.Context, w http.ResponseWriter, response interface{}) error {
	if e, ok := response.(errorer); ok && e.error() != nil {
		encodeError(ctx, e.error(), w)
		return nil
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	return json.NewEncoder(w).Encode(response)
}

type errorer interface {
	error() error
}

// encode errors from business-logic
func encodeError(_ context.Context, err error, w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	switch err {
	case cargo.ErrUnknown:
		w.WriteHeader(http.StatusNotFound)
	case ErrInvalidArgument:
		w.WriteHeader(http.StatusBadRequest)
	default:
		w.WriteHeader(http.StatusInternalServerError)
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": err.Error(),
	})
}
