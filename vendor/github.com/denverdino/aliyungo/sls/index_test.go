package sls

import (
	"fmt"
	"strings"
	"testing"
)

func TestCreateIndex(t *testing.T) {
	p := DefaultProject(t)
	config := &IndexConfig{
		TTL: 7,
		LineConfig: IndexLineConfig{
			TokenList:     []string{",", "\t", "\n", " ", ";"},
			CaseSensitive: false,
		},
	}
	if err := p.CreateIndex(TestLogstoreName, config); err != nil {
		if e, ok := err.(*Error); ok && strings.Contains(e.Message, "already created") {
			//empty
		} else {
			t.Fatalf("fail create index: %s", err)
		}
	}

	i2, err := p.GetIndex(TestLogstoreName)
	if err != nil {
		t.Fatalf("fail get index: %s", err)
	}

	fmt.Println(i2)
	if i2.TTL != 7 {
		t.Fatalf("Expect ttl of index: %d, Actual %d", 7, i2.TTL)
	}

	if err := p.DeleteIndex(TestLogstoreName); err != nil {
		t.Fatalf("fail delete index: %s", err)
	}

}
