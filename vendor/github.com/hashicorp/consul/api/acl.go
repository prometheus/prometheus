package api

import (
	"fmt"
	"io"
	"io/ioutil"
	"time"
)

const (
	// ACLClientType is the client type token
	ACLClientType = "client"

	// ACLManagementType is the management type token
	ACLManagementType = "management"
)

type ACLTokenPolicyLink struct {
	ID   string
	Name string
}

// ACLToken represents an ACL Token
type ACLToken struct {
	CreateIndex uint64
	ModifyIndex uint64
	AccessorID  string
	SecretID    string
	Description string
	Policies    []*ACLTokenPolicyLink
	Local       bool
	CreateTime  time.Time `json:",omitempty"`
	Hash        []byte    `json:",omitempty"`

	// DEPRECATED (ACL-Legacy-Compat)
	// Rules will only be present for legacy tokens returned via the new APIs
	Rules string `json:",omitempty"`
}

type ACLTokenListEntry struct {
	CreateIndex uint64
	ModifyIndex uint64
	AccessorID  string
	Description string
	Policies    []*ACLTokenPolicyLink
	Local       bool
	CreateTime  time.Time
	Hash        []byte
	Legacy      bool
}

// ACLEntry is used to represent a legacy ACL token
// The legacy tokens are deprecated.
type ACLEntry struct {
	CreateIndex uint64
	ModifyIndex uint64
	ID          string
	Name        string
	Type        string
	Rules       string
}

// ACLReplicationStatus is used to represent the status of ACL replication.
type ACLReplicationStatus struct {
	Enabled              bool
	Running              bool
	SourceDatacenter     string
	ReplicationType      string
	ReplicatedIndex      uint64
	ReplicatedTokenIndex uint64
	LastSuccess          time.Time
	LastError            time.Time
}

// ACLPolicy represents an ACL Policy.
type ACLPolicy struct {
	ID          string
	Name        string
	Description string
	Rules       string
	Datacenters []string
	Hash        []byte
	CreateIndex uint64
	ModifyIndex uint64
}

type ACLPolicyListEntry struct {
	ID          string
	Name        string
	Description string
	Datacenters []string
	Hash        []byte
	CreateIndex uint64
	ModifyIndex uint64
}

// ACL can be used to query the ACL endpoints
type ACL struct {
	c *Client
}

// ACL returns a handle to the ACL endpoints
func (c *Client) ACL() *ACL {
	return &ACL{c}
}

// Bootstrap is used to perform a one-time ACL bootstrap operation on a cluster
// to get the first management token.
func (a *ACL) Bootstrap() (*ACLToken, *WriteMeta, error) {
	r := a.c.newRequest("PUT", "/v1/acl/bootstrap")
	rtt, resp, err := requireOK(a.c.doRequest(r))
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	wm := &WriteMeta{RequestTime: rtt}
	var out ACLToken
	if err := decodeBody(resp, &out); err != nil {
		return nil, nil, err
	}
	return &out, wm, nil
}

// Create is used to generate a new token with the given parameters
//
// Deprecated: Use TokenCreate instead.
func (a *ACL) Create(acl *ACLEntry, q *WriteOptions) (string, *WriteMeta, error) {
	r := a.c.newRequest("PUT", "/v1/acl/create")
	r.setWriteOptions(q)
	r.obj = acl
	rtt, resp, err := requireOK(a.c.doRequest(r))
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()

	wm := &WriteMeta{RequestTime: rtt}
	var out struct{ ID string }
	if err := decodeBody(resp, &out); err != nil {
		return "", nil, err
	}
	return out.ID, wm, nil
}

// Update is used to update the rules of an existing token
//
// Deprecated: Use TokenUpdate instead.
func (a *ACL) Update(acl *ACLEntry, q *WriteOptions) (*WriteMeta, error) {
	r := a.c.newRequest("PUT", "/v1/acl/update")
	r.setWriteOptions(q)
	r.obj = acl
	rtt, resp, err := requireOK(a.c.doRequest(r))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	wm := &WriteMeta{RequestTime: rtt}
	return wm, nil
}

