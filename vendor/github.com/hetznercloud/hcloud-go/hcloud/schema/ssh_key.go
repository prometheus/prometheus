package schema

import "time"

// SSHKey defines the schema of a SSH key.
type SSHKey struct {
	ID          int               `json:"id"`
	Name        string            `json:"name"`
	Fingerprint string            `json:"fingerprint"`
	PublicKey   string            `json:"public_key"`
	Labels      map[string]string `json:"labels"`
	Created     time.Time         `json:"created"`
}

// SSHKeyCreateRequest defines the schema of the request
// to create a SSH key.
type SSHKeyCreateRequest struct {
	Name      string             `json:"name"`
	PublicKey string             `json:"public_key"`
	Labels    *map[string]string `json:"labels,omitempty"`
}

// SSHKeyCreateResponse defines the schema of the response
// when creating a SSH key.
type SSHKeyCreateResponse struct {
	SSHKey SSHKey `json:"ssh_key"`
}

// SSHKeyListResponse defines the schema of the response
// when listing SSH keys.
type SSHKeyListResponse struct {
	SSHKeys []SSHKey `json:"ssh_keys"`
}

// SSHKeyGetResponse defines the schema of the response
// when retrieving a single SSH key.
type SSHKeyGetResponse struct {
	SSHKey SSHKey `json:"ssh_key"`
}

// SSHKeyUpdateRequest defines the schema of the request to update a SSH key.
type SSHKeyUpdateRequest struct {
	Name   string             `json:"name,omitempty"`
	Labels *map[string]string `json:"labels,omitempty"`
}

// SSHKeyUpdateResponse defines the schema of the response when updating a SSH key.
type SSHKeyUpdateResponse struct {
	SSHKey SSHKey `json:"ssh_key"`
}
