// Copyright 2012-2015 Oliver Eilhard. All rights reserved.
// Use of this source code is governed by a MIT-license.
// See http://olivere.mit-license.org/license.txt for details.

package elastic

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"time"
)

const (
	testIndexName  = "elastic-test"
	testIndexName2 = "elastic-test2"
	testMapping    = `
{
	"settings":{
		"number_of_shards":1,
		"number_of_replicas":0
	},
	"mappings":{
		"_default_": {
			"_timestamp": {
				"enabled": true
			},
			"_ttl": {
				"enabled": true
			}
		},
		"tweet":{
			"properties":{
				"tags":{
					"type":"string"
				},
				"location":{
					"type":"geo_point"
				},
				"suggest_field":{
					"type":"completion",
					"payloads":true
				}
			}
		},
		"comment":{
			"_parent": {
				"type":	"tweet"
			}
		},
		"order":{
			"properties":{
				"article":{
					"type":"string"
				},
				"manufacturer":{
					"type":"string",
					"index" : "not_analyzed"
				},
				"price":{
					"type":"float"
				},
				"time":{
					"type":"date",
					"format": "YYYY-MM-dd"
				}
			}
		}
	}
}
`
)

type tweet struct {
	User     string        `json:"user"`
	Message  string        `json:"message"`
	Retweets int           `json:"retweets"`
	Image    string        `json:"image,omitempty"`
	Created  time.Time     `json:"created,omitempty"`
	Tags     []string      `json:"tags,omitempty"`
	Location string        `json:"location,omitempty"`
	Suggest  *SuggestField `json:"suggest_field,omitempty"`
}

func (t tweet) String() string {
	return fmt.Sprintf("tweet{User:%q,Message:%q,Retweets:%d}", t.User, t.Message, t.Retweets)
}

type comment struct {
	User    string    `json:"user"`
	Comment string    `json:"comment"`
	Created time.Time `json:"created,omitempty"`
}

func (c comment) String() string {
	return fmt.Sprintf("comment{User:%q,Comment:%q}", c.User, c.Comment)
}

type order struct {
	Article      string  `json:"article"`
	Manufacturer string  `json:"manufacturer"`
	Price        float64 `json:"price"`
	Time         string  `json:"time,omitempty"`
}

func (o order) String() string {
	return fmt.Sprintf("order{Article:%q,Manufacturer:%q,Price:%v,Time:%v}", o.Article, o.Manufacturer, o.Price, o.Time)
}

func isTravis() bool {
	return os.Getenv("TRAVIS") != ""
}

func travisGoVersion() string {
	return os.Getenv("TRAVIS_GO_VERSION")
}

type logger interface {
	Error(args ...interface{})
	Errorf(format string, args ...interface{})
	Fatal(args ...interface{})
	Fatalf(format string, args ...interface{})
	Fail()
	FailNow()
	Log(args ...interface{})
	Logf(format string, args ...interface{})
}

func setupTestClient(t logger, options ...ClientOptionFunc) (client *Client) {
	var err error

	client, err = NewClient(options...)
	if err != nil {
		t.Fatal(err)
	}

	client.DeleteIndex(testIndexName).Do()
	client.DeleteIndex(testIndexName2).Do()

	return client
}

func setupTestClientAndCreateIndex(t logger, options ...ClientOptionFunc) *Client {
	client := setupTestClient(t, options...)

	// Create index
	createIndex, err := client.CreateIndex(testIndexName).Body(testMapping).Do()
	if err != nil {
		t.Fatal(err)
	}
	if createIndex == nil {
		t.Errorf("expected result to be != nil; got: %v", createIndex)
	}

	// Create second index
	createIndex2, err := client.CreateIndex(testIndexName2).Body(testMapping).Do()
	if err != nil {
		t.Fatal(err)
	}
	if createIndex2 == nil {
		t.Errorf("expected result to be != nil; got: %v", createIndex2)
	}

	return client
}

func setupTestClientAndCreateIndexAndLog(t logger, options ...ClientOptionFunc) *Client {
	return setupTestClientAndCreateIndex(t, SetTraceLog(log.New(os.Stdout, "", 0)))
}

func setupTestClientAndCreateIndexAndAddDocs(t logger, options ...ClientOptionFunc) *Client {
	client := setupTestClientAndCreateIndex(t, options...)

	// Add tweets
	tweet1 := tweet{User: "olivere", Message: "Welcome to Golang and Elasticsearch."}
	tweet2 := tweet{User: "olivere", Message: "Another unrelated topic."}
	tweet3 := tweet{User: "sandrae", Message: "Cycling is fun."}
	comment1 := comment{User: "nico", Comment: "You bet."}

	_, err := client.Index().Index(testIndexName).Type("tweet").Id("1").BodyJson(&tweet1).Do()
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Index().Index(testIndexName).Type("tweet").Id("2").BodyJson(&tweet2).Do()
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Index().Index(testIndexName).Type("tweet").Id("3").Routing("someroutingkey").BodyJson(&tweet3).Do()
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Index().Index(testIndexName).Type("comment").Id("1").Parent("3").BodyJson(&comment1).Do()
	if err != nil {
		t.Fatal(err)
	}

	// Add orders
	var orders []order
	orders = append(orders, order{Article: "Apple MacBook", Manufacturer: "Apple", Price: 1290, Time: "2015-01-18"})
	orders = append(orders, order{Article: "Paper", Manufacturer: "Canon", Price: 100, Time: "2015-03-01"})
	orders = append(orders, order{Article: "Apple iPad", Manufacturer: "Apple", Price: 499, Time: "2015-04-12"})
	orders = append(orders, order{Article: "Dell XPS 13", Manufacturer: "Dell", Price: 1600, Time: "2015-04-18"})
	orders = append(orders, order{Article: "Apple Watch", Manufacturer: "Apple", Price: 349, Time: "2015-04-29"})
	orders = append(orders, order{Article: "Samsung TV", Manufacturer: "Samsung", Price: 790, Time: "2015-05-03"})
	orders = append(orders, order{Article: "Hoodie", Manufacturer: "h&m", Price: 49, Time: "2015-06-03"})
	orders = append(orders, order{Article: "T-Shirt", Manufacturer: "h&m", Price: 19, Time: "2015-06-18"})
	for i, o := range orders {
		id := fmt.Sprintf("%d", i)
		_, err = client.Index().Index(testIndexName).Type("order").Id(id).BodyJson(&o).Do()
		if err != nil {
			t.Fatal(err)
		}
	}

	// Flush
	_, err = client.Flush().Index(testIndexName).Do()
	if err != nil {
		t.Fatal(err)
	}
	return client
}

var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func randomString(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}
