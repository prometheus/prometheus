package collins

// The TagService provides functions to list tags and their values.
//
// http://tumblr.github.io/collins/api.html#tag
type TagService struct {
	client *Client
}

// Tag represents a tag that can be set on an asset.
type Tag struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

// TagService.List lists all system tags that are in use.
//
// http://tumblr.github.io/collins/api.html#api-tag-list-tags
func (s TagService) List() ([]Tag, *Response, error) {
	ustr, _ := addOptions("api/tags", nil)

	req, err := s.client.NewRequest("GET", ustr)
	if err != nil {
		return nil, nil, err
	}

	var c struct {
		Tags []Tag
	}

	resp, err := s.client.Do(req, &c)
	if err != nil {
		return nil, resp, err
	}

	return c.Tags, resp, nil
}

// TagService.Values returns all the unique values that a certain tag has.
//
// http://tumblr.github.io/collins/api.html#api-tag-list-tag-values
func (s TagService) Values(tag string) ([]string, *Response, error) {
	ustr, _ := addOptions("api/tag/"+tag, nil)

	req, err := s.client.NewRequest("GET", ustr)
	if err != nil {
		return nil, nil, err
	}

	var c struct {
		Values []string
	}

	resp, err := s.client.Do(req, &c)
	if err != nil {
		return nil, resp, err
	}

	return c.Values, resp, nil
}
