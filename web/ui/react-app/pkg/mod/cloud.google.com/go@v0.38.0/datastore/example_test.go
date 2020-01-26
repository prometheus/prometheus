// Copyright 2014 Google LLC
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

package datastore_test

import (
	"context"
	"fmt"
	"log"
	"time"

	"cloud.google.com/go/datastore"
	"google.golang.org/api/iterator"
)

func ExampleNewClient() {
	ctx := context.Background()
	client, err := datastore.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	_ = client // TODO: Use client.
}

func ExampleClient_Get() {
	ctx := context.Background()
	client, err := datastore.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}

	type Article struct {
		Title       string
		Description string
		Body        string `datastore:",noindex"`
		Author      *datastore.Key
		PublishedAt time.Time
	}
	key := datastore.NameKey("Article", "articled1", nil)
	article := &Article{}
	if err := client.Get(ctx, key, article); err != nil {
		// TODO: Handle error.
	}
}

func ExampleClient_Put() {
	ctx := context.Background()
	client, err := datastore.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}

	type Article struct {
		Title       string
		Description string
		Body        string `datastore:",noindex"`
		Author      *datastore.Key
		PublishedAt time.Time
	}
	newKey := datastore.IncompleteKey("Article", nil)
	_, err = client.Put(ctx, newKey, &Article{
		Title:       "The title of the article",
		Description: "The description of the article...",
		Body:        "...",
		Author:      datastore.NameKey("Author", "jbd", nil),
		PublishedAt: time.Now(),
	})
	if err != nil {
		// TODO: Handle error.
	}
}

func ExampleClient_Put_flatten() {
	ctx := context.Background()
	client, err := datastore.NewClient(ctx, "project-id")
	if err != nil {
		log.Fatal(err)
	}

	type Animal struct {
		Name  string
		Type  string
		Breed string
	}

	type Human struct {
		Name   string
		Height int
		Pet    Animal `datastore:",flatten"`
	}

	newKey := datastore.IncompleteKey("Human", nil)
	_, err = client.Put(ctx, newKey, &Human{
		Name:   "Susan",
		Height: 67,
		Pet: Animal{
			Name:  "Fluffy",
			Type:  "Cat",
			Breed: "Sphynx",
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}

func ExampleClient_Delete() {
	ctx := context.Background()
	client, err := datastore.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}

	key := datastore.NameKey("Article", "articled1", nil)
	if err := client.Delete(ctx, key); err != nil {
		// TODO: Handle error.
	}
}

func ExampleClient_DeleteMulti() {
	ctx := context.Background()
	client, err := datastore.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	var keys []*datastore.Key
	for i := 1; i <= 10; i++ {
		keys = append(keys, datastore.IDKey("Article", int64(i), nil))
	}
	if err := client.DeleteMulti(ctx, keys); err != nil {
		// TODO: Handle error.
	}
}

type Post struct {
	Title       string
	PublishedAt time.Time
	Comments    int
}

func ExampleClient_GetMulti() {
	ctx := context.Background()
	client, err := datastore.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}

	keys := []*datastore.Key{
		datastore.NameKey("Post", "post1", nil),
		datastore.NameKey("Post", "post2", nil),
		datastore.NameKey("Post", "post3", nil),
	}
	posts := make([]Post, 3)
	if err := client.GetMulti(ctx, keys, posts); err != nil {
		// TODO: Handle error.
	}
}

func ExampleMultiError() {
	ctx := context.Background()
	client, err := datastore.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}

	keys := []*datastore.Key{
		datastore.NameKey("bad-key", "bad-key", nil),
	}
	posts := make([]Post, 1)
	if err := client.GetMulti(ctx, keys, posts); err != nil {
		if merr, ok := err.(datastore.MultiError); ok {
			for _, err := range merr {
				// TODO: Handle error.
				_ = err
			}
		} else {
			// TODO: Handle error.
		}
	}
}

func ExampleClient_PutMulti_slice() {
	ctx := context.Background()
	client, err := datastore.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}

	keys := []*datastore.Key{
		datastore.NameKey("Post", "post1", nil),
		datastore.NameKey("Post", "post2", nil),
	}

	// PutMulti with a Post slice.
	posts := []*Post{
		{Title: "Post 1", PublishedAt: time.Now()},
		{Title: "Post 2", PublishedAt: time.Now()},
	}
	if _, err := client.PutMulti(ctx, keys, posts); err != nil {
		// TODO: Handle error.
	}
}

func ExampleClient_PutMulti_interfaceSlice() {
	ctx := context.Background()
	client, err := datastore.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}

	keys := []*datastore.Key{
		datastore.NameKey("Post", "post1", nil),
		datastore.NameKey("Post", "post2", nil),
	}

	// PutMulti with an empty interface slice.
	posts := []interface{}{
		&Post{Title: "Post 1", PublishedAt: time.Now()},
		&Post{Title: "Post 2", PublishedAt: time.Now()},
	}
	if _, err := client.PutMulti(ctx, keys, posts); err != nil {
		// TODO: Handle error.
	}
}

