package profilesvc

// The profilesvc is just over HTTP, so we just have a single transport.go.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/gorilla/mux"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/transport"
	httptransport "github.com/go-kit/kit/transport/http"
)

var (
	// ErrBadRouting is returned when an expected path variable is missing.
	// It always indicates programmer error.
	ErrBadRouting = errors.New("inconsistent mapping between route and handler (programmer error)")
)

// MakeHTTPHandler mounts all of the service endpoints into an http.Handler.
// Useful in a profilesvc server.
func MakeHTTPHandler(s Service, logger log.Logger) http.Handler {
	r := mux.NewRouter()
	e := MakeServerEndpoints(s)
	options := []httptransport.ServerOption{
		httptransport.ServerErrorHandler(transport.NewLogErrorHandler(logger)),
		httptransport.ServerErrorEncoder(encodeError),
	}

	// POST    /profiles/                          adds another profile
	// GET     /profiles/:id                       retrieves the given profile by id
	// PUT     /profiles/:id                       post updated profile information about the profile
	// PATCH   /profiles/:id                       partial updated profile information
	// DELETE  /profiles/:id                       remove the given profile
	// GET     /profiles/:id/addresses/            retrieve addresses associated with the profile
	// GET     /profiles/:id/addresses/:addressID  retrieve a particular profile address
	// POST    /profiles/:id/addresses/            add a new address
	// DELETE  /profiles/:id/addresses/:addressID  remove an address

	r.Methods("POST").Path("/profiles/").Handler(httptransport.NewServer(
		e.PostProfileEndpoint,
		decodePostProfileRequest,
		encodeResponse,
		options...,
	))
	r.Methods("GET").Path("/profiles/{id}").Handler(httptransport.NewServer(
		e.GetProfileEndpoint,
		decodeGetProfileRequest,
		encodeResponse,
		options...,
	))
	r.Methods("PUT").Path("/profiles/{id}").Handler(httptransport.NewServer(
		e.PutProfileEndpoint,
		decodePutProfileRequest,
		encodeResponse,
		options...,
	))
	r.Methods("PATCH").Path("/profiles/{id}").Handler(httptransport.NewServer(
		e.PatchProfileEndpoint,
		decodePatchProfileRequest,
		encodeResponse,
		options...,
	))
	r.Methods("DELETE").Path("/profiles/{id}").Handler(httptransport.NewServer(
		e.DeleteProfileEndpoint,
		decodeDeleteProfileRequest,
		encodeResponse,
		options...,
	))
	r.Methods("GET").Path("/profiles/{id}/addresses/").Handler(httptransport.NewServer(
		e.GetAddressesEndpoint,
		decodeGetAddressesRequest,
		encodeResponse,
		options...,
	))
	r.Methods("GET").Path("/profiles/{id}/addresses/{addressID}").Handler(httptransport.NewServer(
		e.GetAddressEndpoint,
		decodeGetAddressRequest,
		encodeResponse,
		options...,
	))
	r.Methods("POST").Path("/profiles/{id}/addresses/").Handler(httptransport.NewServer(
		e.PostAddressEndpoint,
		decodePostAddressRequest,
		encodeResponse,
		options...,
	))
	r.Methods("DELETE").Path("/profiles/{id}/addresses/{addressID}").Handler(httptransport.NewServer(
		e.DeleteAddressEndpoint,
		decodeDeleteAddressRequest,
		encodeResponse,
		options...,
	))
	return r
}

func decodePostProfileRequest(_ context.Context, r *http.Request) (request interface{}, err error) {
	var req postProfileRequest
	if e := json.NewDecoder(r.Body).Decode(&req.Profile); e != nil {
		return nil, e
	}
	return req, nil
}

func decodeGetProfileRequest(_ context.Context, r *http.Request) (request interface{}, err error) {
	vars := mux.Vars(r)
	id, ok := vars["id"]
	if !ok {
		return nil, ErrBadRouting
	}
	return getProfileRequest{ID: id}, nil
}

func decodePutProfileRequest(_ context.Context, r *http.Request) (request interface{}, err error) {
	vars := mux.Vars(r)
	id, ok := vars["id"]
	if !ok {
		return nil, ErrBadRouting
	}
	var profile Profile
	if err := json.NewDecoder(r.Body).Decode(&profile); err != nil {
		return nil, err
	}
	return putProfileRequest{
		ID:      id,
		Profile: profile,
	}, nil
}

