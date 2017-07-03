// Copyright 2012-present Oliver Eilhard. All rights reserved.
// Use of this source code is governed by a MIT-license.
// See http://olivere.mit-license.org/license.txt for details.

package elastic

import "testing"

func TestTasksListBuildURL(t *testing.T) {
	client := setupTestClient(t)

	tests := []struct {
		TaskId   []int64
		Expected string
	}{
		{
			[]int64{},
			"/_tasks",
		},
		{
			[]int64{42},
			"/_tasks/42",
		},
		{
			[]int64{42, 37},
			"/_tasks/42%2C37",
		},
	}

	for i, test := range tests {
		path, _, err := client.TasksList().TaskId(test.TaskId...).buildURL()
		if err != nil {
			t.Errorf("case #%d: %v", i+1, err)
			continue
		}
		if path != test.Expected {
			t.Errorf("case #%d: expected %q; got: %q", i+1, test.Expected, path)
		}
	}
}

func TestTasksList(t *testing.T) {
	client := setupTestClientAndCreateIndexAndAddDocs(t) //, SetTraceLog(log.New(os.Stdout, "", 0)))
	esversion, err := client.ElasticsearchVersion(DefaultURL)
	if err != nil {
		t.Fatal(err)
	}
	if esversion < "2.3.0" {
		t.Skipf("Elasticsearch %v does not support Tasks Management API yet", esversion)
	}

	res, err := client.TasksList().Pretty(true).Do()
	if err != nil {
		t.Fatal(err)
	}
	if res == nil {
		t.Fatal("response is nil")
	}
	if len(res.Nodes) == 0 {
		t.Fatalf("expected at least 1 node; got: %d", len(res.Nodes))
	}
}