func ExampleNewQuery() {
	// Query for Post entities.
	q := datastore.NewQuery("Post")
	_ = q // TODO: Use the query with Client.Run.
}

func ExampleNewQuery_options() {
	// Query to order the posts by the number of comments they have received.
	q := datastore.NewQuery("Post").Order("-Comments")
	// Start listing from an offset and limit the results.
	q = q.Offset(20).Limit(10)
	_ = q // TODO: Use the query.
}

func ExampleClient_Count() {
	ctx := context.Background()
	client, err := datastore.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	// Count the number of the post entities.
	q := datastore.NewQuery("Post")
	n, err := client.Count(ctx, q)
	if err != nil {
		// TODO: Handle error.
	}
	fmt.Printf("There are %d posts.", n)
}

func ExampleClient_Run() {
	ctx := context.Background()
	client, err := datastore.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	// List the posts published since yesterday.
	yesterday := time.Now().Add(-24 * time.Hour)
	q := datastore.NewQuery("Post").Filter("PublishedAt >", yesterday)
	it := client.Run(ctx, q)
	_ = it // TODO: iterate using Next.
}

func ExampleClient_NewTransaction() {
	ctx := context.Background()
	client, err := datastore.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	const retries = 3

	// Increment a counter.
	// See https://cloud.google.com/appengine/articles/sharding_counters for
	// a more scalable solution.
	type Counter struct {
		Count int
	}

	key := datastore.NameKey("counter", "CounterA", nil)
	var tx *datastore.Transaction
	for i := 0; i < retries; i++ {
		tx, err = client.NewTransaction(ctx)
		if err != nil {
			break
		}

		var c Counter
		if err = tx.Get(key, &c); err != nil && err != datastore.ErrNoSuchEntity {
			break
		}
		c.Count++
		if _, err = tx.Put(key, &c); err != nil {
			break
		}

		// Attempt to commit the transaction. If there's a conflict, try again.
		if _, err = tx.Commit(); err != datastore.ErrConcurrentTransaction {
			break
		}
	}
	if err != nil {
		// TODO: Handle error.
	}
}

func ExampleClient_RunInTransaction() {
	ctx := context.Background()
	client, err := datastore.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}

	// Increment a counter.
	// See https://cloud.google.com/appengine/articles/sharding_counters for
	// a more scalable solution.
	type Counter struct {
		Count int
	}

	var count int
	key := datastore.NameKey("Counter", "singleton", nil)
	_, err = client.RunInTransaction(ctx, func(tx *datastore.Transaction) error {
		var x Counter
		if err := tx.Get(key, &x); err != nil && err != datastore.ErrNoSuchEntity {
			return err
		}
		x.Count++
		if _, err := tx.Put(key, &x); err != nil {
			return err
		}
		count = x.Count
		return nil
	})
	if err != nil {
		// TODO: Handle error.
	}
	// The value of count is only valid once the transaction is successful
	// (RunInTransaction has returned nil).
	fmt.Printf("Count=%d\n", count)
}

func ExampleClient_AllocateIDs() {
	ctx := context.Background()
	client, err := datastore.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	var keys []*datastore.Key
	for i := 0; i < 10; i++ {
		keys = append(keys, datastore.IncompleteKey("Article", nil))
	}
	keys, err = client.AllocateIDs(ctx, keys)
	if err != nil {
		// TODO: Handle error.
	}
	_ = keys // TODO: Use keys.
}

func ExampleKey_Encode() {
	key := datastore.IDKey("Article", 1, nil)
	encoded := key.Encode()
	fmt.Println(encoded)
	// Output: EgsKB0FydGljbGUQAQ
}

func ExampleDecodeKey() {
	const encoded = "EgsKB0FydGljbGUQAQ"
	key, err := datastore.DecodeKey(encoded)
	if err != nil {
		// TODO: Handle error.
	}
	fmt.Println(key)
	// Output: /Article,1
}

func ExampleIDKey() {
	// Key with numeric ID.
	k := datastore.IDKey("Article", 1, nil)
	_ = k // TODO: Use key.
}

func ExampleNameKey() {
	// Key with string ID.
	k := datastore.NameKey("Article", "article8", nil)
	_ = k // TODO: Use key.
}

func ExampleIncompleteKey() {
	k := datastore.IncompleteKey("Article", nil)
	_ = k // TODO: Use incomplete key.
}

