package profilesvc

import "context"
import "encoding/json"
import "errors"
import "net/http"
import "github.com/go-kit/kit/endpoint"
import httptransport "github.com/go-kit/kit/transport/http"

type Profile struct {
	ID        string    `json:"id"`
	Name      string    `json:"name,omitempty"`
	Addresses []Address `json:"addresses,omitempty"`
}
type Address struct {
	ID       string `json:"id"`
	Location string `json:"location,omitempty"`
}
type Service struct {
}

func (s Service) PostProfile(ctx context.Context, p Profile) error {
	panic(errors.New("not implemented"))
}

type PostProfileRequest struct {
	P Profile
}
type PostProfileResponse struct {
	Err error
}

func MakePostProfileEndpoint(s Service) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		req := request.(PostProfileRequest)
		err := s.PostProfile(ctx, req.P)
		return PostProfileResponse{Err: err}, nil
	}
}
func (s Service) GetProfile(ctx context.Context, id string) (Profile, error) {
	panic(errors.New("not implemented"))
}

type GetProfileRequest struct {
	Id string
}
type GetProfileResponse struct {
	P   Profile
	Err error
}

func MakeGetProfileEndpoint(s Service) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		req := request.(GetProfileRequest)
		P, err := s.GetProfile(ctx, req.Id)
		return GetProfileResponse{P: P, Err: err}, nil
	}
}
func (s Service) PutProfile(ctx context.Context, id string, p Profile) error {
	panic(errors.New("not implemented"))
}

type PutProfileRequest struct {
	Id string
	P  Profile
}
type PutProfileResponse struct {
	Err error
}

func MakePutProfileEndpoint(s Service) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		req := request.(PutProfileRequest)
		err := s.PutProfile(ctx, req.Id, req.P)
		return PutProfileResponse{Err: err}, nil
	}
}
func (s Service) PatchProfile(ctx context.Context, id string, p Profile) error {
	panic(errors.New("not implemented"))
}

type PatchProfileRequest struct {
	Id string
	P  Profile
}
type PatchProfileResponse struct {
	Err error
}

func MakePatchProfileEndpoint(s Service) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		req := request.(PatchProfileRequest)
		err := s.PatchProfile(ctx, req.Id, req.P)
		return PatchProfileResponse{Err: err}, nil
	}
}
func (s Service) DeleteProfile(ctx context.Context, id string) error {
	panic(errors.New("not implemented"))
}

type DeleteProfileRequest struct {
	Id string
}
type DeleteProfileResponse struct {
	Err error
}

func MakeDeleteProfileEndpoint(s Service) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		req := request.(DeleteProfileRequest)
		err := s.DeleteProfile(ctx, req.Id)
		return DeleteProfileResponse{Err: err}, nil
	}
}
func (s Service) GetAddresses(ctx context.Context, profileID string) ([]Address, error) {
	panic(errors.New("not implemented"))
}

type GetAddressesRequest struct {
	ProfileID string
}
type GetAddressesResponse struct {
	S   []Address
	Err error
}

func MakeGetAddressesEndpoint(s Service) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		req := request.(GetAddressesRequest)
		slice1, err := s.GetAddresses(ctx, req.ProfileID)
		return GetAddressesResponse{S: slice1, Err: err}, nil
	}
}
func (s Service) GetAddress(ctx context.Context, profileID string, addressID string) (Address, error) {
	panic(errors.New("not implemented"))
}

type GetAddressRequest struct {
	ProfileID string
	AddressID string
}
type GetAddressResponse struct {
	A   Address
	Err error
}

func MakeGetAddressEndpoint(s Service) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		req := request.(GetAddressRequest)
		A, err := s.GetAddress(ctx, req.ProfileID, req.AddressID)
		return GetAddressResponse{A: A, Err: err}, nil
	}
}
func (s Service) PostAddress(ctx context.Context, profileID string, a Address) error {
	panic(errors.New("not implemented"))
}

type PostAddressRequest struct {
	ProfileID string
	A         Address
}
type PostAddressResponse struct {
	Err error
}

func MakePostAddressEndpoint(s Service) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		req := request.(PostAddressRequest)
		err := s.PostAddress(ctx, req.ProfileID, req.A)
		return PostAddressResponse{Err: err}, nil
	}
}
func (s Service) DeleteAddress(ctx context.Context, profileID string, addressID string) error {
	panic(errors.New("not implemented"))
}

type DeleteAddressRequest struct {
	ProfileID string
	AddressID string
}
type DeleteAddressResponse struct {
	Err error
}

func MakeDeleteAddressEndpoint(s Service) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		req := request.(DeleteAddressRequest)
		err := s.DeleteAddress(ctx, req.ProfileID, req.AddressID)
		return DeleteAddressResponse{Err: err}, nil
	}
}

