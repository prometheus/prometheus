package profilesvc

import (
	"context"
	"net/url"
	"strings"

	"github.com/go-kit/kit/endpoint"
	httptransport "github.com/go-kit/kit/transport/http"
)

// Endpoints collects all of the endpoints that compose a profile service. It's
// meant to be used as a helper struct, to collect all of the endpoints into a
// single parameter.
//
// In a server, it's useful for functions that need to operate on a per-endpoint
// basis. For example, you might pass an Endpoints to a function that produces
// an http.Handler, with each method (endpoint) wired up to a specific path. (It
// is probably a mistake in design to invoke the Service methods on the
// Endpoints struct in a server.)
//
// In a client, it's useful to collect individually constructed endpoints into a
// single type that implements the Service interface. For example, you might
// construct individual endpoints using transport/http.NewClient, combine them
// into an Endpoints, and return it to the caller as a Service.
type Endpoints struct {
	PostProfileEndpoint   endpoint.Endpoint
	GetProfileEndpoint    endpoint.Endpoint
	PutProfileEndpoint    endpoint.Endpoint
	PatchProfileEndpoint  endpoint.Endpoint
	DeleteProfileEndpoint endpoint.Endpoint
	GetAddressesEndpoint  endpoint.Endpoint
	GetAddressEndpoint    endpoint.Endpoint
	PostAddressEndpoint   endpoint.Endpoint
	DeleteAddressEndpoint endpoint.Endpoint
}

// MakeServerEndpoints returns an Endpoints struct where each endpoint invokes
// the corresponding method on the provided service. Useful in a profilesvc
// server.
func MakeServerEndpoints(s Service) Endpoints {
	return Endpoints{
		PostProfileEndpoint:   MakePostProfileEndpoint(s),
		GetProfileEndpoint:    MakeGetProfileEndpoint(s),
		PutProfileEndpoint:    MakePutProfileEndpoint(s),
		PatchProfileEndpoint:  MakePatchProfileEndpoint(s),
		DeleteProfileEndpoint: MakeDeleteProfileEndpoint(s),
		GetAddressesEndpoint:  MakeGetAddressesEndpoint(s),
		GetAddressEndpoint:    MakeGetAddressEndpoint(s),
		PostAddressEndpoint:   MakePostAddressEndpoint(s),
		DeleteAddressEndpoint: MakeDeleteAddressEndpoint(s),
	}
}

// MakeClientEndpoints returns an Endpoints struct where each endpoint invokes
// the corresponding method on the remote instance, via a transport/http.Client.
// Useful in a profilesvc client.
func MakeClientEndpoints(instance string) (Endpoints, error) {
	if !strings.HasPrefix(instance, "http") {
		instance = "http://" + instance
	}
	tgt, err := url.Parse(instance)
	if err != nil {
		return Endpoints{}, err
	}
	tgt.Path = ""

	options := []httptransport.ClientOption{}

	// Note that the request encoders need to modify the request URL, changing
	// the path. That's fine: we simply need to provide specific encoders for
	// each endpoint.

	return Endpoints{
		PostProfileEndpoint:   httptransport.NewClient("POST", tgt, encodePostProfileRequest, decodePostProfileResponse, options...).Endpoint(),
		GetProfileEndpoint:    httptransport.NewClient("GET", tgt, encodeGetProfileRequest, decodeGetProfileResponse, options...).Endpoint(),
		PutProfileEndpoint:    httptransport.NewClient("PUT", tgt, encodePutProfileRequest, decodePutProfileResponse, options...).Endpoint(),
		PatchProfileEndpoint:  httptransport.NewClient("PATCH", tgt, encodePatchProfileRequest, decodePatchProfileResponse, options...).Endpoint(),
		DeleteProfileEndpoint: httptransport.NewClient("DELETE", tgt, encodeDeleteProfileRequest, decodeDeleteProfileResponse, options...).Endpoint(),
		GetAddressesEndpoint:  httptransport.NewClient("GET", tgt, encodeGetAddressesRequest, decodeGetAddressesResponse, options...).Endpoint(),
		GetAddressEndpoint:    httptransport.NewClient("GET", tgt, encodeGetAddressRequest, decodeGetAddressResponse, options...).Endpoint(),
		PostAddressEndpoint:   httptransport.NewClient("POST", tgt, encodePostAddressRequest, decodePostAddressResponse, options...).Endpoint(),
		DeleteAddressEndpoint: httptransport.NewClient("DELETE", tgt, encodeDeleteAddressRequest, decodeDeleteAddressResponse, options...).Endpoint(),
	}, nil
}