func decodePatchProfileRequest(_ context.Context, r *http.Request) (request interface{}, err error) {
	vars := mux.Vars(r)
	id, ok := vars["id"]
	if !ok {
		return nil, ErrBadRouting
	}
	var profile Profile
	if err := json.NewDecoder(r.Body).Decode(&profile); err != nil {
		return nil, err
	}
	return patchProfileRequest{
		ID:      id,
		Profile: profile,
	}, nil
}

func decodeDeleteProfileRequest(_ context.Context, r *http.Request) (request interface{}, err error) {
	vars := mux.Vars(r)
	id, ok := vars["id"]
	if !ok {
		return nil, ErrBadRouting
	}
	return deleteProfileRequest{ID: id}, nil
}

func decodeGetAddressesRequest(_ context.Context, r *http.Request) (request interface{}, err error) {
	vars := mux.Vars(r)
	id, ok := vars["id"]
	if !ok {
		return nil, ErrBadRouting
	}
	return getAddressesRequest{ProfileID: id}, nil
}

func decodeGetAddressRequest(_ context.Context, r *http.Request) (request interface{}, err error) {
	vars := mux.Vars(r)
	id, ok := vars["id"]
	if !ok {
		return nil, ErrBadRouting
	}
	addressID, ok := vars["addressID"]
	if !ok {
		return nil, ErrBadRouting
	}
	return getAddressRequest{
		ProfileID: id,
		AddressID: addressID,
	}, nil
}

func decodePostAddressRequest(_ context.Context, r *http.Request) (request interface{}, err error) {
	vars := mux.Vars(r)
	id, ok := vars["id"]
	if !ok {
		return nil, ErrBadRouting
	}
	var address Address
	if err := json.NewDecoder(r.Body).Decode(&address); err != nil {
		return nil, err
	}
	return postAddressRequest{
		ProfileID: id,
		Address:   address,
	}, nil
}

func decodeDeleteAddressRequest(_ context.Context, r *http.Request) (request interface{}, err error) {
	vars := mux.Vars(r)
	id, ok := vars["id"]
	if !ok {
		return nil, ErrBadRouting
	}
	addressID, ok := vars["addressID"]
	if !ok {
		return nil, ErrBadRouting
	}
	return deleteAddressRequest{
		ProfileID: id,
		AddressID: addressID,
	}, nil
}

func encodePostProfileRequest(ctx context.Context, req *http.Request, request interface{}) error {
	// r.Methods("POST").Path("/profiles/")
	req.URL.Path = "/profiles/"
	return encodeRequest(ctx, req, request)
}

func encodeGetProfileRequest(ctx context.Context, req *http.Request, request interface{}) error {
	// r.Methods("GET").Path("/profiles/{id}")
	r := request.(getProfileRequest)
	profileID := url.QueryEscape(r.ID)
	req.URL.Path = "/profiles/" + profileID
	return encodeRequest(ctx, req, request)
}

func encodePutProfileRequest(ctx context.Context, req *http.Request, request interface{}) error {
	// r.Methods("PUT").Path("/profiles/{id}")
	r := request.(putProfileRequest)
	profileID := url.QueryEscape(r.ID)
	req.URL.Path = "/profiles/" + profileID
	return encodeRequest(ctx, req, request)
}

func encodePatchProfileRequest(ctx context.Context, req *http.Request, request interface{}) error {
	// r.Methods("PATCH").Path("/profiles/{id}")
	r := request.(patchProfileRequest)
	profileID := url.QueryEscape(r.ID)
	req.URL.Path = "/profiles/" + profileID
	return encodeRequest(ctx, req, request)
}

func encodeDeleteProfileRequest(ctx context.Context, req *http.Request, request interface{}) error {
	// r.Methods("DELETE").Path("/profiles/{id}")
	r := request.(deleteProfileRequest)
	profileID := url.QueryEscape(r.ID)
	req.URL.Path = "/profiles/" + profileID
	return encodeRequest(ctx, req, request)
}

func encodeGetAddressesRequest(ctx context.Context, req *http.Request, request interface{}) error {
	// r.Methods("GET").Path("/profiles/{id}/addresses/")
	r := request.(getAddressesRequest)
	profileID := url.QueryEscape(r.ProfileID)
	req.URL.Path = "/profiles/" + profileID + "/addresses/"
	return encodeRequest(ctx, req, request)
}

func encodeGetAddressRequest(ctx context.Context, req *http.Request, request interface{}) error {
	// r.Methods("GET").Path("/profiles/{id}/addresses/{addressID}")
	r := request.(getAddressRequest)
	profileID := url.QueryEscape(r.ProfileID)
	addressID := url.QueryEscape(r.AddressID)
	req.URL.Path = "/profiles/" + profileID + "/addresses/" + addressID
	return encodeRequest(ctx, req, request)
}

