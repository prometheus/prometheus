package base

// Client is a Warp10 client
type Client struct {
	Host         string
	Warp10Header string

	ExecPath   string
	UpdatePath string
	MetaPath   string
	FetchPath  string
	FindPath   string
	DeletePath string

	ReadToken  string
	WriteToken string
}

// NewClient return a configured Warp10 Client
func NewClient(host string) *Client {
	return &Client{
		Host:         host,
		Warp10Header: "X-Warp10-Token",
		ExecPath:     "/api/v0/exec",
		UpdatePath:   "/api/v0/update",
		MetaPath:     "/api/v0/meta",
		FetchPath:    "/api/v0/fetch",
		FindPath:     "/api/v0/find",
		DeletePath:   "/api/v0/delete",
	}
}