// PostProfile implements Service. Primarily useful in a client.
func (e Endpoints) PostProfile(ctx context.Context, p Profile) error {
	request := postProfileRequest{Profile: p}
	response, err := e.PostProfileEndpoint(ctx, request)
	if err != nil {
		return err
	}
	resp := response.(postProfileResponse)
	return resp.Err
}

// GetProfile implements Service. Primarily useful in a client.
func (e Endpoints) GetProfile(ctx context.Context, id string) (Profile, error) {
	request := getProfileRequest{ID: id}
	response, err := e.GetProfileEndpoint(ctx, request)
	if err != nil {
		return Profile{}, err
	}
	resp := response.(getProfileResponse)
	return resp.Profile, resp.Err
}

// PutProfile implements Service. Primarily useful in a client.
func (e Endpoints) PutProfile(ctx context.Context, id string, p Profile) error {
	request := putProfileRequest{ID: id, Profile: p}
	response, err := e.PutProfileEndpoint(ctx, request)
	if err != nil {
		return err
	}
	resp := response.(putProfileResponse)
	return resp.Err
}

// PatchProfile implements Service. Primarily useful in a client.
func (e Endpoints) PatchProfile(ctx context.Context, id string, p Profile) error {
	request := patchProfileRequest{ID: id, Profile: p}
	response, err := e.PatchProfileEndpoint(ctx, request)
	if err != nil {
		return err
	}
	resp := response.(patchProfileResponse)
	return resp.Err
}

// DeleteProfile implements Service. Primarily useful in a client.
func (e Endpoints) DeleteProfile(ctx context.Context, id string) error {
	request := deleteProfileRequest{ID: id}
	response, err := e.DeleteProfileEndpoint(ctx, request)
	if err != nil {
		return err
	}
	resp := response.(deleteProfileResponse)
	return resp.Err
}

// GetAddresses implements Service. Primarily useful in a client.
func (e Endpoints) GetAddresses(ctx context.Context, profileID string) ([]Address, error) {
	request := getAddressesRequest{ProfileID: profileID}
	response, err := e.GetAddressesEndpoint(ctx, request)
	if err != nil {
		return nil, err
	}
	resp := response.(getAddressesResponse)
	return resp.Addresses, resp.Err
}

// GetAddress implements Service. Primarily useful in a client.
func (e Endpoints) GetAddress(ctx context.Context, profileID string, addressID string) (Address, error) {
	request := getAddressRequest{ProfileID: profileID, AddressID: addressID}
	response, err := e.GetAddressEndpoint(ctx, request)
	if err != nil {
		return Address{}, err
	}
	resp := response.(getAddressResponse)
	return resp.Address, resp.Err
}

// PostAddress implements Service. Primarily useful in a client.
func (e Endpoints) PostAddress(ctx context.Context, profileID string, a Address) error {
	request := postAddressRequest{ProfileID: profileID, Address: a}
	response, err := e.PostAddressEndpoint(ctx, request)
	if err != nil {
		return err
	}
	resp := response.(postAddressResponse)
	return resp.Err
}

// DeleteAddress implements Service. Primarily useful in a client.
func (e Endpoints) DeleteAddress(ctx context.Context, profileID string, addressID string) error {
	request := deleteAddressRequest{ProfileID: profileID, AddressID: addressID}
	response, err := e.DeleteAddressEndpoint(ctx, request)
	if err != nil {
		return err
	}
	resp := response.(deleteAddressResponse)
	return resp.Err
}

// MakePostProfileEndpoint returns an endpoint via the passed service.
// Primarily useful in a server.
func MakePostProfileEndpoint(s Service) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (response interface{}, err error) {
		req := request.(postProfileRequest)
		e := s.PostProfile(ctx, req.Profile)
		return postProfileResponse{Err: e}, nil
	}
}

// MakeGetProfileEndpoint returns an endpoint via the passed service.
// Primarily useful in a server.
func MakeGetProfileEndpoint(s Service) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (response interface{}, err error) {
		req := request.(getProfileRequest)
		p, e := s.GetProfile(ctx, req.ID)
		return getProfileResponse{Profile: p, Err: e}, nil
	}
}

// MakePutProfileEndpoint returns an endpoint via the passed service.
// Primarily useful in a server.
func MakePutProfileEndpoint(s Service) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (response interface{}, err error) {
		req := request.(putProfileRequest)
		e := s.PutProfile(ctx, req.ID, req.Profile)
		return putProfileResponse{Err: e}, nil
	}
}

