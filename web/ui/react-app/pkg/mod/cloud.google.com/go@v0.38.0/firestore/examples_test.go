// Copyright 2017 Google LLC
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

// TODO(jba): add Output comments to examples when feasible.

package firestore_test

import (
	"context"
	"fmt"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
)

func ExampleNewClient() {
	ctx := context.Background()
	client, err := firestore.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	defer client.Close() // Close client when done.
	_ = client           // TODO: Use client.
}

func ExampleClient_Collection() {
	ctx := context.Background()
	client, err := firestore.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	defer client.Close()
	coll1 := client.Collection("States")
	coll2 := client.Collection("States/NewYork/Cities")
	fmt.Println(coll1, coll2)
}

func ExampleClient_Doc() {
	ctx := context.Background()
	client, err := firestore.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	defer client.Close()
	doc1 := client.Doc("States/NewYork")
	doc2 := client.Doc("States/NewYork/Cities/Albany")
	fmt.Println(doc1, doc2)
}

func ExampleClient_GetAll() {
	ctx := context.Background()
	client, err := firestore.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	defer client.Close()
	docs, err := client.GetAll(ctx, []*firestore.DocumentRef{
		client.Doc("States/NorthCarolina"),
		client.Doc("States/SouthCarolina"),
		client.Doc("States/WestCarolina"),
		client.Doc("States/EastCarolina"),
	})
	if err != nil {
		// TODO: Handle error.
	}
	// docs is a slice with four DocumentSnapshots, but the last two are
	// nil because there is no West or East Carolina.
	fmt.Println(docs)
}

func ExampleClient_Batch() {
	ctx := context.Background()
	client, err := firestore.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	defer client.Close()
	b := client.Batch()
	_ = b // TODO: Use batch.
}

func ExampleWriteBatch_Commit() {
	ctx := context.Background()
	client, err := firestore.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	defer client.Close()

	type State struct {
		Capital    string  `firestore:"capital"`
		Population float64 `firestore:"pop"` // in millions
	}

	ny := client.Doc("States/NewYork")
	ca := client.Doc("States/California")

	writeResults, err := client.Batch().
		Create(ny, State{Capital: "Albany", Population: 19.8}).
		Set(ca, State{Capital: "Sacramento", Population: 39.14}).
		Delete(client.Doc("States/WestDakota")).
		Commit(ctx)
	if err != nil {
		// TODO: Handle error.
	}
	fmt.Println(writeResults)
}

func ExampleCollectionRef_Add() {
	ctx := context.Background()
	client, err := firestore.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	defer client.Close()

	doc, wr, err := client.Collection("Users").Add(ctx, map[string]interface{}{
		"name":  "Alice",
		"email": "aj@example.com",
	})
	if err != nil {
		// TODO: Handle error.
	}
	fmt.Println(doc, wr)
}

func ExampleCollectionRef_Doc() {
	ctx := context.Background()
	client, err := firestore.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	defer client.Close()

	fl := client.Collection("States").Doc("Florida")
	ta := client.Collection("States").Doc("Florida/Cities/Tampa")

	fmt.Println(fl, ta)
}

func ExampleCollectionRef_NewDoc() {
	ctx := context.Background()
	client, err := firestore.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	defer client.Close()

	doc := client.Collection("Users").NewDoc()

	fmt.Println(doc)
}

func ExampleDocumentRef_Collection() {
	ctx := context.Background()
	client, err := firestore.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	defer client.Close()

	mi := client.Collection("States").Doc("Michigan")
	cities := mi.Collection("Cities")

	fmt.Println(cities)
}

func ExampleDocumentRef_Create_map() {
	ctx := context.Background()
	client, err := firestore.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	defer client.Close()

	wr, err := client.Doc("States/Colorado").Create(ctx, map[string]interface{}{
		"capital": "Denver",
		"pop":     5.5,
	})
	if err != nil {
		// TODO: Handle error.
	}
	fmt.Println(wr.UpdateTime)
}

func ExampleDocumentRef_Create_struct() {
	ctx := context.Background()
	client, err := firestore.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	defer client.Close()

	type State struct {
		Capital    string  `firestore:"capital"`
		Population float64 `firestore:"pop"` // in millions
	}

	wr, err := client.Doc("States/Colorado").Create(ctx, State{
		Capital:    "Denver",
		Population: 5.5,
	})
	if err != nil {
		// TODO: Handle error.
	}
	fmt.Println(wr.UpdateTime)
}