// Destroy is used to destroy a given ACL token ID
//
// Deprecated: Use TokenDelete instead.
func (a *ACL) Destroy(id string, q *WriteOptions) (*WriteMeta, error) {
	r := a.c.newRequest("PUT", "/v1/acl/destroy/"+id)
	r.setWriteOptions(q)
	rtt, resp, err := requireOK(a.c.doRequest(r))
	if err != nil {
		return nil, err
	}
	resp.Body.Close()

	wm := &WriteMeta{RequestTime: rtt}
	return wm, nil
}

// Clone is used to return a new token cloned from an existing one
//
// Deprecated: Use TokenClone instead.
func (a *ACL) Clone(id string, q *WriteOptions) (string, *WriteMeta, error) {
	r := a.c.newRequest("PUT", "/v1/acl/clone/"+id)
	r.setWriteOptions(q)
	rtt, resp, err := requireOK(a.c.doRequest(r))
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()

	wm := &WriteMeta{RequestTime: rtt}
	var out struct{ ID string }
	if err := decodeBody(resp, &out); err != nil {
		return "", nil, err
	}
	return out.ID, wm, nil
}

// Info is used to query for information about an ACL token
//
// Deprecated: Use TokenRead instead.
func (a *ACL) Info(id string, q *QueryOptions) (*ACLEntry, *QueryMeta, error) {
	r := a.c.newRequest("GET", "/v1/acl/info/"+id)
	r.setQueryOptions(q)
	rtt, resp, err := requireOK(a.c.doRequest(r))
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	qm := &QueryMeta{}
	parseQueryMeta(resp, qm)
	qm.RequestTime = rtt

	var entries []*ACLEntry
	if err := decodeBody(resp, &entries); err != nil {
		return nil, nil, err
	}
	if len(entries) > 0 {
		return entries[0], qm, nil
	}
	return nil, qm, nil
}

// List is used to get all the ACL tokens
//
// Deprecated: Use TokenList instead.
func (a *ACL) List(q *QueryOptions) ([]*ACLEntry, *QueryMeta, error) {
	r := a.c.newRequest("GET", "/v1/acl/list")
	r.setQueryOptions(q)
	rtt, resp, err := requireOK(a.c.doRequest(r))
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	qm := &QueryMeta{}
	parseQueryMeta(resp, qm)
	qm.RequestTime = rtt

	var entries []*ACLEntry
	if err := decodeBody(resp, &entries); err != nil {
		return nil, nil, err
	}
	return entries, qm, nil
}

// Replication returns the status of the ACL replication process in the datacenter
func (a *ACL) Replication(q *QueryOptions) (*ACLReplicationStatus, *QueryMeta, error) {
	r := a.c.newRequest("GET", "/v1/acl/replication")
	r.setQueryOptions(q)
	rtt, resp, err := requireOK(a.c.doRequest(r))
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	qm := &QueryMeta{}
	parseQueryMeta(resp, qm)
	qm.RequestTime = rtt

	var entries *ACLReplicationStatus
	if err := decodeBody(resp, &entries); err != nil {
		return nil, nil, err
	}
	return entries, qm, nil
}

// TokenCreate creates a new ACL token. It requires that the AccessorID and SecretID fields
// of the ACLToken structure to be empty as these will be filled in by Consul.
func (a *ACL) TokenCreate(token *ACLToken, q *WriteOptions) (*ACLToken, *WriteMeta, error) {
	if token.AccessorID != "" {
		return nil, nil, fmt.Errorf("Cannot specify an AccessorID in Token Creation")
	}

	if token.SecretID != "" {
		return nil, nil, fmt.Errorf("Cannot specify a SecretID in Token Creation")
	}

	r := a.c.newRequest("PUT", "/v1/acl/token")
	r.setWriteOptions(q)
	r.obj = token
	rtt, resp, err := requireOK(a.c.doRequest(r))
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	wm := &WriteMeta{RequestTime: rtt}
	var out ACLToken
	if err := decodeBody(resp, &out); err != nil {
		return nil, nil, err
	}

	return &out, wm, nil
}

// TokenUpdate updates a token in place without modifying its AccessorID or SecretID. A valid
// AccessorID must be set in the ACLToken structure passed to this function but the SecretID may
// be omitted and will be filled in by Consul with its existing value.
func (a *ACL) TokenUpdate(token *ACLToken, q *WriteOptions) (*ACLToken, *WriteMeta, error) {
	if token.AccessorID == "" {
		return nil, nil, fmt.Errorf("Must specify an AccessorID for Token Updating")
	}
	r := a.c.newRequest("PUT", "/v1/acl/token/"+token.AccessorID)
	r.setWriteOptions(q)
	r.obj = token
	rtt, resp, err := requireOK(a.c.doRequest(r))
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	wm := &WriteMeta{RequestTime: rtt}
	var out ACLToken
	if err := decodeBody(resp, &out); err != nil {
		return nil, nil, err
	}

	return &out, wm, nil
}

