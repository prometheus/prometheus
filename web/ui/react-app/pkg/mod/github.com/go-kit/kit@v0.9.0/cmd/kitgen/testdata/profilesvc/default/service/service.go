package service

import "context"

import "errors"

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
func (s Service) GetProfile(ctx context.Context, id string) (Profile, error) {
	panic(errors.New("not implemented"))
}
func (s Service) PutProfile(ctx context.Context, id string, p Profile) error {
	panic(errors.New("not implemented"))
}
func (s Service) PatchProfile(ctx context.Context, id string, p Profile) error {
	panic(errors.New("not implemented"))
}
func (s Service) DeleteProfile(ctx context.Context, id string) error {
	panic(errors.New("not implemented"))
}
func (s Service) GetAddresses(ctx context.Context, profileID string) ([]Address, error) {
	panic(errors.New("not implemented"))
}
func (s Service) GetAddress(ctx context.Context, profileID string, addressID string) (Address, error) {
	panic(errors.New("not implemented"))
}
func (s Service) PostAddress(ctx context.Context, profileID string, a Address) error {
	panic(errors.New("not implemented"))
}
func (s Service) DeleteAddress(ctx context.Context, profileID string, addressID string) error {
	panic(errors.New("not implemented"))
}