func ExampleDocumentRef_Set() {
	ctx := context.Background()
	client, err := firestore.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	defer client.Close()

	// Overwrite the document with the given data. Any other fields currently
	// in the document will be removed.
	wr, err := client.Doc("States/Alabama").Set(ctx, map[string]interface{}{
		"capital": "Montgomery",
		"pop":     4.9,
	})
	if err != nil {
		// TODO: Handle error.
	}
	fmt.Println(wr.UpdateTime)
}

func ExampleDocumentRef_Set_merge() {
	ctx := context.Background()
	client, err := firestore.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	defer client.Close()

	// Overwrite only the fields in the map; preserve all others.
	_, err = client.Doc("States/Alabama").Set(ctx, map[string]interface{}{
		"pop": 5.2,
	}, firestore.MergeAll)
	if err != nil {
		// TODO: Handle error.
	}

	type State struct {
		Capital    string  `firestore:"capital"`
		Population float64 `firestore:"pop"` // in millions
	}

	// To do a merging Set with struct data, specify the exact fields to overwrite.
	// MergeAll is disallowed here, because it would probably be a mistake: the "capital"
	// field would be overwritten with the empty string.
	_, err = client.Doc("States/Alabama").Set(ctx, State{Population: 5.2}, firestore.Merge([]string{"pop"}))
	if err != nil {
		// TODO: Handle error.
	}
}

func ExampleDocumentRef_Update() {
	ctx := context.Background()
	client, err := firestore.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	defer client.Close()

	tenn := client.Doc("States/Tennessee")
	wr, err := tenn.Update(ctx, []firestore.Update{
		{Path: "pop", Value: 6.6},
		{FieldPath: []string{".", "*", "/"}, Value: "odd"},
	})
	if err != nil {
		// TODO: Handle error.
	}
	fmt.Println(wr.UpdateTime)
}

func ExampleDocumentRef_Delete() {
	ctx := context.Background()
	client, err := firestore.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	defer client.Close()

	// Oops, Ontario is a Canadian province...
	if _, err = client.Doc("States/Ontario").Delete(ctx); err != nil {
		// TODO: Handle error.
	}
}

func ExampleDocumentRef_Get() {
	ctx := context.Background()
	client, err := firestore.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	defer client.Close()

	docsnap, err := client.Doc("States/Ohio").Get(ctx)
	if err != nil {
		// TODO: Handle error.
	}
	_ = docsnap // TODO: Use DocumentSnapshot.
}

func ExampleDocumentRef_Snapshots() {
	ctx := context.Background()
	client, err := firestore.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	defer client.Close()
	iter := client.Doc("States/Idaho").Snapshots(ctx)
	defer iter.Stop()
	for {
		docsnap, err := iter.Next()
		if err != nil {
			// TODO: Handle error.
		}
		_ = docsnap // TODO: Use DocumentSnapshot.
	}
}

func ExampleDocumentSnapshot_Data() {
	ctx := context.Background()
	client, err := firestore.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	defer client.Close()

	docsnap, err := client.Doc("States/Ohio").Get(ctx)
	if err != nil {
		// TODO: Handle error.
	}
	ohioMap := docsnap.Data()
	fmt.Println(ohioMap["capital"])
}

func ExampleDocumentSnapshot_DataAt() {
	ctx := context.Background()
	client, err := firestore.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	defer client.Close()

	docsnap, err := client.Doc("States/Ohio").Get(ctx)
	if err != nil {
		// TODO: Handle error.
	}
	cap, err := docsnap.DataAt("capital")
	if err != nil {
		// TODO: Handle error.
	}
	fmt.Println(cap)
}

func ExampleDocumentSnapshot_DataAtPath() {
	ctx := context.Background()
	client, err := firestore.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	defer client.Close()

	docsnap, err := client.Doc("States/Ohio").Get(ctx)
	if err != nil {
		// TODO: Handle error.
	}
	pop, err := docsnap.DataAtPath([]string{"capital", "population"})
	if err != nil {
		// TODO: Handle error.
	}
	fmt.Println(pop)
}

func ExampleDocumentSnapshot_DataTo() {
	ctx := context.Background()
	client, err := firestore.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	defer client.Close()

	docsnap, err := client.Doc("States/Ohio").Get(ctx)
	if err != nil {
		// TODO: Handle error.
	}

	type State struct {
		Capital    string  `firestore:"capital"`
		Population float64 `firestore:"pop"` // in millions
	}

	var s State
	if err := docsnap.DataTo(&s); err != nil {
		// TODO: Handle error.
	}
	fmt.Println(s)
}