func ExampleClient_GetAll() {
	ctx := context.Background()
	client, err := datastore.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	var posts []*Post
	keys, err := client.GetAll(ctx, datastore.NewQuery("Post"), &posts)
	if err != nil {
		// TODO: Handle error.
	}
	for i, key := range keys {
		fmt.Println(key)
		fmt.Println(posts[i])
	}
}

func ExampleClient_Mutate() {
	ctx := context.Background()
	client, err := datastore.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}

	key1 := datastore.NameKey("Post", "post1", nil)
	key2 := datastore.NameKey("Post", "post2", nil)
	key3 := datastore.NameKey("Post", "post3", nil)
	key4 := datastore.NameKey("Post", "post4", nil)

	_, err = client.Mutate(ctx,
		datastore.NewInsert(key1, Post{Title: "Post 1"}),
		datastore.NewUpsert(key2, Post{Title: "Post 2"}),
		datastore.NewUpdate(key3, Post{Title: "Post 3"}),
		datastore.NewDelete(key4))
	if err != nil {
		// TODO: Handle error.
	}
}

func ExampleCommit_Key() {
	ctx := context.Background()
	client, err := datastore.NewClient(ctx, "")
	if err != nil {
		// TODO: Handle error.
	}
	var pk1, pk2 *datastore.PendingKey
	// Create two posts in a single transaction.
	commit, err := client.RunInTransaction(ctx, func(tx *datastore.Transaction) error {
		var err error
		pk1, err = tx.Put(datastore.IncompleteKey("Post", nil), &Post{Title: "Post 1", PublishedAt: time.Now()})
		if err != nil {
			return err
		}
		pk2, err = tx.Put(datastore.IncompleteKey("Post", nil), &Post{Title: "Post 2", PublishedAt: time.Now()})
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		// TODO: Handle error.
	}
	// Now pk1, pk2 are valid PendingKeys. Let's convert them into real keys
	// using the Commit object.
	k1 := commit.Key(pk1)
	k2 := commit.Key(pk2)
	fmt.Println(k1, k2)
}

func ExampleIterator_Next() {
	ctx := context.Background()
	client, err := datastore.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	it := client.Run(ctx, datastore.NewQuery("Post"))
	for {
		var p Post
		key, err := it.Next(&p)
		if err == iterator.Done {
			break
		}
		if err != nil {
			// TODO: Handle error.
		}
		fmt.Println(key, p)
	}
}

func ExampleIterator_Cursor() {
	ctx := context.Background()
	client, err := datastore.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	it := client.Run(ctx, datastore.NewQuery("Post"))
	for {
		var p Post
		_, err := it.Next(&p)
		if err == iterator.Done {
			break
		}
		if err != nil {
			// TODO: Handle error.
		}
		fmt.Println(p)
		cursor, err := it.Cursor()
		if err != nil {
			// TODO: Handle error.
		}
		// When printed, a cursor will display as a string that can be passed
		// to datastore.NewCursor.
		fmt.Printf("to resume with this post, use cursor %s\n", cursor)
	}
}

func ExampleDecodeCursor() {
	// See Query.Start for a fuller example of DecodeCursor.
	// getCursor represents a function that returns a cursor from a previous
	// iteration in string form.
	cursorString := getCursor()
	cursor, err := datastore.DecodeCursor(cursorString)
	if err != nil {
		// TODO: Handle error.
	}
	_ = cursor // TODO: Use the cursor with Query.Start or Query.End.
}

func getCursor() string { return "" }

func ExampleQuery_Start() {
	// This example demonstrates how to use cursors and Query.Start
	// to resume an iteration.
	ctx := context.Background()
	client, err := datastore.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	// getCursor represents a function that returns a cursor from a previous
	// iteration in string form.
	cursorString := getCursor()
	cursor, err := datastore.DecodeCursor(cursorString)
	if err != nil {
		// TODO: Handle error.
	}
	it := client.Run(ctx, datastore.NewQuery("Post").Start(cursor))
	_ = it // TODO: Use iterator.
}

func ExampleLoadStruct() {
	type Player struct {
		User  string
		Score int
	}
	// Normally LoadStruct would only be used inside a custom implementation of
	// PropertyLoadSaver; this is for illustrative purposes only.
	props := []datastore.Property{
		{Name: "User", Value: "Alice"},
		{Name: "Score", Value: int64(97)},
	}

	var p Player
	if err := datastore.LoadStruct(&p, props); err != nil {
		// TODO: Handle error.
	}
	fmt.Println(p)
	// Output: {Alice 97}
}

func ExampleSaveStruct() {
	type Player struct {
		User  string
		Score int
	}

	p := &Player{
		User:  "Alice",
		Score: 97,
	}
	props, err := datastore.SaveStruct(p)
	if err != nil {
		// TODO: Handle error.
	}
	fmt.Println(props)
	// TODO(jba): make this output stable: Output: [{User Alice false} {Score 97 false}]
}
