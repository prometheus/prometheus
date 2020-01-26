package endpoints

import "context"

import "github.com/go-kit/kit/endpoint"

import "github.com/go-kit/kit/cmd/kitgen/testdata/profilesvc/default/service"

type PostProfileRequest struct {
	P service.Profile
}
type PostProfileResponse struct {
	Err error
}

func MakePostProfileEndpoint(s service.Service) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		req := request.(PostProfileRequest)
		err := s.PostProfile(ctx, req.P)
		return PostProfileResponse{Err: err}, nil
	}
}

type GetProfileRequest struct {
	Id string
}
type GetProfileResponse struct {
	P   service.Profile
	Err error
}

func MakeGetProfileEndpoint(s service.Service) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		req := request.(GetProfileRequest)
		P, err := s.GetProfile(ctx, req.Id)
		return GetProfileResponse{P: P, Err: err}, nil
	}
}

type PutProfileRequest struct {
	Id string
	P  service.Profile
}
type PutProfileResponse struct {
	Err error
}

func MakePutProfileEndpoint(s service.Service) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		req := request.(PutProfileRequest)
		err := s.PutProfile(ctx, req.Id, req.P)
		return PutProfileResponse{Err: err}, nil
	}
}

type PatchProfileRequest struct {
	Id string
	P  service.Profile
}
type PatchProfileResponse struct {
	Err error
}

func MakePatchProfileEndpoint(s service.Service) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		req := request.(PatchProfileRequest)
		err := s.PatchProfile(ctx, req.Id, req.P)
		return PatchProfileResponse{Err: err}, nil
	}
}

type DeleteProfileRequest struct {
	Id string
}
type DeleteProfileResponse struct {
	Err error
}

func MakeDeleteProfileEndpoint(s service.Service) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		req := request.(DeleteProfileRequest)
		err := s.DeleteProfile(ctx, req.Id)
		return DeleteProfileResponse{Err: err}, nil
	}
}

type GetAddressesRequest struct {
	ProfileID string
}
type GetAddressesResponse struct {
	S   []service.Address
	Err error
}

func MakeGetAddressesEndpoint(s service.Service) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		req := request.(GetAddressesRequest)
		slice1, err := s.GetAddresses(ctx, req.ProfileID)
		return GetAddressesResponse{S: slice1, Err: err}, nil
	}
}

type GetAddressRequest struct {
	ProfileID string
	AddressID string
}
type GetAddressResponse struct {
	A   service.Address
	Err error
}

func MakeGetAddressEndpoint(s service.Service) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		req := request.(GetAddressRequest)
		A, err := s.GetAddress(ctx, req.ProfileID, req.AddressID)
		return GetAddressResponse{A: A, Err: err}, nil
	}
}

type PostAddressRequest struct {
	ProfileID string
	A         service.Address
}
type PostAddressResponse struct {
	Err error
}

func MakePostAddressEndpoint(s service.Service) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		req := request.(PostAddressRequest)
		err := s.PostAddress(ctx, req.ProfileID, req.A)
		return PostAddressResponse{Err: err}, nil
	}
}

type DeleteAddressRequest struct {
	ProfileID string
	AddressID string
}
type DeleteAddressResponse struct {
	Err error
}

func MakeDeleteAddressEndpoint(s service.Service) endpoint.Endpoint {
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
