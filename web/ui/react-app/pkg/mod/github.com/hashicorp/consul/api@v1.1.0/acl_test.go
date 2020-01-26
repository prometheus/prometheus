package api

import (
	"strings"
	"testing"

	"github.com/hashicorp/consul/sdk/testutil/retry"

	"github.com/stretchr/testify/require"
)

func TestAPI_ACLBootstrap(t *testing.T) {
	// TODO (slackpad) We currently can't inject the version, and the
	// version in the binary depends on Git tags, so we can't reliably
	// test this until we are just running an agent in-process here and
	// have full control over the config.
}

func TestAPI_ACLCreateDestroy(t *testing.T) {
	t.Parallel()
	c, s := makeACLClient(t)
	defer s.Stop()
	s.WaitForSerfCheck(t)

	acl := c.ACL()

	ae := ACLEntry{
		Name:  "API test",
		Type:  ACLClientType,
		Rules: `key "" { policy = "deny" }`,
	}

	id, wm, err := acl.Create(&ae, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if wm.RequestTime == 0 {
		t.Fatalf("bad: %v", wm)
	}

	if id == "" {
		t.Fatalf("invalid: %v", id)
	}

	ae2, _, err := acl.Info(id, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if ae2.Name != ae.Name || ae2.Type != ae.Type || ae2.Rules != ae.Rules {
		t.Fatalf("Bad: %#v", ae2)
	}

	wm, err = acl.Destroy(id, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if wm.RequestTime == 0 {
		t.Fatalf("bad: %v", wm)
	}
}

func TestAPI_ACLCloneDestroy(t *testing.T) {
	t.Parallel()
	c, s := makeACLClient(t)
	defer s.Stop()

	acl := c.ACL()

	id, wm, err := acl.Clone(c.config.Token, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if wm.RequestTime == 0 {
		t.Fatalf("bad: %v", wm)
	}

	if id == "" {
		t.Fatalf("invalid: %v", id)
	}

	wm, err = acl.Destroy(id, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if wm.RequestTime == 0 {
		t.Fatalf("bad: %v", wm)
	}
}

func TestAPI_ACLInfo(t *testing.T) {
	t.Parallel()
	c, s := makeACLClient(t)
	defer s.Stop()

	acl := c.ACL()

	ae, qm, err := acl.Info(c.config.Token, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if qm.LastIndex == 0 {
		t.Fatalf("bad: %v", qm)
	}
	if !qm.KnownLeader {
		t.Fatalf("bad: %v", qm)
	}

	if ae == nil || ae.ID != c.config.Token || ae.Type != ACLManagementType {
		t.Fatalf("bad: %#v", ae)
	}
}

func TestAPI_ACLList(t *testing.T) {
	t.Parallel()
	c, s := makeACLClient(t)
	defer s.Stop()

	acl := c.ACL()

	acls, qm, err := acl.List(nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	// anon token is a new token
	if len(acls) < 1 {
		t.Fatalf("bad: %v", acls)
	}

	if qm.LastIndex == 0 {
		t.Fatalf("bad: %v", qm)
	}
	if !qm.KnownLeader {
		t.Fatalf("bad: %v", qm)
	}
}

func TestAPI_ACLReplication(t *testing.T) {
	t.Parallel()
	c, s := makeACLClient(t)
	defer s.Stop()

	acl := c.ACL()

	repl, qm, err := acl.Replication(nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if repl == nil {
		t.Fatalf("bad: %v", repl)
	}

	if repl.Running {
		t.Fatal("bad: repl should not be running")
	}

	if repl.Enabled {
		t.Fatal("bad: repl should not be enabled")
	}

	if qm.RequestTime == 0 {
		t.Fatalf("bad: %v", qm)
	}
}

func TestAPI_ACLPolicy_CreateReadDelete(t *testing.T) {
	t.Parallel()
	c, s := makeACLClient(t)
	defer s.Stop()

	acl := c.ACL()

	created, wm, err := acl.PolicyCreate(&ACLPolicy{
		Name:        "test-policy",
		Description: "test-policy description",
		Rules:       `node_prefix "" { policy = "read" }`,
		Datacenters: []string{"dc1"},
	}, nil)

	require.NoError(t, err)
	require.NotNil(t, created)
	require.NotEqual(t, "", created.ID)
	require.NotEqual(t, 0, wm.RequestTime)

	read, qm, err := acl.PolicyRead(created.ID, nil)
	require.NoError(t, err)
	require.NotEqual(t, 0, qm.LastIndex)
	require.True(t, qm.KnownLeader)

	require.Equal(t, created, read)

	wm, err = acl.PolicyDelete(created.ID, nil)
	require.NoError(t, err)
	require.NotEqual(t, 0, wm.RequestTime)

	read, _, err = acl.PolicyRead(created.ID, nil)
	require.Nil(t, read)
	require.Error(t, err)
}

func TestAPI_ACLPolicy_CreateUpdate(t *testing.T) {
	t.Parallel()
	c, s := makeACLClient(t)
	defer s.Stop()

	acl := c.ACL()

	created, _, err := acl.PolicyCreate(&ACLPolicy{
		Name:        "test-policy",
		Description: "test-policy description",
		Rules:       `node_prefix "" { policy = "read" }`,
		Datacenters: []string{"dc1"},
	}, nil)

	require.NoError(t, err)
	require.NotNil(t, created)
	require.NotEqual(t, "", created.ID)

	read, _, err := acl.PolicyRead(created.ID, nil)
	require.NoError(t, err)
	require.Equal(t, created, read)

	read.Rules += ` service_prefix "" { policy = "read" }`
	read.Datacenters = nil

	updated, wm, err := acl.PolicyUpdate(read, nil)
	require.NoError(t, err)
	require.Equal(t, created.ID, updated.ID)
	require.Equal(t, created.Description, updated.Description)
	require.Equal(t, read.Rules, updated.Rules)
	require.Equal(t, created.CreateIndex, updated.CreateIndex)
	require.NotEqual(t, created.ModifyIndex, updated.ModifyIndex)
	require.Nil(t, updated.Datacenters)
	require.NotEqual(t, 0, wm.RequestTime)

	updated_read, _, err := acl.PolicyRead(created.ID, nil)
	require.NoError(t, err)
	require.Equal(t, updated, updated_read)
}

func TestAPI_ACLPolicy_List(t *testing.T) {
	t.Parallel()
	c, s := makeACLClient(t)
	defer s.Stop()

	acl := c.ACL()

	created1, _, err := acl.PolicyCreate(&ACLPolicy{
		Name:        "policy1",
		Description: "policy1 description",
		Rules:       `node_prefix "" { policy = "read" }`,
		Datacenters: []string{"dc1"},
	}, nil)

	require.NoError(t, err)
	require.NotNil(t, created1)
	require.NotEqual(t, "", created1.ID)

	created2, _, err := acl.PolicyCreate(&ACLPolicy{
		Name:        "policy2",
		Description: "policy2 description",
		Rules:       `service "app" { policy = "write" }`,
		Datacenters: []string{"dc1", "dc2"},
	}, nil)

	require.NoError(t, err)
	require.NotNil(t, created2)
	require.NotEqual(t, "", created2.ID)

	created3, _, err := acl.PolicyCreate(&ACLPolicy{
		Name:        "policy3",
		Description: "policy3 description",
		Rules:       `acl = "read"`,
	}, nil)

	require.NoError(t, err)
	require.NotNil(t, created3)
	require.NotEqual(t, "", created3.ID)

	policies, qm, err := acl.PolicyList(nil)
	require.NoError(t, err)
	require.Len(t, policies, 4)
	require.NotEqual(t, 0, qm.LastIndex)
	require.True(t, qm.KnownLeader)

	policyMap := make(map[string]*ACLPolicyListEntry)
	for _, policy := range policies {
		policyMap[policy.ID] = policy
	}

	policy1, ok := policyMap[created1.ID]
	require.True(t, ok)
	require.NotNil(t, policy1)
	require.Equal(t, created1.Name, policy1.Name)
	require.Equal(t, created1.Description, policy1.Description)
	require.Equal(t, created1.CreateIndex, policy1.CreateIndex)
	require.Equal(t, created1.ModifyIndex, policy1.ModifyIndex)
	require.Equal(t, created1.Hash, policy1.Hash)
	require.ElementsMatch(t, created1.Datacenters, policy1.Datacenters)

	policy2, ok := policyMap[created2.ID]
	require.True(t, ok)
	require.NotNil(t, policy2)
	require.Equal(t, created2.Name, policy2.Name)
	require.Equal(t, created2.Description, policy2.Description)
	require.Equal(t, created2.CreateIndex, policy2.CreateIndex)
	require.Equal(t, created2.ModifyIndex, policy2.ModifyIndex)
	require.Equal(t, created2.Hash, policy2.Hash)
	require.ElementsMatch(t, created2.Datacenters, policy2.Datacenters)

	policy3, ok := policyMap[created3.ID]
	require.True(t, ok)
	require.NotNil(t, policy3)
	require.Equal(t, created3.Name, policy3.Name)
	require.Equal(t, created3.Description, policy3.Description)
	require.Equal(t, created3.CreateIndex, policy3.CreateIndex)
	require.Equal(t, created3.ModifyIndex, policy3.ModifyIndex)
	require.Equal(t, created3.Hash, policy3.Hash)
	require.ElementsMatch(t, created3.Datacenters, policy3.Datacenters)

	// make sure the 4th policy is the global management
	policy4, ok := policyMap["00000000-0000-0000-0000-000000000001"]
	require.True(t, ok)
	require.NotNil(t, policy4)
}

func prepTokenPolicies(t *testing.T, acl *ACL) (policies []*ACLPolicy) {
	policy, _, err := acl.PolicyCreate(&ACLPolicy{
		Name:        "one",
		Description: "one description",
		Rules:       `acl = "read"`,
		Datacenters: []string{"dc1", "dc2"},
	}, nil)

	require.NoError(t, err)
	require.NotNil(t, policy)
	policies = append(policies, policy)

	policy, _, err = acl.PolicyCreate(&ACLPolicy{
		Name:        "two",
		Description: "two description",
		Rules:       `node_prefix "" { policy = "read" }`,
		Datacenters: []string{"dc1", "dc2"},
	}, nil)

	require.NoError(t, err)
	require.NotNil(t, policy)
	policies = append(policies, policy)

	policy, _, err = acl.PolicyCreate(&ACLPolicy{
		Name:        "three",
		Description: "three description",
		Rules:       `service_prefix "" { policy = "read" }`,
	}, nil)

	require.NoError(t, err)
	require.NotNil(t, policy)
	policies = append(policies, policy)

	policy, _, err = acl.PolicyCreate(&ACLPolicy{
		Name:        "four",
		Description: "four description",
		Rules:       `agent "foo" { policy = "write" }`,
	}, nil)

	require.NoError(t, err)
	require.NotNil(t, policy)
	policies = append(policies, policy)
	return
}

func TestAPI_ACLToken_CreateReadDelete(t *testing.T) {
	t.Parallel()
	c, s := makeACLClient(t)
	defer s.Stop()

	acl := c.ACL()

	policies := prepTokenPolicies(t, acl)

	created, wm, err := acl.TokenCreate(&ACLToken{
		Description: "token created",
		Policies: []*ACLTokenPolicyLink{
			&ACLTokenPolicyLink{
				ID: policies[0].ID,
			},
			&ACLTokenPolicyLink{
				ID: policies[1].ID,
			},
			&ACLTokenPolicyLink{
				Name: policies[2].Name,
			},
			&ACLTokenPolicyLink{
				Name: policies[3].Name,
			},
		},
	}, nil)

	require.NoError(t, err)
	require.NotNil(t, created)
	require.NotEqual(t, "", created.AccessorID)
	require.NotEqual(t, "", created.SecretID)
	require.NotEqual(t, 0, wm.RequestTime)

	read, qm, err := acl.TokenRead(created.AccessorID, nil)
	require.NoError(t, err)
	require.Equal(t, created, read)
	require.NotEqual(t, 0, qm.LastIndex)
	require.True(t, qm.KnownLeader)

	acl.c.config.Token = created.SecretID
	self, _, err := acl.TokenReadSelf(nil)
	require.NoError(t, err)
	require.Equal(t, created, self)
	acl.c.config.Token = "root"

	_, err = acl.TokenDelete(created.AccessorID, nil)
	require.NoError(t, err)

	read, _, err = acl.TokenRead(created.AccessorID, nil)
	require.Nil(t, read)
	require.Error(t, err)
}

func TestAPI_ACLToken_CreateUpdate(t *testing.T) {
	t.Parallel()
	c, s := makeACLClient(t)
	defer s.Stop()

	acl := c.ACL()

	policies := prepTokenPolicies(t, acl)

	created, _, err := acl.TokenCreate(&ACLToken{
		Description: "token created",
		Policies: []*ACLTokenPolicyLink{
			&ACLTokenPolicyLink{
				ID: policies[0].ID,
			},
			&ACLTokenPolicyLink{
				Name: policies[2].Name,
			},
		},
	}, nil)

	require.NoError(t, err)
	require.NotNil(t, created)
	require.NotEqual(t, "", created.AccessorID)
	require.NotEqual(t, "", created.SecretID)

	read, _, err := acl.TokenRead(created.AccessorID, nil)
	require.NoError(t, err)
	require.Equal(t, created, read)

	read.Policies = append(read.Policies, &ACLTokenPolicyLink{ID: policies[1].ID})
	read.Policies = append(read.Policies, &ACLTokenPolicyLink{Name: policies[2].Name})

	expectedPolicies := []*ACLTokenPolicyLink{
		&ACLTokenPolicyLink{
			ID:   policies[0].ID,
			Name: policies[0].Name,
		},
		&ACLTokenPolicyLink{
			ID:   policies[1].ID,
			Name: policies[1].Name,
		},
		&ACLTokenPolicyLink{
			ID:   policies[2].ID,
			Name: policies[2].Name,
		},
	}

	updated, wm, err := acl.TokenUpdate(read, nil)
	require.NoError(t, err)
	require.Equal(t, created.AccessorID, updated.AccessorID)
	require.Equal(t, created.SecretID, updated.SecretID)
	require.Equal(t, created.Description, updated.Description)
	require.Equal(t, created.CreateIndex, updated.CreateIndex)
	require.NotEqual(t, created.ModifyIndex, updated.ModifyIndex)
	require.ElementsMatch(t, expectedPolicies, updated.Policies)
	require.NotEqual(t, 0, wm.RequestTime)

	updated_read, _, err := acl.TokenRead(created.AccessorID, nil)
	require.NoError(t, err)
	require.Equal(t, updated, updated_read)
}

func TestAPI_ACLToken_List(t *testing.T) {
	t.Parallel()
	c, s := makeACLClient(t)
	defer s.Stop()

	acl := c.ACL()
	s.WaitForSerfCheck(t)

	policies := prepTokenPolicies(t, acl)

	created1, _, err := acl.TokenCreate(&ACLToken{
		Description: "token created1",
		Policies: []*ACLTokenPolicyLink{
			&ACLTokenPolicyLink{
				ID: policies[0].ID,
			},
		},
	}, nil)

	require.NoError(t, err)
	require.NotNil(t, created1)
	require.NotEqual(t, "", created1.AccessorID)
	require.NotEqual(t, "", created1.SecretID)

	created2, _, err := acl.TokenCreate(&ACLToken{
		Description: "token created2",
		Policies: []*ACLTokenPolicyLink{
			&ACLTokenPolicyLink{
				ID: policies[1].ID,
			},
		},
	}, nil)

	require.NoError(t, err)
	require.NotNil(t, created2)
	require.NotEqual(t, "", created2.AccessorID)
	require.NotEqual(t, "", created2.SecretID)

	created3, _, err := acl.TokenCreate(&ACLToken{
		Description: "token created3",
		Policies: []*ACLTokenPolicyLink{
			&ACLTokenPolicyLink{
				ID: policies[2].ID,
			},
		},
	}, nil)

	require.NoError(t, err)
	require.NotNil(t, created3)
	require.NotEqual(t, "", created3.AccessorID)
	require.NotEqual(t, "", created3.SecretID)

	tokens, qm, err := acl.TokenList(nil)
	require.NoError(t, err)
	// 3 + anon + master
	require.Len(t, tokens, 5)
	require.NotEqual(t, 0, qm.LastIndex)
	require.True(t, qm.KnownLeader)

	tokenMap := make(map[string]*ACLTokenListEntry)
	for _, token := range tokens {
		tokenMap[token.AccessorID] = token
	}

	token1, ok := tokenMap[created1.AccessorID]
	require.True(t, ok)
	require.NotNil(t, token1)
	require.Equal(t, created1.Description, token1.Description)
	require.Equal(t, created1.CreateIndex, token1.CreateIndex)
	require.Equal(t, created1.ModifyIndex, token1.ModifyIndex)
	require.Equal(t, created1.Hash, token1.Hash)
	require.ElementsMatch(t, created1.Policies, token1.Policies)

	token2, ok := tokenMap[created2.AccessorID]
	require.True(t, ok)
	require.NotNil(t, token2)
	require.Equal(t, created2.Description, token2.Description)
	require.Equal(t, created2.CreateIndex, token2.CreateIndex)
	require.Equal(t, created2.ModifyIndex, token2.ModifyIndex)
	require.Equal(t, created2.Hash, token2.Hash)
	require.ElementsMatch(t, created2.Policies, token2.Policies)

	token3, ok := tokenMap[created3.AccessorID]
	require.True(t, ok)
	require.NotNil(t, token3)
	require.Equal(t, created3.Description, token3.Description)
	require.Equal(t, created3.CreateIndex, token3.CreateIndex)
	require.Equal(t, created3.ModifyIndex, token3.ModifyIndex)
	require.Equal(t, created3.Hash, token3.Hash)
	require.ElementsMatch(t, created3.Policies, token3.Policies)

	// make sure the there is an anon token
	token4, ok := tokenMap["00000000-0000-0000-0000-000000000002"]
	require.True(t, ok)
	require.NotNil(t, token4)

	// ensure the 5th token is the root master token
	root, _, err := acl.TokenReadSelf(nil)
	require.NoError(t, err)
	require.NotNil(t, root)
	token5, ok := tokenMap[root.AccessorID]
	require.True(t, ok)
	require.NotNil(t, token5)
}

func TestAPI_ACLToken_Clone(t *testing.T) {
	t.Parallel()
	c, s := makeACLClient(t)
	defer s.Stop()

	acl := c.ACL()

	master, _, err := acl.TokenReadSelf(nil)
	require.NoError(t, err)
	require.NotNil(t, master)

	cloned, _, err := acl.TokenClone(master.AccessorID, "cloned", nil)
	require.NoError(t, err)
	require.NotNil(t, cloned)
	require.NotEqual(t, master.AccessorID, cloned.AccessorID)
	require.NotEqual(t, master.SecretID, cloned.SecretID)
	require.Equal(t, "cloned", cloned.Description)
	require.ElementsMatch(t, master.Policies, cloned.Policies)

	read, _, err := acl.TokenRead(cloned.AccessorID, nil)
	require.NoError(t, err)
	require.NotNil(t, read)
	require.Equal(t, cloned, read)
}

func TestAPI_RulesTranslate_FromToken(t *testing.T) {
	t.Parallel()
	c, s := makeACLClient(t)
	defer s.Stop()

	acl := c.ACL()

	ae := ACLEntry{
		Name:  "API test",
		Type:  ACLClientType,
		Rules: `key "" { policy = "deny" }`,
	}

	id, _, err := acl.Create(&ae, nil)
	require.NoError(t, err)

	var accessor string
	acl.c.config.Token = id

	// This relies on the token upgrade loop running in the background
	// to assign an accessor
	retry.Run(t, func(t *retry.R) {
		token, _, err := acl.TokenReadSelf(nil)
		require.NoError(t, err)
		require.NotEqual(t, "", token.AccessorID)
		accessor = token.AccessorID
	})
	acl.c.config.Token = "root"

	rules, err := acl.RulesTranslateToken(accessor)
	require.NoError(t, err)
	require.Equal(t, "key_prefix \"\" {\n  policy = \"deny\"\n}", rules)
}

func TestAPI_RulesTranslate_Raw(t *testing.T) {
	t.Parallel()
	c, s := makeACLClient(t)
	defer s.Stop()

	acl := c.ACL()

	input := `#start of policy
agent "" {
   policy = "read"
}

node "" {
   policy = "read"
}

service "" {
   policy = "read"
}

key "" {
   policy = "read"
}

session "" {
   policy = "read"
}

event "" {
   policy = "read"
}

query "" {
   policy = "read"
}`

	expected := `#start of policy
agent_prefix "" {
  policy = "read"
}

node_prefix "" {
  policy = "read"
}

service_prefix "" {
  policy = "read"
}

key_prefix "" {
  policy = "read"
}

session_prefix "" {
  policy = "read"
}

event_prefix "" {
  policy = "read"
}

query_prefix "" {
  policy = "read"
}`

	rules, err := acl.RulesTranslate(strings.NewReader(input))
	require.NoError(t, err)
	require.Equal(t, expected, rules)
}
