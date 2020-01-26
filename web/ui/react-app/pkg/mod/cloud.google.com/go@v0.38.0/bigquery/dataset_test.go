// Copyright 2015 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package bigquery

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	"github.com/google/go-cmp/cmp"
	bq "google.golang.org/api/bigquery/v2"
	itest "google.golang.org/api/iterator/testing"
)

// readServiceStub services read requests by returning data from an in-memory list of values.
type listTablesStub struct {
	expectedProject, expectedDataset string
	tables                           []*bq.TableListTables
}

func (s *listTablesStub) listTables(it *TableIterator, pageSize int, pageToken string) (*bq.TableList, error) {
	if it.dataset.ProjectID != s.expectedProject {
		return nil, errors.New("wrong project id")
	}
	if it.dataset.DatasetID != s.expectedDataset {
		return nil, errors.New("wrong dataset id")
	}
	const maxPageSize = 2
	if pageSize <= 0 || pageSize > maxPageSize {
		pageSize = maxPageSize
	}
	start := 0
	if pageToken != "" {
		var err error
		start, err = strconv.Atoi(pageToken)
		if err != nil {
			return nil, err
		}
	}
	end := start + pageSize
	if end > len(s.tables) {
		end = len(s.tables)
	}
	nextPageToken := ""
	if end < len(s.tables) {
		nextPageToken = strconv.Itoa(end)
	}
	return &bq.TableList{
		Tables:        s.tables[start:end],
		NextPageToken: nextPageToken,
	}, nil
}

func TestTables(t *testing.T) {
	c := &Client{projectID: "p1"}
	inTables := []*bq.TableListTables{
		{TableReference: &bq.TableReference{ProjectId: "p1", DatasetId: "d1", TableId: "t1"}},
		{TableReference: &bq.TableReference{ProjectId: "p1", DatasetId: "d1", TableId: "t2"}},
		{TableReference: &bq.TableReference{ProjectId: "p1", DatasetId: "d1", TableId: "t3"}},
	}
	outTables := []*Table{
		{ProjectID: "p1", DatasetID: "d1", TableID: "t1", c: c},
		{ProjectID: "p1", DatasetID: "d1", TableID: "t2", c: c},
		{ProjectID: "p1", DatasetID: "d1", TableID: "t3", c: c},
	}

	lts := &listTablesStub{
		expectedProject: "p1",
		expectedDataset: "d1",
		tables:          inTables,
	}
	old := listTables
	listTables = lts.listTables // cannot use t.Parallel with this test
	defer func() { listTables = old }()

	msg, ok := itest.TestIterator(outTables,
		func() interface{} { return c.Dataset("d1").Tables(context.Background()) },
		func(it interface{}) (interface{}, error) { return it.(*TableIterator).Next() })
	if !ok {
		t.Error(msg)
	}
}

type listDatasetsStub struct {
	expectedProject string
	datasets        []*bq.DatasetListDatasets
	hidden          map[*bq.DatasetListDatasets]bool
}

func (s *listDatasetsStub) listDatasets(it *DatasetIterator, pageSize int, pageToken string) (*bq.DatasetList, error) {
	const maxPageSize = 2
	if pageSize <= 0 || pageSize > maxPageSize {
		pageSize = maxPageSize
	}
	if it.Filter != "" {
		return nil, errors.New("filter not supported")
	}
	if it.ProjectID != s.expectedProject {
		return nil, errors.New("bad project ID")
	}
	start := 0
	if pageToken != "" {
		var err error
		start, err = strconv.Atoi(pageToken)
		if err != nil {
			return nil, err
		}
	}
	var (
		i             int
		result        []*bq.DatasetListDatasets
		nextPageToken string
	)
	for i = start; len(result) < pageSize && i < len(s.datasets); i++ {
		if s.hidden[s.datasets[i]] && !it.ListHidden {
			continue
		}
		result = append(result, s.datasets[i])
	}
	if i < len(s.datasets) {
		nextPageToken = strconv.Itoa(i)
	}
	return &bq.DatasetList{
		Datasets:      result,
		NextPageToken: nextPageToken,
	}, nil
}

func TestDatasets(t *testing.T) {
	client := &Client{projectID: "p"}
	inDatasets := []*bq.DatasetListDatasets{
		{DatasetReference: &bq.DatasetReference{ProjectId: "p", DatasetId: "a"}},
		{DatasetReference: &bq.DatasetReference{ProjectId: "p", DatasetId: "b"}},
		{DatasetReference: &bq.DatasetReference{ProjectId: "p", DatasetId: "hidden"}},
		{DatasetReference: &bq.DatasetReference{ProjectId: "p", DatasetId: "c"}},
	}
	outDatasets := []*Dataset{
		{"p", "a", client},
		{"p", "b", client},
		{"p", "hidden", client},
		{"p", "c", client},
	}
	lds := &listDatasetsStub{
		expectedProject: "p",
		datasets:        inDatasets,
		hidden:          map[*bq.DatasetListDatasets]bool{inDatasets[2]: true},
	}
	old := listDatasets
	listDatasets = lds.listDatasets // cannot use t.Parallel with this test
	defer func() { listDatasets = old }()

	msg, ok := itest.TestIterator(outDatasets,
		func() interface{} { it := client.Datasets(context.Background()); it.ListHidden = true; return it },
		func(it interface{}) (interface{}, error) { return it.(*DatasetIterator).Next() })
	if !ok {
		t.Fatalf("ListHidden=true: %s", msg)
	}

	msg, ok = itest.TestIterator([]*Dataset{outDatasets[0], outDatasets[1], outDatasets[3]},
		func() interface{} { it := client.Datasets(context.Background()); it.ListHidden = false; return it },
		func(it interface{}) (interface{}, error) { return it.(*DatasetIterator).Next() })
	if !ok {
		t.Fatalf("ListHidden=false: %s", msg)
	}
}