func ExampleQuery_Documents() {
	ctx := context.Background()
	client, err := firestore.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	defer client.Close()

	q := client.Collection("States").Select("pop").
		Where("pop", ">", 10).
		OrderBy("pop", firestore.Desc).
		Limit(10)
	iter1 := q.Documents(ctx)
	_ = iter1 // TODO: Use iter1.

	// You can call Documents directly on a CollectionRef as well.
	iter2 := client.Collection("States").Documents(ctx)
	_ = iter2 // TODO: Use iter2.
}

// This example is just like the one above, but illustrates
// how to use the XXXPath methods of Query for field paths
// that can't be expressed as a dot-separated string.
func ExampleQuery_Documents_path_methods() {
	ctx := context.Background()
	client, err := firestore.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	defer client.Close()

	q := client.Collection("Unusual").SelectPaths([]string{"*"}, []string{"[~]"}).
		WherePath([]string{"/"}, ">", 10).
		OrderByPath([]string{"/"}, firestore.Desc).
		Limit(10)
	iter1 := q.Documents(ctx)
	_ = iter1 // TODO: Use iter1.

	// You can call Documents directly on a CollectionRef as well.
	iter2 := client.Collection("States").Documents(ctx)
	_ = iter2 // TODO: Use iter2.
}

func ExampleQuery_Snapshots() {
	ctx := context.Background()
	client, err := firestore.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	defer client.Close()

	q := client.Collection("States").
		Where("pop", ">", 10).
		OrderBy("pop", firestore.Desc).
		Limit(10)
	qsnapIter := q.Snapshots(ctx)
	// Listen forever for changes to the query's results.
	for {
		qsnap, err := qsnapIter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			// TODO: Handle error.
		}
		fmt.Printf("At %s there were %d results.\n", qsnap.ReadTime, qsnap.Size)
		_ = qsnap.Documents // TODO: Iterate over the results if desired.
		_ = qsnap.Changes   // TODO: Use the list of incremental changes if desired.
	}
}

func ExampleDocumentIterator_Next() {
	ctx := context.Background()
	client, err := firestore.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	defer client.Close()

	q := client.Collection("States").
		Where("pop", ">", 10).
		OrderBy("pop", firestore.Desc)
	iter := q.Documents(ctx)
	defer iter.Stop()
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			// TODO: Handle error.
		}
		fmt.Println(doc.Data())
	}
}

func ExampleDocumentIterator_GetAll() {
	ctx := context.Background()
	client, err := firestore.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	defer client.Close()

	q := client.Collection("States").
		Where("pop", ">", 10).
		OrderBy("pop", firestore.Desc).
		Limit(10) // a good idea with GetAll, to avoid filling memory
	docs, err := q.Documents(ctx).GetAll()
	if err != nil {
		// TODO: Handle error.
	}
	for _, doc := range docs {
		fmt.Println(doc.Data())
	}
}

func ExampleClient_RunTransaction() {
	ctx := context.Background()
	client, err := firestore.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	defer client.Close()

	nm := client.Doc("States/NewMexico")
	err = client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		doc, err := tx.Get(nm) // tx.Get, NOT nm.Get!
		if err != nil {
			return err
		}
		pop, err := doc.DataAt("pop")
		if err != nil {
			return err
		}
		return tx.Update(nm, []firestore.Update{{Path: "pop", Value: pop.(float64) + 0.2}})
	})
	if err != nil {
		// TODO: Handle error.
	}
}

func ExampleArrayUnion_create() {
	ctx := context.Background()
	client, err := firestore.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	defer client.Close()

	wr, err := client.Doc("States/Colorado").Create(ctx, map[string]interface{}{
		"cities": firestore.ArrayUnion("Denver", "Golden", "Boulder"),
		"pop":    5.5,
	})
	if err != nil {
		// TODO: Handle error.
	}
	fmt.Println(wr.UpdateTime)
}

func ExampleArrayUnion_update() {
	ctx := context.Background()
	client, err := firestore.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	defer client.Close()

	co := client.Doc("States/Colorado")
	wr, err := co.Update(ctx, []firestore.Update{
		{Path: "cities", Value: firestore.ArrayUnion("Broomfield")},
	})
	if err != nil {
		// TODO: Handle error.
	}
	fmt.Println(wr.UpdateTime)
}

func ExampleArrayRemove_update() {
	ctx := context.Background()
	client, err := firestore.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	defer client.Close()

	co := client.Doc("States/Colorado")
	wr, err := co.Update(ctx, []firestore.Update{
		{Path: "cities", Value: firestore.ArrayRemove("Denver")},
	})
	if err != nil {
		// TODO: Handle error.
	}
	fmt.Println(wr.UpdateTime)
}