func encodePostAddressRequest(ctx context.Context, req *http.Request, request interface{}) error {
	// r.Methods("POST").Path("/profiles/{id}/addresses/")
	r := request.(postAddressRequest)
	profileID := url.QueryEscape(r.ProfileID)
	req.URL.Path = "/profiles/" + profileID + "/addresses/"
	return encodeRequest(ctx, req, request)
}

func encodeDeleteAddressRequest(ctx context.Context, req *http.Request, request interface{}) error {
	// r.Methods("DELETE").Path("/profiles/{id}/addresses/{addressID}")
	r := request.(deleteAddressRequest)
	profileID := url.QueryEscape(r.ProfileID)
	addressID := url.QueryEscape(r.AddressID)
	req.URL.Path = "/profiles/" + profileID + "/addresses/" + addressID
	return encodeRequest(ctx, req, request)
}

func decodePostProfileResponse(_ context.Context, resp *http.Response) (interface{}, error) {
	var response postProfileResponse
	err := json.NewDecoder(resp.Body).Decode(&response)
	return response, err
}

func decodeGetProfileResponse(_ context.Context, resp *http.Response) (interface{}, error) {
	var response getProfileResponse
	err := json.NewDecoder(resp.Body).Decode(&response)
	return response, err
}

func decodePutProfileResponse(_ context.Context, resp *http.Response) (interface{}, error) {
	var response putProfileResponse
	err := json.NewDecoder(resp.Body).Decode(&response)
	return response, err
}

func decodePatchProfileResponse(_ context.Context, resp *http.Response) (interface{}, error) {
	var response patchProfileResponse
	err := json.NewDecoder(resp.Body).Decode(&response)
	return response, err
}

func decodeDeleteProfileResponse(_ context.Context, resp *http.Response) (interface{}, error) {
	var response deleteProfileResponse
	err := json.NewDecoder(resp.Body).Decode(&response)
	return response, err
}

func decodeGetAddressesResponse(_ context.Context, resp *http.Response) (interface{}, error) {
	var response getAddressesResponse
	err := json.NewDecoder(resp.Body).Decode(&response)
	return response, err
}

func decodeGetAddressResponse(_ context.Context, resp *http.Response) (interface{}, error) {
	var response getAddressResponse
	err := json.NewDecoder(resp.Body).Decode(&response)
	return response, err
}

func decodePostAddressResponse(_ context.Context, resp *http.Response) (interface{}, error) {
	var response postAddressResponse
	err := json.NewDecoder(resp.Body).Decode(&response)
	return response, err
}

func decodeDeleteAddressResponse(_ context.Context, resp *http.Response) (interface{}, error) {
	var response deleteAddressResponse
	err := json.NewDecoder(resp.Body).Decode(&response)
	return response, err
}

// errorer is implemented by all concrete response types that may contain
// errors. It allows us to change the HTTP response code without needing to
// trigger an endpoint (transport-level) error. For more information, read the
// big comment in endpoints.go.
type errorer interface {
	error() error
}

// encodeResponse is the common method to encode all response types to the
// client. I chose to do it this way because, since we're using JSON, there's no
// reason to provide anything more specific. It's certainly possible to
// specialize on a per-response (per-method) basis.
func encodeResponse(ctx context.Context, w http.ResponseWriter, response interface{}) error {
	if e, ok := response.(errorer); ok && e.error() != nil {
		// Not a Go kit transport error, but a business-logic error.
		// Provide those as HTTP errors.
		encodeError(ctx, e.error(), w)
		return nil
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	return json.NewEncoder(w).Encode(response)
}

// encodeRequest likewise JSON-encodes the request to the HTTP request body.
// Don't use it directly as a transport/http.Client EncodeRequestFunc:
// profilesvc endpoints require mutating the HTTP method and request path.
func encodeRequest(_ context.Context, req *http.Request, request interface{}) error {
	var buf bytes.Buffer
	err := json.NewEncoder(&buf).Encode(request)
	if err != nil {
		return err
	}
	req.Body = ioutil.NopCloser(&buf)
	return nil
}

func encodeError(_ context.Context, err error, w http.ResponseWriter) {
	if err == nil {
		panic("encodeError with nil error")
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(codeFrom(err))
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": err.Error(),
	})
}

func codeFrom(err error) int {
	switch err {
	case ErrNotFound:
		return http.StatusNotFound
	case ErrAlreadyExists, ErrInconsistentIDs:
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}
