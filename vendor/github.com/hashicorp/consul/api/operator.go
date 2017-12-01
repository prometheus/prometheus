package api

// Operator can be used to perform low-level operator tasks for Consul.
type Operator struct {
	c *Client
}

// Operator returns a handle to the operator endpoints.
func (c *Client) Operator() *Operator {
	return &Operator{c}
}

// RaftServer has information about a server in the Raft configuration.
type RaftServer struct {
	// ID is the unique ID for the server. These are currently the same
	// as the address, but they will be changed to a real GUID in a future
	// release of Consul.
	ID string

	// Node is the node name of the server, as known by Consul, or this
	// will be set to "(unknown)" otherwise.
	Node string

	// Address is the IP:port of the server, used for Raft communications.
	Address string

	// Leader is true if this server is the current cluster leader.
	Leader bool

	// Voter is true if this server has a vote in the cluster. This might
	// be false if the server is staging and still coming online, or if
	// it's a non-voting server, which will be added in a future release of
	// Consul.
	Voter bool
}

// RaftConfigration is returned when querying for the current Raft configuration.
type RaftConfiguration struct {
	// Servers has the list of servers in the Raft configuration.
	Servers []*RaftServer

	// Index has the Raft index of this configuration.
	Index uint64
}

// keyringRequest is used for performing Keyring operations
type keyringRequest struct {
	Key string
}

// KeyringResponse is returned when listing the gossip encryption keys
type KeyringResponse struct {
	// Whether this response is for a WAN ring
	WAN bool

	// The datacenter name this request corresponds to
	Datacenter string

	// A map of the encryption keys to the number of nodes they're installed on
	Keys map[string]int

	// The total number of nodes in this ring
	NumNodes int
}

// RaftGetConfiguration is used to query the current Raft peer set.
func (op *Operator) RaftGetConfiguration(q *QueryOptions) (*RaftConfiguration, error) {
	r := op.c.newRequest("GET", "/v1/operator/raft/configuration")
	r.setQueryOptions(q)
	_, resp, err := requireOK(op.c.doRequest(r))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var out RaftConfiguration
	if err := decodeBody(resp, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// RaftRemovePeerByAddress is used to kick a stale peer (one that it in the Raft
// quorum but no longer known to Serf or the catalog) by address in the form of
// "IP:port".
func (op *Operator) RaftRemovePeerByAddress(address string, q *WriteOptions) error {
	r := op.c.newRequest("DELETE", "/v1/operator/raft/peer")
	r.setWriteOptions(q)

	// TODO (slackpad) Currently we made address a query parameter. Once
	// IDs are in place this will be DELETE /v1/operator/raft/peer/<id>.
	r.params.Set("address", string(address))

	_, resp, err := requireOK(op.c.doRequest(r))
	if err != nil {
		return err
	}

	resp.Body.Close()
	return nil
}

// KeyringInstall is used to install a new gossip encryption key into the cluster
func (op *Operator) KeyringInstall(key string, q *WriteOptions) error {
	r := op.c.newRequest("POST", "/v1/operator/keyring")
	r.setWriteOptions(q)
	r.obj = keyringRequest{
		Key: key,
	}
	_, resp, err := requireOK(op.c.doRequest(r))
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// KeyringList is used to list the gossip keys installed in the cluster
func (op *Operator) KeyringList(q *QueryOptions) ([]*KeyringResponse, error) {
	r := op.c.newRequest("GET", "/v1/operator/keyring")
	r.setQueryOptions(q)
	_, resp, err := requireOK(op.c.doRequest(r))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var out []*KeyringResponse
	if err := decodeBody(resp, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// KeyringRemove is used to remove a gossip encryption key from the cluster
func (op *Operator) KeyringRemove(key string, q *WriteOptions) error {
	r := op.c.newRequest("DELETE", "/v1/operator/keyring")
	r.setWriteOptions(q)
	r.obj = keyringRequest{
		Key: key,
	}
	_, resp, err := requireOK(op.c.doRequest(r))
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// KeyringUse is used to change the active gossip encryption key
func (op *Operator) KeyringUse(key string, q *WriteOptions) error {
	r := op.c.newRequest("PUT", "/v1/operator/keyring")
	r.setWriteOptions(q)
	r.obj = keyringRequest{
		Key: key,
	}
	_, resp, err := requireOK(op.c.doRequest(r))
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}
