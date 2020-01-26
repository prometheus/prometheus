/*
Copyright 2017 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package spanner_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/api/iterator"
	sppb "google.golang.org/genproto/googleapis/spanner/v1"
)

func ExampleNewClient() {
	ctx := context.Background()
	const myDB = "projects/my-project/instances/my-instance/database/my-db"
	client, err := spanner.NewClient(ctx, myDB)
	if err != nil {
		// TODO: Handle error.
	}
	_ = client // TODO: Use client.
}

const myDB = "projects/my-project/instances/my-instance/database/my-db"

func ExampleNewClientWithConfig() {
	ctx := context.Background()
	const myDB = "projects/my-project/instances/my-instance/database/my-db"
	client, err := spanner.NewClientWithConfig(ctx, myDB, spanner.ClientConfig{
		NumChannels: 10,
	})
	if err != nil {
		// TODO: Handle error.
	}
	_ = client     // TODO: Use client.
	client.Close() // Close client when done.
}

func ExampleClient_Single() {
	ctx := context.Background()
	client, err := spanner.NewClient(ctx, myDB)
	if err != nil {
		// TODO: Handle error.
	}
	iter := client.Single().Query(ctx, spanner.NewStatement("SELECT FirstName FROM Singers"))
	_ = iter // TODO: iterate using Next or Do.
}

func ExampleClient_ReadOnlyTransaction() {
	ctx := context.Background()
	client, err := spanner.NewClient(ctx, myDB)
	if err != nil {
		// TODO: Handle error.
	}
	t := client.ReadOnlyTransaction()
	defer t.Close()
	// TODO: Read with t using Read, ReadRow, ReadUsingIndex, or Query.
}

func ExampleClient_ReadWriteTransaction() {
	ctx := context.Background()
	client, err := spanner.NewClient(ctx, myDB)
	if err != nil {
		// TODO: Handle error.
	}
	_, err = client.ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		var balance int64
		row, err := txn.ReadRow(ctx, "Accounts", spanner.Key{"alice"}, []string{"balance"})
		if err != nil {
			// This function will be called again if this is an
			// IsAborted error.
			return err
		}
		if err := row.Column(0, &balance); err != nil {
			return err
		}

		if balance <= 10 {
			return errors.New("insufficient funds in account")
		}
		balance -= 10
		m := spanner.Update("Accounts", []string{"user", "balance"}, []interface{}{"alice", balance})
		return txn.BufferWrite([]*spanner.Mutation{m})
		// The buffered mutation will be committed.  If the commit
		// fails with an IsAborted error, this function will be called
		// again.
	})
	if err != nil {
		// TODO: Handle error.
	}
}

func ExampleUpdate() {
	ctx := context.Background()
	client, err := spanner.NewClient(ctx, myDB)
	if err != nil {
		// TODO: Handle error.
	}
	_, err = client.ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		row, err := txn.ReadRow(ctx, "Accounts", spanner.Key{"alice"}, []string{"balance"})
		if err != nil {
			return err
		}
		var balance int64
		if err := row.Column(0, &balance); err != nil {
			return err
		}
		return txn.BufferWrite([]*spanner.Mutation{
			spanner.Update("Accounts", []string{"user", "balance"}, []interface{}{"alice", balance + 10}),
		})
	})
	if err != nil {
		// TODO: Handle error.
	}
}

// This example is the same as the one for Update, except for the use of UpdateMap.
func ExampleUpdateMap() {
	ctx := context.Background()
	client, err := spanner.NewClient(ctx, myDB)
	if err != nil {
		// TODO: Handle error.
	}
	_, err = client.ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		row, err := txn.ReadRow(ctx, "Accounts", spanner.Key{"alice"}, []string{"balance"})
		if err != nil {
			return err
		}
		var balance int64
		if err := row.Column(0, &balance); err != nil {
			return err
		}
		return txn.BufferWrite([]*spanner.Mutation{
			spanner.UpdateMap("Accounts", map[string]interface{}{
				"user":    "alice",
				"balance": balance + 10,
			}),
		})
	})
	if err != nil {
		// TODO: Handle error.
	}
}

// This example is the same as the one for Update, except for the use of UpdateStruct.
func ExampleUpdateStruct() {
	ctx := context.Background()
	client, err := spanner.NewClient(ctx, myDB)
	if err != nil {
		// TODO: Handle error.
	}
	type account struct {
		User    string `spanner:"user"`
		Balance int64  `spanner:"balance"`
	}
	_, err = client.ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		row, err := txn.ReadRow(ctx, "Accounts", spanner.Key{"alice"}, []string{"balance"})
		if err != nil {
			return err
		}
		var balance int64
		if err := row.Column(0, &balance); err != nil {
			return err
		}
		m, err := spanner.UpdateStruct("Accounts", account{
			User:    "alice",
			Balance: balance + 10,
		})
		if err != nil {
			return err
		}
		return txn.BufferWrite([]*spanner.Mutation{m})
	})
	if err != nil {
		// TODO: Handle error.
	}
}

func ExampleClient_Apply() {
	ctx := context.Background()
	client, err := spanner.NewClient(ctx, myDB)
	if err != nil {
		// TODO: Handle error.
	}
	m := spanner.Update("Users", []string{"name", "email"}, []interface{}{"alice", "a@example.com"})
	_, err = client.Apply(ctx, []*spanner.Mutation{m})
	if err != nil {
		// TODO: Handle error.
	}
}

func ExampleInsert() {
	m := spanner.Insert("Users", []string{"name", "email"}, []interface{}{"alice", "a@example.com"})
	_ = m // TODO: use with Client.Apply or in a ReadWriteTransaction.
}

func ExampleInsertMap() {
	m := spanner.InsertMap("Users", map[string]interface{}{
		"name":  "alice",
		"email": "a@example.com",
	})
	_ = m // TODO: use with Client.Apply or in a ReadWriteTransaction.
}

func ExampleInsertStruct() {
	type User struct {
		Name, Email string
	}
	u := User{Name: "alice", Email: "a@example.com"}
	m, err := spanner.InsertStruct("Users", u)
	if err != nil {
		// TODO: Handle error.
	}
	_ = m // TODO: use with Client.Apply or in a ReadWriteTransaction.
}

func ExampleDelete() {
	m := spanner.Delete("Users", spanner.Key{"alice"})
	_ = m // TODO: use with Client.Apply or in a ReadWriteTransaction.
}

func ExampleDelete_keyRange() {
	m := spanner.Delete("Users", spanner.KeyRange{
		Start: spanner.Key{"alice"},
		End:   spanner.Key{"bob"},
		Kind:  spanner.ClosedClosed,
	})
	_ = m // TODO: use with Client.Apply or in a ReadWriteTransaction.
}

func ExampleRowIterator_Next() {
	ctx := context.Background()
	client, err := spanner.NewClient(ctx, myDB)
	if err != nil {
		// TODO: Handle error.
	}
	iter := client.Single().Query(ctx, spanner.NewStatement("SELECT FirstName FROM Singers"))
	defer iter.Stop()
	for {
		row, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			// TODO: Handle error.
		}
		var firstName string
		if err := row.Column(0, &firstName); err != nil {
			// TODO: Handle error.
		}
		fmt.Println(firstName)
	}
}

func ExampleRowIterator_Do() {
	ctx := context.Background()
	client, err := spanner.NewClient(ctx, myDB)
	if err != nil {
		// TODO: Handle error.
	}
	iter := client.Single().Query(ctx, spanner.NewStatement("SELECT FirstName FROM Singers"))
	err = iter.Do(func(r *spanner.Row) error {
		var firstName string
		if err := r.Column(0, &firstName); err != nil {
			return err
		}
		fmt.Println(firstName)
		return nil
	})
	if err != nil {
		// TODO: Handle error.
	}
}

func ExampleRow_Size() {
	ctx := context.Background()
	client, err := spanner.NewClient(ctx, myDB)
	if err != nil {
		// TODO: Handle error.
	}
	row, err := client.Single().ReadRow(ctx, "Accounts", spanner.Key{"alice"}, []string{"name", "balance"})
	if err != nil {
		// TODO: Handle error.
	}
	fmt.Println(row.Size()) // size is 2
}

func ExampleRow_ColumnName() {
	ctx := context.Background()
	client, err := spanner.NewClient(ctx, myDB)
	if err != nil {
		// TODO: Handle error.
	}
	row, err := client.Single().ReadRow(ctx, "Accounts", spanner.Key{"alice"}, []string{"name", "balance"})
	if err != nil {
		// TODO: Handle error.
	}
	fmt.Println(row.ColumnName(1)) // prints "balance"
}

func ExampleRow_ColumnIndex() {
	ctx := context.Background()
	client, err := spanner.NewClient(ctx, myDB)
	if err != nil {
		// TODO: Handle error.
	}
	row, err := client.Single().ReadRow(ctx, "Accounts", spanner.Key{"alice"}, []string{"name", "balance"})
	if err != nil {
		// TODO: Handle error.
	}
	index, err := row.ColumnIndex("balance")
	if err != nil {
		// TODO: Handle error.
	}
	fmt.Println(index)
}

func ExampleRow_ColumnNames() {
	ctx := context.Background()
	client, err := spanner.NewClient(ctx, myDB)
	if err != nil {
		// TODO: Handle error.
	}
	row, err := client.Single().ReadRow(ctx, "Accounts", spanner.Key{"alice"}, []string{"name", "balance"})
	if err != nil {
		// TODO: Handle error.
	}
	fmt.Println(row.ColumnNames())
}

func ExampleRow_ColumnByName() {
	ctx := context.Background()
	client, err := spanner.NewClient(ctx, myDB)
	if err != nil {
		// TODO: Handle error.
	}
	row, err := client.Single().ReadRow(ctx, "Accounts", spanner.Key{"alice"}, []string{"name", "balance"})
	if err != nil {
		// TODO: Handle error.
	}
	var balance int64
	if err := row.ColumnByName("balance", &balance); err != nil {
		// TODO: Handle error.
	}
	fmt.Println(balance)
}

func ExampleRow_Columns() {
	ctx := context.Background()
	client, err := spanner.NewClient(ctx, myDB)
	if err != nil {
		// TODO: Handle error.
	}
	row, err := client.Single().ReadRow(ctx, "Accounts", spanner.Key{"alice"}, []string{"name", "balance"})
	if err != nil {
		// TODO: Handle error.
	}
	var name string
	var balance int64
	if err := row.Columns(&name, &balance); err != nil {
		// TODO: Handle error.
	}
	fmt.Println(name, balance)
}

func ExampleRow_ToStruct() {
	ctx := context.Background()
	client, err := spanner.NewClient(ctx, myDB)
	if err != nil {
		// TODO: Handle error.
	}
	row, err := client.Single().ReadRow(ctx, "Accounts", spanner.Key{"alice"}, []string{"name", "balance"})
	if err != nil {
		// TODO: Handle error.
	}

	type Account struct {
		Name    string
		Balance int64
	}

	var acct Account
	if err := row.ToStruct(&acct); err != nil {
		// TODO: Handle error.
	}
	fmt.Println(acct)
}

func ExampleReadOnlyTransaction_Read() {
	ctx := context.Background()
	client, err := spanner.NewClient(ctx, myDB)
	if err != nil {
		// TODO: Handle error.
	}
	iter := client.Single().Read(ctx, "Users",
		spanner.KeySets(spanner.Key{"alice"}, spanner.Key{"bob"}),
		[]string{"name", "email"})
	_ = iter // TODO: iterate using Next or Do.
}

func ExampleReadOnlyTransaction_ReadUsingIndex() {
	ctx := context.Background()
	client, err := spanner.NewClient(ctx, myDB)
	if err != nil {
		// TODO: Handle error.
	}
	iter := client.Single().ReadUsingIndex(ctx, "Users",
		"UsersByEmail",
		spanner.KeySets(spanner.Key{"a@example.com"}, spanner.Key{"b@example.com"}),
		[]string{"name", "email"})
	_ = iter // TODO: iterate using Next or Do.
}

func ExampleReadOnlyTransaction_ReadWithOptions() {
	ctx := context.Background()
	client, err := spanner.NewClient(ctx, myDB)
	if err != nil {
		// TODO: Handle error.
	}
	// Use an index, and limit to 100 rows at most.
	iter := client.Single().ReadWithOptions(ctx, "Users",
		spanner.KeySets(spanner.Key{"a@example.com"}, spanner.Key{"b@example.com"}),
		[]string{"name", "email"}, &spanner.ReadOptions{
			Index: "UsersByEmail",
			Limit: 100,
		})
	_ = iter // TODO: iterate using Next or Do.
}

func ExampleReadOnlyTransaction_ReadRow() {
	ctx := context.Background()
	client, err := spanner.NewClient(ctx, myDB)
	if err != nil {
		// TODO: Handle error.
	}
	row, err := client.Single().ReadRow(ctx, "Users", spanner.Key{"alice"},
		[]string{"name", "email"})
	if err != nil {
		// TODO: Handle error.
	}
	_ = row // TODO: use row
}

func ExampleReadOnlyTransaction_Query() {
	ctx := context.Background()
	client, err := spanner.NewClient(ctx, myDB)
	if err != nil {
		// TODO: Handle error.
	}
	iter := client.Single().Query(ctx, spanner.NewStatement("SELECT FirstName FROM Singers"))
	_ = iter // TODO: iterate using Next or Do.
}

func ExampleNewStatement() {
	stmt := spanner.NewStatement("SELECT FirstName, LastName FROM SINGERS WHERE LastName >= @start")
	stmt.Params["start"] = "Dylan"
	// TODO: Use stmt in Query.
}

func ExampleNewStatement_structLiteral() {
	stmt := spanner.Statement{
		SQL: `SELECT FirstName, LastName FROM SINGERS WHERE LastName = ("Lea", "Martin")`,
	}
	_ = stmt // TODO: Use stmt in Query.
}

func ExampleStructParam() {
	stmt := spanner.Statement{
		SQL: "SELECT * FROM SINGERS WHERE (FirstName, LastName) = @singerinfo",
		Params: map[string]interface{}{
			"singerinfo": struct {
				FirstName string
				LastName  string
			}{"Bob", "Dylan"},
		},
	}
	_ = stmt // TODO: Use stmt in Query.
}

func ExampleArrayOfStructParam() {
	stmt := spanner.Statement{
		SQL: "SELECT * FROM SINGERS WHERE (FirstName, LastName) IN UNNEST(@singerinfo)",
		Params: map[string]interface{}{
			"singerinfo": []struct {
				FirstName string
				LastName  string
			}{
				{"Ringo", "Starr"},
				{"John", "Lennon"},
			},
		},
	}
	_ = stmt // TODO: Use stmt in Query.
}

func ExampleReadOnlyTransaction_Timestamp() {
	ctx := context.Background()
	client, err := spanner.NewClient(ctx, myDB)
	if err != nil {
		// TODO: Handle error.
	}
	txn := client.Single()
	row, err := txn.ReadRow(ctx, "Users", spanner.Key{"alice"},
		[]string{"name", "email"})
	if err != nil {
		// TODO: Handle error.
	}
	readTimestamp, err := txn.Timestamp()
	if err != nil {
		// TODO: Handle error.
	}
	fmt.Println("read happened at", readTimestamp)
	_ = row // TODO: use row
}

func ExampleReadOnlyTransaction_WithTimestampBound() {
	ctx := context.Background()
	client, err := spanner.NewClient(ctx, myDB)
	if err != nil {
		// TODO: Handle error.
	}
	txn := client.Single().WithTimestampBound(spanner.MaxStaleness(30 * time.Second))
	row, err := txn.ReadRow(ctx, "Users", spanner.Key{"alice"}, []string{"name", "email"})
	if err != nil {
		// TODO: Handle error.
	}
	_ = row // TODO: use row
	readTimestamp, err := txn.Timestamp()
	if err != nil {
		// TODO: Handle error.
	}
	fmt.Println("read happened at", readTimestamp)
}

func ExampleGenericColumnValue_Decode() {
	// In real applications, rows can be retrieved by methods like client.Single().ReadRow().
	row, err := spanner.NewRow([]string{"intCol", "strCol"}, []interface{}{42, "my-text"})
	if err != nil {
		// TODO: Handle error.
	}
	for i := 0; i < row.Size(); i++ {
		var col spanner.GenericColumnValue
		if err := row.Column(i, &col); err != nil {
			// TODO: Handle error.
		}
		switch col.Type.Code {
		case sppb.TypeCode_INT64:
			var v int64
			if err := col.Decode(&v); err != nil {
				// TODO: Handle error.
			}
			fmt.Println("int", v)
		case sppb.TypeCode_STRING:
			var v string
			if err := col.Decode(&v); err != nil {
				// TODO: Handle error.
			}
			fmt.Println("string", v)
		}
	}
	// Output:
	// int 42
	// string my-text
}

func ExampleClient_BatchReadOnlyTransaction() {
	ctx := context.Background()
	var (
		client *spanner.Client
		txn    *spanner.BatchReadOnlyTransaction
		err    error
	)
	if client, err = spanner.NewClient(ctx, myDB); err != nil {
		// TODO: Handle error.
	}
	defer client.Close()
	if txn, err = client.BatchReadOnlyTransaction(ctx, spanner.StrongRead()); err != nil {
		// TODO: Handle error.
	}
	defer txn.Close()

	// Singer represents the elements in a row from the Singers table.
	type Singer struct {
		SingerID   int64
		FirstName  string
		LastName   string
		SingerInfo []byte
	}
	stmt := spanner.Statement{SQL: "SELECT * FROM Singers;"}
	partitions, err := txn.PartitionQuery(ctx, stmt, spanner.PartitionOptions{})
	if err != nil {
		// TODO: Handle error.
	}
	// Note: here we use multiple goroutines, but you should use separate processes/machines.
	wg := sync.WaitGroup{}
	for i, p := range partitions {
		wg.Add(1)
		go func(i int, p *spanner.Partition) {
			defer wg.Done()
			iter := txn.Execute(ctx, p)
			defer iter.Stop()
			for {
				row, err := iter.Next()
				if err == iterator.Done {
					break
				} else if err != nil {
					// TODO: Handle error.
				}
				var s Singer
				if err := row.ToStruct(&s); err != nil {
					// TODO: Handle error.
				}
				_ = s // TODO: Process the row.
			}
		}(i, p)
	}
	wg.Wait()
}

func ExampleCommitTimestamp() {
	ctx := context.Background()
	client, err := spanner.NewClient(ctx, myDB)
	if err != nil {
		// TODO: Handle error.
	}

	type account struct {
		User     string
		Creation spanner.NullTime // time.Time can also be used if column is NOT NULL
	}

	a := account{User: "Joe", Creation: spanner.NullTime{spanner.CommitTimestamp, true}}
	m, err := spanner.InsertStruct("Accounts", a)
	if err != nil {
		// TODO: Handle error.
	}
	_, err = client.Apply(ctx, []*spanner.Mutation{m}, spanner.ApplyAtLeastOnce())
	if err != nil {
		// TODO: Handle error.
	}

	if r, e := client.Single().ReadRow(ctx, "Accounts", spanner.Key{"Joe"}, []string{"User", "Creation"}); e != nil {
		// TODO: Handle error.
	} else {
		var got account
		if err := r.ToStruct(&got); err != nil {
			// TODO: Handle error.
		}
		_ = got // TODO: Process row.
	}
}

func ExampleStatement_regexpContains() {
	// Search for accounts with valid emails using regexp as per:
	//   https://cloud.google.com/spanner/docs/functions-and-operators#regexp_contains
	stmt := spanner.Statement{
		SQL: `SELECT * FROM users WHERE REGEXP_CONTAINS(email, @valid_email)`,
		Params: map[string]interface{}{
			"valid_email": `\Q@\E`,
		},
	}
	_ = stmt // TODO: Use stmt in a query.
}
