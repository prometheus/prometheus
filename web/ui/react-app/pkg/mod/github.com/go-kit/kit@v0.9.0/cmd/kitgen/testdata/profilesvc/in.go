package profilesvc

type Service interface {
	PostProfile(ctx context.Context, p Profile) error
	GetProfile(ctx context.Context, id string) (Profile, error)
	PutProfile(ctx context.Context, id string, p Profile) error
	PatchProfile(ctx context.Context, id string, p Profile) error
	DeleteProfile(ctx context.Context, id string) error
	GetAddresses(ctx context.Context, profileID string) ([]Address, error)
	GetAddress(ctx context.Context, profileID string, addressID string) (Address, error)
	PostAddress(ctx context.Context, profileID string, a Address) error
	DeleteAddress(ctx context.Context, profileID string, addressID string) error
}

type Profile struct {
	ID        string    `json:"id"`
	Name      string    `json:"name,omitempty"`
	Addresses []Address `json:"addresses,omitempty"`
}

type Address struct {
	ID       string `json:"id"`
	Location string `json:"location,omitempty"`
}
