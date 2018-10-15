package collins

// The StateService provides functions to manage states in collins.
//
// http://tumblr.github.io/collins/api.html#state
type StateService struct {
	client *Client
}

// State represents a state which describes a lifecycle specific to a status.
type State struct {
	ID     int    `json:"ID" url:"id,omitempty"`
	Name   string `json:"NAME" url:"name"`
	Status struct {
		ID          int    `json:"ID" url:",omitempty"`
		Name        string `json:"NAME" url:",omitempty"`
		Description string `json:"DESCRIPTION" url:",omitempty"`
	} `json:"STATUS" url:",omitempty"`
	Label        string `json:"LABEL" url:"label"`
	Description  string `json:"DESCRIPTION" url:"description"`
	CreateStatus string `url:"status,omitempty"` // Only used when creating states associated with a status
}

// StateUpdateOpts is a struct for supplying information to Update(). Each of
// the struct members are optional, and only those specified will be used for
// update.
type StateUpdateOpts struct {
	Name        string `url:"name,omitempty"`
	Label       string `url:"label,omitempty"`
	Description string `url:"description,omitempty"`
}

// StateService.Create creates a state with the supplied name, label and
// description. If `status` is non-empty, the new state is bound to the status.
//
// http://tumblr.github.io/collins/api.html#api-state-state-create
func (s StateService) Create(name, label, description, status string) (*Response, error) {
	state := State{Name: name, Label: label, Description: description}

	if status != "" {
		state.CreateStatus = status
	}

	ustr, err := addOptions("api/state/"+name, state)
	if err != nil {
		return nil, err
	}

	req, err := s.client.NewRequest("PUT", ustr)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req, nil)
	return resp, err
}

// StateService.Update updates the state with name `old_name` using the
// information in `opts`. See the StateUpdateOpts for options on what can be
// updated.
//
// http://tumblr.github.io/collins/api.html#api-state-state-updat
func (s StateService) Update(oldName string, opts StateUpdateOpts) (*Response, error) {
	ustr, err := addOptions("api/state/"+oldName, opts)
	if err != nil {
		return nil, err
	}

	req, err := s.client.NewRequest("POST", ustr)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req, nil)
	return resp, err
}

// StateService.Delete deletes the state with name `name`.
//
// http://tumblr.github.io/collins/api.html#api-state-state-delete
func (s StateService) Delete(name string) (*Response, error) {
	ustr, err := addOptions("api/state/"+name, nil)
	if err != nil {
		return nil, err
	}

	req, err := s.client.NewRequest("DELETE", ustr)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req, nil)
	if err != nil {
		return resp, err
	}

	return resp, nil
}

// StateService.Get returns the state with name `name`.
//
// http://tumblr.github.io/collins/api.html#api-state-state-get
func (s StateService) Get(name string) (*State, *Response, error) {
	ustr, err := addOptions("api/state/"+name, nil)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("GET", ustr)
	if err != nil {
		return nil, nil, err
	}

	state := new(State)
	resp, err := s.client.Do(req, state)
	if err != nil {
		return nil, resp, err
	}

	return state, resp, nil
}

// StateService.List retrieves a list of all states collins knows about.
//
// http://tumblr.github.io/collins/api.html#api-state-state-get-all
func (s StateService) List() ([]State, *Response, error) {
	ustr, _ := addOptions("api/states", nil)

	req, err := s.client.NewRequest("GET", ustr)
	if err != nil {
		return nil, nil, err
	}

	states := []State{}
	resp, err := s.client.Do(req, &states)
	if err != nil {
		return nil, resp, err
	}

	return states, resp, nil
}
