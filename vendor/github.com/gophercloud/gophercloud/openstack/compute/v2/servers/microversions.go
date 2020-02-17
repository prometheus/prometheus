package servers

// TagsExt is an extension to the base Server struct.
// Use this in combination with the Server struct to create
// a composed struct in order to extract a slice of servers
// with the Tag field.
//
// This requires the client to be set to microversion 2.26 or later.
//
// To interact with a server's tags directly, see the
// openstack/compute/v2/extensions/tags package.
type TagsExt struct {
	// Tags contains a list of server tags.
	Tags []string `json:"tags"`
}

// ExtractTags will extract the tags of a server.
//
// This requires the client to be set to microversion 2.26 or later.
//
// To interact with a server's tags directly, see the
// openstack/compute/v2/extensions/tags package.
func (r serverResult) ExtractTags() ([]string, error) {
	var s struct {
		Tags []string `json:"tags"`
	}
	err := r.ExtractInto(&s)
	return s.Tags, err
}