// MakePatchProfileEndpoint returns an endpoint via the passed service.
// Primarily useful in a server.
func MakePatchProfileEndpoint(s Service) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (response interface{}, err error) {
		req := request.(patchProfileRequest)
		e := s.PatchProfile(ctx, req.ID, req.Profile)
		return patchProfileResponse{Err: e}, nil
	}
}

// MakeDeleteProfileEndpoint returns an endpoint via the passed service.
// Primarily useful in a server.
func MakeDeleteProfileEndpoint(s Service) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (response interface{}, err error) {
		req := request.(deleteProfileRequest)
		e := s.DeleteProfile(ctx, req.ID)
		return deleteProfileResponse{Err: e}, nil
	}
}

// MakeGetAddressesEndpoint returns an endpoint via the passed service.
// Primarily useful in a server.
func MakeGetAddressesEndpoint(s Service) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (response interface{}, err error) {
		req := request.(getAddressesRequest)
		a, e := s.GetAddresses(ctx, req.ProfileID)
		return getAddressesResponse{Addresses: a, Err: e}, nil
	}
}

// MakeGetAddressEndpoint returns an endpoint via the passed service.
// Primarily useful in a server.
func MakeGetAddressEndpoint(s Service) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (response interface{}, err error) {
		req := request.(getAddressRequest)
		a, e := s.GetAddress(ctx, req.ProfileID, req.AddressID)
		return getAddressResponse{Address: a, Err: e}, nil
	}
}

// MakePostAddressEndpoint returns an endpoint via the passed service.
// Primarily useful in a server.
func MakePostAddressEndpoint(s Service) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (response interface{}, err error) {
		req := request.(postAddressRequest)
		e := s.PostAddress(ctx, req.ProfileID, req.Address)
		return postAddressResponse{Err: e}, nil
	}
}

// MakeDeleteAddressEndpoint returns an endpoint via the passed service.
// Primarily useful in a server.
func MakeDeleteAddressEndpoint(s Service) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (response interface{}, err error) {
		req := request.(deleteAddressRequest)
		e := s.DeleteAddress(ctx, req.ProfileID, req.AddressID)
		return deleteAddressResponse{Err: e}, nil
	}
}

// We have two options to return errors from the business logic.
//
// We could return the error via the endpoint itself. That makes certain things
// a little bit easier, like providing non-200 HTTP responses to the client. But
// Go kit assumes that endpoint errors are (or may be treated as)
// transport-domain errors. For example, an endpoint error will count against a
// circuit breaker error count.
//
// Therefore, it's often better to return service (business logic) errors in the
// response object. This means we have to do a bit more work in the HTTP
// response encoder to detect e.g. a not-found error and provide a proper HTTP
// status code. That work is done with the errorer interface, in transport.go.
// Response types that may contain business-logic errors implement that
// interface.

type postProfileRequest struct {
	Profile Profile
}

type postProfileResponse struct {
	Err error `json:"err,omitempty"`
}

func (r postProfileResponse) error() error { return r.Err }

type getProfileRequest struct {
	ID string
}

type getProfileResponse struct {
	Profile Profile `json:"profile,omitempty"`
	Err     error   `json:"err,omitempty"`
}

func (r getProfileResponse) error() error { return r.Err }

type putProfileRequest struct {
	ID      string
	Profile Profile
}

type putProfileResponse struct {
	Err error `json:"err,omitempty"`
}

func (r putProfileResponse) error() error { return nil }

type patchProfileRequest struct {
	ID      string
	Profile Profile
}

type patchProfileResponse struct {
	Err error `json:"err,omitempty"`
}

func (r patchProfileResponse) error() error { return r.Err }

type deleteProfileRequest struct {
	ID string
}

type deleteProfileResponse struct {
	Err error `json:"err,omitempty"`
}

func (r deleteProfileResponse) error() error { return r.Err }

type getAddressesRequest struct {
	ProfileID string
}

type getAddressesResponse struct {
	Addresses []Address `json:"addresses,omitempty"`
	Err       error     `json:"err,omitempty"`
}

func (r getAddressesResponse) error() error { return r.Err }

type getAddressRequest struct {
	ProfileID string
	AddressID string
}

type getAddressResponse struct {
	Address Address `json:"address,omitempty"`
	Err     error   `json:"err,omitempty"`
}

func (r getAddressResponse) error() error { return r.Err }

type postAddressRequest struct {
	ProfileID string
	Address   Address
}

type postAddressResponse struct {
	Err error `json:"err,omitempty"`
}

func (r postAddressResponse) error() error { return r.Err }

type deleteAddressRequest struct {
	ProfileID string
	AddressID string
}

type deleteAddressResponse struct {
	Err error `json:"err,omitempty"`
}

func (r deleteAddressResponse) error() error { return r.Err }