// TokenClone will create a new token with the same policies and locality as the original
// token but will have its own auto-generated AccessorID and SecretID as well having the
// description passed to this function. The tokenID parameter must be a valid Accessor ID
// of an existing token.
func (a *ACL) TokenClone(tokenID string, description string, q *WriteOptions) (*ACLToken, *WriteMeta, error) {
	if tokenID == "" {
		return nil, nil, fmt.Errorf("Must specify a tokenID for Token Cloning")
	}

	r := a.c.newRequest("PUT", "/v1/acl/token/"+tokenID+"/clone")
	r.setWriteOptions(q)
	r.obj = struct{ Description string }{description}
	rtt, resp, err := requireOK(a.c.doRequest(r))
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	wm := &WriteMeta{RequestTime: rtt}
	var out ACLToken
	if err := decodeBody(resp, &out); err != nil {
		return nil, nil, err
	}

	return &out, wm, nil
}

// TokenDelete removes a single ACL token. The tokenID parameter must be a valid
// Accessor ID of an existing token.
func (a *ACL) TokenDelete(tokenID string, q *WriteOptions) (*WriteMeta, error) {
	r := a.c.newRequest("DELETE", "/v1/acl/token/"+tokenID)
	r.setWriteOptions(q)
	rtt, resp, err := requireOK(a.c.doRequest(r))
	if err != nil {
		return nil, err
	}
	resp.Body.Close()

	wm := &WriteMeta{RequestTime: rtt}
	return wm, nil
}

// TokenRead retrieves the full token details. The tokenID parameter must be a valid
// Accessor ID of an existing token.
func (a *ACL) TokenRead(tokenID string, q *QueryOptions) (*ACLToken, *QueryMeta, error) {
	r := a.c.newRequest("GET", "/v1/acl/token/"+tokenID)
	r.setQueryOptions(q)
	rtt, resp, err := requireOK(a.c.doRequest(r))
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	qm := &QueryMeta{}
	parseQueryMeta(resp, qm)
	qm.RequestTime = rtt

	var out ACLToken
	if err := decodeBody(resp, &out); err != nil {
		return nil, nil, err
	}

	return &out, qm, nil
}

// TokenReadSelf retrieves the full token details of the token currently
// assigned to the API Client. In this manner its possible to read a token
// by its Secret ID.
func (a *ACL) TokenReadSelf(q *QueryOptions) (*ACLToken, *QueryMeta, error) {
	r := a.c.newRequest("GET", "/v1/acl/token/self")
	r.setQueryOptions(q)
	rtt, resp, err := requireOK(a.c.doRequest(r))
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	qm := &QueryMeta{}
	parseQueryMeta(resp, qm)
	qm.RequestTime = rtt

	var out ACLToken
	if err := decodeBody(resp, &out); err != nil {
		return nil, nil, err
	}

	return &out, qm, nil
}

// TokenList lists all tokens. The listing does not contain any SecretIDs as those
// may only be retrieved by a call to TokenRead.
func (a *ACL) TokenList(q *QueryOptions) ([]*ACLTokenListEntry, *QueryMeta, error) {
	r := a.c.newRequest("GET", "/v1/acl/tokens")
	r.setQueryOptions(q)
	rtt, resp, err := requireOK(a.c.doRequest(r))
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	qm := &QueryMeta{}
	parseQueryMeta(resp, qm)
	qm.RequestTime = rtt

	var entries []*ACLTokenListEntry
	if err := decodeBody(resp, &entries); err != nil {
		return nil, nil, err
	}
	return entries, qm, nil
}

// PolicyCreate will create a new policy. It is not allowed for the policy parameters
// ID field to be set as this will be generated by Consul while processing the request.
func (a *ACL) PolicyCreate(policy *ACLPolicy, q *WriteOptions) (*ACLPolicy, *WriteMeta, error) {
	if policy.ID != "" {
		return nil, nil, fmt.Errorf("Cannot specify an ID in Policy Creation")
	}

	r := a.c.newRequest("PUT", "/v1/acl/policy")
	r.setWriteOptions(q)
	r.obj = policy
	rtt, resp, err := requireOK(a.c.doRequest(r))
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	wm := &WriteMeta{RequestTime: rtt}
	var out ACLPolicy
	if err := decodeBody(resp, &out); err != nil {
		return nil, nil, err
	}

	return &out, wm, nil
}

