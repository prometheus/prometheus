package collins

// AssetTypeService provides functions to manage asset types in collins.
//
// http://tumblr.github.io/collins/api.html#asset%20type
type AssetTypeService struct {
	client *Client
}

// AssetType Represents an asset type in the collins eco system. Examples of
// asset types are `SERVER_NODE`, `ROUTER` and `DATA_CENTER`. Each asset type
// has an ID, a name, and a human readable label.
type AssetType struct {
	ID    int    `json:"ID" url:"-"`
	Name  string `json:"NAME" url:"name,omitempty"`
	Label string `json:"LABEL" url:"label,omitempty"`
}

// AssetTypeService.Create creates a new asset type with the specified name and
// label.
//
// http://tumblr.github.io/collins/api.html#api-asset%20type-asset-type-create
func (s AssetTypeService) Create(name, label string) (*AssetType, *Response, error) {
	at := AssetType{Label: label}
	ustr, err := addOptions("api/assettype/"+name, at)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("PUT", ustr)
	if err != nil {
		return nil, nil, err
	}

	resp, err := s.client.Do(req, nil)
	if err != nil {
		return nil, resp, err
	}

	// Set name down here so we don't send any unwanted parameters to the API
	at.Name = name
	return &at, resp, nil
}

// AssetTypeService.Update updates and asset type with a new name and label.
//
// http://tumblr.github.io/collins/api.html#api-asset%20type-asset-type-update
func (s AssetTypeService) Update(oldName, newName, newLabel string) (*AssetType, *Response, error) {
	at := AssetType{
		Name:  newName,
		Label: newLabel,
	}

	ustr, err := addOptions("api/assettype/"+oldName, at)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("POST", ustr)
	if err != nil {
		return nil, nil, err
	}

	resp, err := s.client.Do(req, nil)
	if err != nil {
		return nil, resp, err
	}

	return &at, resp, nil
}

// AssetTypeService.Get queries the API for a specific asset type with name
// `name`.
//
// http://tumblr.github.io/collins/api.html#api-asset%20type-asset-type-get
func (s AssetTypeService) Get(name string) (*AssetType, *Response, error) {
	ustr, err := addOptions("api/assettype/"+name, nil)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("GET", ustr)
	if err != nil {
		return nil, nil, err
	}

	assetType := new(AssetType)
	resp, err := s.client.Do(req, assetType)
	if err != nil {
		return nil, resp, err
	}

	return assetType, resp, nil
}

// AssetTypeService.List queries the API for all available asset types.
//
// http://tumblr.github.io/collins/api.html#api-asset%20type-asset-type-list
func (s AssetTypeService) List() (*[]AssetType, *Response, error) {
	ustr, _ := addOptions("api/assettypes", nil)

	req, err := s.client.NewRequest("GET", ustr)
	if err != nil {
		return nil, nil, err
	}

	types := []AssetType{}
	resp, err := s.client.Do(req, &types)
	if err != nil {
		return nil, resp, err
	}

	return &types, resp, nil
}

// AssetTypeService.Delete deletes asset type with the name `asset_type`.
//
// http://tumblr.github.io/collins/api.html#api-asset%20type-asset-type-delete
func (s AssetTypeService) Delete(assetType string) (*Response, error) {
	ustr, err := addOptions("api/assettype/"+assetType, nil)
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