func TestDatasetToBQ(t *testing.T) {
	for _, test := range []struct {
		in   *DatasetMetadata
		want *bq.Dataset
	}{
		{nil, &bq.Dataset{}},
		{&DatasetMetadata{Name: "name"}, &bq.Dataset{FriendlyName: "name"}},
		{&DatasetMetadata{
			Name:                   "name",
			Description:            "desc",
			DefaultTableExpiration: time.Hour,
			Location:               "EU",
			Labels:                 map[string]string{"x": "y"},
			Access:                 []*AccessEntry{{Role: OwnerRole, Entity: "example.com", EntityType: DomainEntity}},
		}, &bq.Dataset{
			FriendlyName:             "name",
			Description:              "desc",
			DefaultTableExpirationMs: 60 * 60 * 1000,
			Location:                 "EU",
			Labels:                   map[string]string{"x": "y"},
			Access:                   []*bq.DatasetAccess{{Role: "OWNER", Domain: "example.com"}},
		}},
	} {
		got, err := test.in.toBQ()
		if err != nil {
			t.Fatal(err)
		}
		if !testutil.Equal(got, test.want) {
			t.Errorf("%v:\ngot  %+v\nwant %+v", test.in, got, test.want)
		}
	}

	// Check that non-writeable fields are unset.
	aTime := time.Date(2017, 1, 26, 0, 0, 0, 0, time.Local)
	for _, dm := range []*DatasetMetadata{
		{CreationTime: aTime},
		{LastModifiedTime: aTime},
		{FullID: "x"},
		{ETag: "e"},
	} {
		if _, err := dm.toBQ(); err == nil {
			t.Errorf("%+v: got nil, want error", dm)
		}
	}
}

func TestBQToDatasetMetadata(t *testing.T) {
	cTime := time.Date(2017, 1, 26, 0, 0, 0, 0, time.Local)
	cMillis := cTime.UnixNano() / 1e6
	mTime := time.Date(2017, 10, 31, 0, 0, 0, 0, time.Local)
	mMillis := mTime.UnixNano() / 1e6
	q := &bq.Dataset{
		CreationTime:             cMillis,
		LastModifiedTime:         mMillis,
		FriendlyName:             "name",
		Description:              "desc",
		DefaultTableExpirationMs: 60 * 60 * 1000,
		Location:                 "EU",
		Labels:                   map[string]string{"x": "y"},
		Access: []*bq.DatasetAccess{
			{Role: "READER", UserByEmail: "joe@example.com"},
			{Role: "WRITER", GroupByEmail: "users@example.com"},
		},
		Etag: "etag",
	}
	want := &DatasetMetadata{
		CreationTime:           cTime,
		LastModifiedTime:       mTime,
		Name:                   "name",
		Description:            "desc",
		DefaultTableExpiration: time.Hour,
		Location:               "EU",
		Labels:                 map[string]string{"x": "y"},
		Access: []*AccessEntry{
			{Role: ReaderRole, Entity: "joe@example.com", EntityType: UserEmailEntity},
			{Role: WriterRole, Entity: "users@example.com", EntityType: GroupEmailEntity},
		},
		ETag: "etag",
	}
	got, err := bqToDatasetMetadata(q)
	if err != nil {
		t.Fatal(err)
	}
	if diff := testutil.Diff(got, want); diff != "" {
		t.Errorf("-got, +want:\n%s", diff)
	}
}

func TestDatasetMetadataToUpdateToBQ(t *testing.T) {
	dm := DatasetMetadataToUpdate{
		Description:            "desc",
		Name:                   "name",
		DefaultTableExpiration: time.Hour,
	}
	dm.SetLabel("label", "value")
	dm.DeleteLabel("del")

	got, err := dm.toBQ()
	if err != nil {
		t.Fatal(err)
	}
	want := &bq.Dataset{
		Description:              "desc",
		FriendlyName:             "name",
		DefaultTableExpirationMs: 60 * 60 * 1000,
		Labels:                   map[string]string{"label": "value"},
		ForceSendFields:          []string{"Description", "FriendlyName"},
		NullFields:               []string{"Labels.del"},
	}
	if diff := testutil.Diff(got, want); diff != "" {
		t.Errorf("-got, +want:\n%s", diff)
	}
}

func TestConvertAccessEntry(t *testing.T) {
	c := &Client{projectID: "pid"}
	for _, e := range []*AccessEntry{
		{Role: ReaderRole, Entity: "e", EntityType: DomainEntity},
		{Role: WriterRole, Entity: "e", EntityType: GroupEmailEntity},
		{Role: OwnerRole, Entity: "e", EntityType: UserEmailEntity},
		{Role: ReaderRole, Entity: "e", EntityType: SpecialGroupEntity},
		{Role: ReaderRole, EntityType: ViewEntity,
			View: &Table{ProjectID: "p", DatasetID: "d", TableID: "t", c: c}},
	} {
		q, err := e.toBQ()
		if err != nil {
			t.Fatal(err)
		}
		got, err := bqToAccessEntry(q, c)
		if err != nil {
			t.Fatal(err)
		}
		if diff := testutil.Diff(got, e, cmp.AllowUnexported(Table{}, Client{})); diff != "" {
			t.Errorf("got=-, want=+:\n%s", diff)
		}
	}

	e := &AccessEntry{Role: ReaderRole, Entity: "e"}
	if _, err := e.toBQ(); err == nil {
		t.Error("got nil, want error")
	}
	if _, err := bqToAccessEntry(&bq.DatasetAccess{Role: "WRITER"}, nil); err == nil {
		t.Error("got nil, want error")
	}
}