// PolicyUpdate updates a policy. The ID field of the policy parameter must be set to an
// existing policy ID
func (a *ACL) PolicyUpdate(policy *ACLPolicy, q *WriteOptions) (*ACLPolicy, *WriteMeta, error) {
	if policy.ID == "" {
		return nil, nil, fmt.Errorf("Must specify an ID in Policy Creation")
	}

	r := a.c.newRequest("PUT", "/v1/acl/policy/"+policy.ID)
	r.setWriteOptions(q)
	r.obj = policy
	rtt, resp, err := requireOK(a.c.doRequest(r))
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	wm := &WriteMeta{RequestTime: rtt}
	var out ACLPolicy
	if err := decodeBody(resp, &out); err != nil {
		return nil, nil, err
	}

	return &out, wm, nil
}

// PolicyDelete deletes a policy given its ID.
func (a *ACL) PolicyDelete(policyID string, q *WriteOptions) (*WriteMeta, error) {
	r := a.c.newRequest("DELETE", "/v1/acl/policy/"+policyID)
	r.setWriteOptions(q)
	rtt, resp, err := requireOK(a.c.doRequest(r))
	if err != nil {
		return nil, err
	}
	resp.Body.Close()

	wm := &WriteMeta{RequestTime: rtt}
	return wm, nil
}

// PolicyRead retrieves the policy details including the rule set.
func (a *ACL) PolicyRead(policyID string, q *QueryOptions) (*ACLPolicy, *QueryMeta, error) {
	r := a.c.newRequest("GET", "/v1/acl/policy/"+policyID)
	r.setQueryOptions(q)
	rtt, resp, err := requireOK(a.c.doRequest(r))
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	qm := &QueryMeta{}
	parseQueryMeta(resp, qm)
	qm.RequestTime = rtt

	var out ACLPolicy
	if err := decodeBody(resp, &out); err != nil {
		return nil, nil, err
	}

	return &out, qm, nil
}

// PolicyList retrieves a listing of all policies. The listing does not include the
// rules for any policy as those should be retrieved by subsequent calls to PolicyRead.
func (a *ACL) PolicyList(q *QueryOptions) ([]*ACLPolicyListEntry, *QueryMeta, error) {
	r := a.c.newRequest("GET", "/v1/acl/policies")
	r.setQueryOptions(q)
	rtt, resp, err := requireOK(a.c.doRequest(r))
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	qm := &QueryMeta{}
	parseQueryMeta(resp, qm)
	qm.RequestTime = rtt

	var entries []*ACLPolicyListEntry
	if err := decodeBody(resp, &entries); err != nil {
		return nil, nil, err
	}
	return entries, qm, nil
}

// RulesTranslate translates the legacy rule syntax into the current syntax.
//
// Deprecated: Support for the legacy syntax translation will be removed
// when legacy ACL support is removed.
func (a *ACL) RulesTranslate(rules io.Reader) (string, error) {
	r := a.c.newRequest("POST", "/v1/acl/rules/translate")
	r.body = rules
	rtt, resp, err := requireOK(a.c.doRequest(r))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	qm := &QueryMeta{}
	parseQueryMeta(resp, qm)
	qm.RequestTime = rtt

	ruleBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("Failed to read translated rule body: %v", err)
	}

	return string(ruleBytes), nil
}

// RulesTranslateToken translates the rules associated with the legacy syntax
// into the current syntax and returns the results.
//
// Deprecated: Support for the legacy syntax translation will be removed
// when legacy ACL support is removed.
func (a *ACL) RulesTranslateToken(tokenID string) (string, error) {
	r := a.c.newRequest("GET", "/v1/acl/rules/translate/"+tokenID)
	rtt, resp, err := requireOK(a.c.doRequest(r))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	qm := &QueryMeta{}
	parseQueryMeta(resp, qm)
	qm.RequestTime = rtt

	ruleBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("Failed to read translated rule body: %v", err)
	}

	return string(ruleBytes), nil
}
