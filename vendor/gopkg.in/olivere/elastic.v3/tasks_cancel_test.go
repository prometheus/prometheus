// Copyright 2012-present Oliver Eilhard. All rights reserved.
// Use of this source code is governed by a MIT-license.
// See http://olivere.mit-license.org/license.txt for details.

package elastic

import (
	"testing"
)

func TestTasksCancelBuildURL(t *testing.T) {
	client := setupTestClient(t)

	// Cancel all
	got, _, err := client.TasksCancel().buildURL()
	if err != nil {
		t.Fatal(err)
	}
	want := "/_tasks/_cancel"
	if got != want {
		t.Errorf("want %q; got %q", want, got)
	}

	// Cancel specific task
	got, _, err = client.TasksCancel().TaskId(42).buildURL()
	if err != nil {
		t.Fatal(err)
	}
	want = "/_tasks/42/_cancel"
	if got != want {
		t.Errorf("want %q; got %q", want, got)
	}
}

/*
func TestTasksCancel(t *testing.T) {
	client := setupTestClientAndCreateIndexAndAddDocs(t)
	esversion, err := client.ElasticsearchVersion(DefaultURL)
	if err != nil {
		t.Fatal(err)
	}
	if esversion < "2.3.0" {
		t.Skipf("Elasticsearch %v does not support Tasks Management API yet", esversion)
	}
	res, err := client.TasksCancel("1").Do()
	if err != nil {
		t.Fatal(err)
	}
	if res == nil {
		t.Fatal("response is nil")
	}
}
*/
