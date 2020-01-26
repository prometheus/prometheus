package http

import "context"
import "encoding/json"

import "net/http"

import httptransport "github.com/go-kit/kit/transport/http"
import "github.com/go-kit/kit/cmd/kitgen/testdata/profilesvc/default/endpoints"

func NewHTTPHandler(endpoints endpoints.Endpoints) http.Handler {
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
	var req endpoints.PostProfileRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	return req, err
}
func EncodePostProfileResponse(_ context.Context, w http.ResponseWriter, response interface{}) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	return json.NewEncoder(w).Encode(response)
}
func DecodeGetProfileRequest(_ context.Context, r *http.Request) (interface{}, error) {
	var req endpoints.GetProfileRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	return req, err
}
func EncodeGetProfileResponse(_ context.Context, w http.ResponseWriter, response interface{}) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	return json.NewEncoder(w).Encode(response)
}
func DecodePutProfileRequest(_ context.Context, r *http.Request) (interface{}, error) {
	var req endpoints.PutProfileRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	return req, err
}
func EncodePutProfileResponse(_ context.Context, w http.ResponseWriter, response interface{}) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	return json.NewEncoder(w).Encode(response)
}
func DecodePatchProfileRequest(_ context.Context, r *http.Request) (interface{}, error) {
	var req endpoints.PatchProfileRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	return req, err
}
func EncodePatchProfileResponse(_ context.Context, w http.ResponseWriter, response interface{}) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	return json.NewEncoder(w).Encode(response)
}
func DecodeDeleteProfileRequest(_ context.Context, r *http.Request) (interface{}, error) {
	var req endpoints.DeleteProfileRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	return req, err
}
func EncodeDeleteProfileResponse(_ context.Context, w http.ResponseWriter, response interface{}) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	return json.NewEncoder(w).Encode(response)
}
func DecodeGetAddressesRequest(_ context.Context, r *http.Request) (interface{}, error) {
	var req endpoints.GetAddressesRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	return req, err
}
func EncodeGetAddressesResponse(_ context.Context, w http.ResponseWriter, response interface{}) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	return json.NewEncoder(w).Encode(response)
}
func DecodeGetAddressRequest(_ context.Context, r *http.Request) (interface{}, error) {
	var req endpoints.GetAddressRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	return req, err
}
func EncodeGetAddressResponse(_ context.Context, w http.ResponseWriter, response interface{}) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	return json.NewEncoder(w).Encode(response)
}
func DecodePostAddressRequest(_ context.Context, r *http.Request) (interface{}, error) {
	var req endpoints.PostAddressRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	return req, err
}
func EncodePostAddressResponse(_ context.Context, w http.ResponseWriter, response interface{}) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	return json.NewEncoder(w).Encode(response)
}
func DecodeDeleteAddressRequest(_ context.Context, r *http.Request) (interface{}, error) {
	var req endpoints.DeleteAddressRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	return req, err
}
func EncodeDeleteAddressResponse(_ context.Context, w http.ResponseWriter, response interface{}) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	return json.NewEncoder(w).Encode(response)
}
