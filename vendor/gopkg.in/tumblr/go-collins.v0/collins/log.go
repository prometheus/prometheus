package collins

// The LogService provides functions to add, delete and manage the logs for
// assets in collins.
//
// http://tumblr.github.io/collins/api.html#asset%20log
type LogService struct {
	client *Client
}

// Log represents a single log message along with metadata.
type Log struct {
	ID       int    `json:"ID"`
	AssetTag string `json:"ASSET_TAG"`
	Created  string `json:"CREATED"`
	Format   string `json:"FORMAT"`
	Source   string `json:"SOURCE"`
	Type     string `json:"TYPE"`
	Message  string `json:"MESSAGE"`
}

// LogCreateOpts are options to be passed when creating a log entry.
type LogCreateOpts struct {
	Message string `url:"message"`
	Type    string `url:"type,omitempty"`
}

// LogGetOpts are options to be passed when getting logs.
type LogGetOpts struct {
	PageOpts
}

// LogService.Create adds a log message for the asset with tag `tag`.
//
// http://tumblr.github.io/collins/api.html#api-asset%20log-log-create
func (s LogService) Create(tag string, opts *LogCreateOpts) (*Log, *Response, error) {
	ustr, err := addOptions("api/asset/"+tag+"/log", opts)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("PUT", ustr)
	if err != nil {
		return nil, nil, err
	}

	var c struct {
		Log `json:"DATA"`
	}

	resp, err := s.client.Do(req, &c)
	if err != nil {
		return nil, resp, err
	}

	return &c.Log, resp, nil
}

// LogService.Get returns logs associated with the asset with asset tag `tag`.
//
// http://tumblr.github.io/collins/api.html#api-asset%20log-log-get
func (s LogService) Get(tag string, opts *LogGetOpts) ([]Log, *Response, error) {
	ustr, err := addOptions("api/asset/"+tag+"/logs", opts)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("GET", ustr)
	if err != nil {
		return nil, nil, err
	}

	var c struct {
		Logs []Log `json:"DATA"`
	}

	resp, err := s.client.Do(req, &c)
	if err != nil {
		return nil, resp, err
	}

	return c.Logs, resp, nil
}

// LogService.GetAll returns logs for all assets.
//
// http://tumblr.github.io/collins/api.html#api-asset%20log-log-get-all
func (s LogService) GetAll(opts *LogGetOpts) ([]Log, *Response, error) {
	ustr, err := addOptions("api/assets/logs", opts)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("GET", ustr)
	if err != nil {
		return nil, nil, err
	}

	var c struct {
		Logs []Log `json:"DATA"`
	}

	resp, err := s.client.Do(req, &c)
	if err != nil {
		return nil, resp, err
	}

	return c.Logs, resp, nil
}
