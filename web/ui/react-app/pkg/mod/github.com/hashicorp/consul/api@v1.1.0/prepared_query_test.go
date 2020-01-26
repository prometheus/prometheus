package api

import (
	"reflect"
	"testing"

	"github.com/hashicorp/consul/sdk/testutil/retry"
)

func TestAPI_PreparedQuery(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	// Set up a node and a service.
	reg := &CatalogRegistration{
		Datacenter: "dc1",
		Node:       "foobar",
		Address:    "192.168.10.10",
		TaggedAddresses: map[string]string{
			"wan": "127.0.0.1",
		},
		NodeMeta: map[string]string{"somekey": "somevalue"},
		Service: &AgentService{
			ID:      "redis1",
			Service: "redis",
			Tags:    []string{"master", "v1"},
			Meta:    map[string]string{"redis-version": "4.0"},
			Port:    8000,
		},
	}

	catalog := c.Catalog()
	retry.Run(t, func(r *retry.R) {
		if _, err := catalog.Register(reg, nil); err != nil {
			r.Fatal(err)
		}
		if _, _, err := catalog.Node("foobar", nil); err != nil {
			r.Fatal(err)
		}
	})

	// Create a simple prepared query.
	def := &PreparedQueryDefinition{
		Name: "test",
		Service: ServiceQuery{
			Service:     "redis",
			NodeMeta:    map[string]string{"somekey": "somevalue"},
			ServiceMeta: map[string]string{"redis-version": "4.0"},
		},
	}

	query := c.PreparedQuery()
	var err error
	def.ID, _, err = query.Create(def, nil)
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	// Read it back.
	defs, _, err := query.Get(def.ID, nil)
	if err != nil {
		t.Fatalf("err: %s", err)
	}
	if len(defs) != 1 || !reflect.DeepEqual(defs[0], def) {
		t.Fatalf("bad: %v", defs)
	}

	// List them all.
	defs, _, err = query.List(nil)
	if err != nil {
		t.Fatalf("err: %s", err)
	}
	if len(defs) != 1 || !reflect.DeepEqual(defs[0], def) {
		t.Fatalf("bad: %v", defs)
	}

	// Make an update.
	def.Name = "my-query"
	_, err = query.Update(def, nil)
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	// Read it back again to verify the update worked.
	defs, _, err = query.Get(def.ID, nil)
	if err != nil {
		t.Fatalf("err: %s", err)
	}
	if len(defs) != 1 || !reflect.DeepEqual(defs[0], def) {
		t.Fatalf("bad: %v", defs)
	}

	// Execute by ID.
	results, _, err := query.Execute(def.ID, nil)
	if err != nil {
		t.Fatalf("err: %s", err)
	}
	if len(results.Nodes) != 1 || results.Nodes[0].Node.Node != "foobar" {
		t.Fatalf("bad: %v", results)
	}
	if wan, ok := results.Nodes[0].Node.TaggedAddresses["wan"]; !ok || wan != "127.0.0.1" {
		t.Fatalf("bad: %v", results)
	}

	// Execute by name.
	results, _, err = query.Execute("my-query", nil)
	if err != nil {
		t.Fatalf("err: %s", err)
	}
	if len(results.Nodes) != 1 || results.Nodes[0].Node.Node != "foobar" {
		t.Fatalf("bad: %v", results)
	}
	if wan, ok := results.Nodes[0].Node.TaggedAddresses["wan"]; !ok || wan != "127.0.0.1" {
		t.Fatalf("bad: %v", results)
	}
	if results.Nodes[0].Node.Datacenter != "dc1" {
		t.Fatalf("bad datacenter: %v", results)
	}

	// Add new node with failing health check.
	reg2 := reg
	reg2.Node = "failingnode"
	reg2.Check = &AgentCheck{
		Node:        "failingnode",
		ServiceID:   "redis1",
		ServiceName: "redis",
		Name:        "failingcheck",
		Status:      "critical",
	}
	retry.Run(t, func(r *retry.R) {
		if _, err := catalog.Register(reg2, nil); err != nil {
			r.Fatal(err)
		}
		if _, _, err := catalog.Node("failingnode", nil); err != nil {
			r.Fatal(err)
		}
	})

	// Execute by ID. Should return only healthy node.
	results, _, err = query.Execute(def.ID, nil)
	if err != nil {
		t.Fatalf("err: %s", err)
	}
	if len(results.Nodes) != 1 || results.Nodes[0].Node.Node != "foobar" {
		t.Fatalf("bad: %v", results)
	}
	if wan, ok := results.Nodes[0].Node.TaggedAddresses["wan"]; !ok || wan != "127.0.0.1" {
		t.Fatalf("bad: %v", results)
	}

	// Update PQ with ignore rule for the failing check
	def.Service.IgnoreCheckIDs = []string{"failingcheck"}
	_, err = query.Update(def, nil)
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	// Execute by ID. Should return BOTH nodes ignoring the failing check.
	results, _, err = query.Execute(def.ID, nil)
	if err != nil {
		t.Fatalf("err: %s", err)
	}
	if len(results.Nodes) != 2 {
		t.Fatalf("got %d nodes, want 2", len(results.Nodes))
	}

	// Delete it.
	_, err = query.Delete(def.ID, nil)
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	// Make sure there are no longer any queries.
	defs, _, err = query.List(nil)
	if err != nil {
		t.Fatalf("err: %s", err)
	}
	if len(defs) != 0 {
		t.Fatalf("bad: %v", defs)
	}
}
