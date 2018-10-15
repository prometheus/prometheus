package collins

import (
	"fmt"
)

// AssetService.GetAttribute returns the value of the requested attribute, or returns
// an empty string and an error if the asset does not have the attribute set.
//
// http://tumblr.github.io/collins/api.html#api-asset-asset-get
func (s AssetService) GetAttribute(tag, attribute string) (string, error) {
	return s.GetAttributeWithDim(tag, attribute, 0)
}

// AssetService.SetAttribute sets an attribute on an asset to an arbitrary string.
//
// http://tumblr.github.io/collins/api.html#api-asset-asset-update
func (s AssetService) SetAttribute(tag, attribute, value string) (*Response, error) {
	return s.SetAttributeWithDim(tag, attribute, value, 0)
}

// AssetService.DeleteAttribute removes an attribute from an asset, or returns an
// error if the asset could not be found.
//
// http://tumblr.github.io/collins/api.html#api-asset-asset-delete-tag
func (s AssetService) DeleteAttribute(tag, attribute string) (*Response, error) {
	return s.DeleteAttributeWithDim(tag, attribute, 0)
}

// AssetService.GetAttributeWithDim performs the same operation as GetAttribute, but
// takes a dimension parameter in order to work with multidimensional
// attributes.
//
// http://tumblr.github.io/collins/api.html#api-asset-asset-get
func (s AssetService) GetAttributeWithDim(tag, attribute string, dimension int) (string, error) {
	asset, _, err := s.Get(tag)
	if err != nil {
		return "", err
	}

	dim := fmt.Sprintf("%d", dimension)
	value, ok := asset.Attributes[dim][attribute]
	if !ok {
		return "", fmt.Errorf("Asset does not have attribute %s.", attribute)
	}
	return value, nil
}

// AssetService.DeleteAttributeWithDim deletes a an attribute with the specified
// dimension from an asset.
//
// http://tumblr.github.io/collins/api.html#api-asset-asset-delete-tag
func (s AssetService) DeleteAttributeWithDim(tag, attribute string, dimension int) (*Response, error) {
	opts := &AssetUpdateOpts{
		GroupID: dimension,
	}
	ustr, err := addOptions("api/asset/"+tag+"/attribute/"+attribute, opts)
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

// AssetService.SetAttributeWithDim sets an attribute with the specified
// dimension on an asset.
//
// http://tumblr.github.io/collins/api.html#api-asset-asset-update
func (s AssetService) SetAttributeWithDim(tag, attribute, value string, dimension int) (*Response, error) {
	opts := &AssetUpdateOpts{
		Attribute: attribute + ";" + value,
		GroupID:   dimension,
	}

	return s.Update(tag, opts)
}
