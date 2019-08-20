package sls

import (
	"fmt"
	"testing"
)

func TestShards(t *testing.T) {
	p := DefaultProject(t)
	shards, err := p.ListShards(TestLogstoreName)
	if err != nil {
		t.Fatalf("error find logstore %v", err)
	}

	fmt.Println(shards)
}

func TestLogstores(t *testing.T) {
	p := DefaultProject(t)
	list, err := p.ListLogstore()
	if err != nil {
		t.Fatalf("TestLogstores error: %v", err)
	}
	fmt.Println(list)
}