type Endpoints struct {
	PostProfile   endpoint.Endpoint
	GetProfile    endpoint.Endpoint
	PutProfile    endpoint.Endpoint
	PatchProfile  endpoint.Endpoint
	DeleteProfile endpoint.Endpoint
	GetAddresses  endpoint.Endpoint
	GetAddress    endpoint.Endpoint
	PostAddress   endpoint.Endpoint
	DeleteAddress endpoint.Endpoint
}

func NewHTTPHandler(endpoints Endpoints) http.Handler {
	m := http.NewServeMux()
	m.Handle("/postprofile", httptransport.NewServer(endpoints.PostProfile, DecodePostProfileRequest, EncodePostProfileResponse))
	m.Handle("/getprofile", httptransport.NewServer(endpoints.GetProfile, DecodeGetProfileRequest, EncodeGetProfileResponse))
	m.Handle("/putprofile", httptransport.NewServer(endpoints.PutProfile, DecodePutProfileRequest, EncodePutProfileResponse))
	m.Handle("/patchprofile", httptransport.NewServer(endpoints.PatchProfile, DecodePatchProfileRequest, EncodePatchProfileResponse))
	m.Handle("/deleteprofile", httptransport.NewServer(endpoints.DeleteProfile, DecodeDeleteProfileRequest, EncodeDeleteProfileResponse))
	m.Handle("/getaddresses", httptransport.NewServer(endpoints.GetAddresses, DecodeGetAddressesRequest, EncodeGetAddressesResponse))
	m.Handle("/getaddress", httptransport.NewServer(endpoints.GetAddress, DecodeGetAddressRequest, EncodeGetAddressResponse))
	m.Handle("/postaddress", httptransport.NewServer(endpoints.PostAddress, DecodePostAddressRequest, EncodePostAddressResponse))
	m.Handle("/deleteaddress", httptransport.NewServer(endpoints.DeleteAddress, DecodeDeleteAddressRequest, EncodeDeleteAddressResponse))
	return m
}
func DecodePostProfileRequest(_ context.Context, r *http.Request) (interface{}, error) {
	var req PostProfileRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	return req, err
}
func EncodePostProfileResponse(_ context.Context, w http.ResponseWriter, response interface{}) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	return json.NewEncoder(w).Encode(response)
}
func DecodeGetProfileRequest(_ context.Context, r *http.Request) (interface{}, error) {
	var req GetProfileRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	return req, err
}
func EncodeGetProfileResponse(_ context.Context, w http.ResponseWriter, response interface{}) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	return json.NewEncoder(w).Encode(response)
}
func DecodePutProfileRequest(_ context.Context, r *http.Request) (interface{}, error) {
	var req PutProfileRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	return req, err
}
func EncodePutProfileResponse(_ context.Context, w http.ResponseWriter, response interface{}) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	return json.NewEncoder(w).Encode(response)
}
func DecodePatchProfileRequest(_ context.Context, r *http.Request) (interface{}, error) {
	var req PatchProfileRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	return req, err
}
func EncodePatchProfileResponse(_ context.Context, w http.ResponseWriter, response interface{}) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	return json.NewEncoder(w).Encode(response)
}
func DecodeDeleteProfileRequest(_ context.Context, r *http.Request) (interface{}, error) {
	var req DeleteProfileRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	return req, err
}
func EncodeDeleteProfileResponse(_ context.Context, w http.ResponseWriter, response interface{}) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	return json.NewEncoder(w).Encode(response)
}
func DecodeGetAddressesRequest(_ context.Context, r *http.Request) (interface{}, error) {
	var req GetAddressesRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	return req, err
}
func EncodeGetAddressesResponse(_ context.Context, w http.ResponseWriter, response interface{}) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	return json.NewEncoder(w).Encode(response)
}
func DecodeGetAddressRequest(_ context.Context, r *http.Request) (interface{}, error) {
	var req GetAddressRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	return req, err
}
func EncodeGetAddressResponse(_ context.Context, w http.ResponseWriter, response interface{}) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	return json.NewEncoder(w).Encode(response)
}
func DecodePostAddressRequest(_ context.Context, r *http.Request) (interface{}, error) {
	var req PostAddressRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	return req, err
}
func EncodePostAddressResponse(_ context.Context, w http.ResponseWriter, response interface{}) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	return json.NewEncoder(w).Encode(response)
}
func DecodeDeleteAddressRequest(_ context.Context, r *http.Request) (interface{}, error) {
	var req DeleteAddressRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	return req, err
}
func EncodeDeleteAddressResponse(_ context.Context, w http.ResponseWriter, response interface{}) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	return json.NewEncoder(w).Encode(response)
}
