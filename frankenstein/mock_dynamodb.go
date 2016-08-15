// Copyright 2016 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package frankenstein

import (
	"bytes"
	"fmt"
	"sort"
	"sync"

	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/prometheus/common/log"
)

type mockDynamoDB struct {
	mtx    sync.RWMutex
	tables map[string]mockDynamoDBTable
}

type mockDynamoDBTable struct {
	hashKey  string
	rangeKey string
	items    map[string][]mockDynamoDBItem
}

type mockDynamoDBItem map[string]*dynamodb.AttributeValue

func newMockDynamoDB() *mockDynamoDB {
	return &mockDynamoDB{
		tables: map[string]mockDynamoDBTable{},
	}
}

func (m *mockDynamoDB) CreateTable(input *dynamodb.CreateTableInput) (*dynamodb.CreateTableOutput, error) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	var hashKey, rangeKey string
	for _, schemaElement := range input.KeySchema {
		if *schemaElement.KeyType == "HASH" {
			hashKey = *schemaElement.AttributeName
		} else if *schemaElement.KeyType == "RANGE" {
			rangeKey = *schemaElement.AttributeName
		}
	}

	m.tables[*input.TableName] = mockDynamoDBTable{
		hashKey:  hashKey,
		rangeKey: rangeKey,
		items:    map[string][]mockDynamoDBItem{},
	}

	return &dynamodb.CreateTableOutput{}, nil
}

func (m *mockDynamoDB) ListTables(*dynamodb.ListTablesInput) (*dynamodb.ListTablesOutput, error) {
	m.mtx.RLock()
	defer m.mtx.RUnlock()

	var tableNames []*string
	for tableName, _ := range m.tables {
		func(tableName string) {
			tableNames = append(tableNames, &tableName)
		}(tableName)
	}
	return &dynamodb.ListTablesOutput{
		TableNames: tableNames,
	}, nil
}

func (m *mockDynamoDB) BatchWriteItem(input *dynamodb.BatchWriteItemInput) (*dynamodb.BatchWriteItemOutput, error) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	for tableName, writeRequests := range input.RequestItems {
		table, ok := m.tables[tableName]
		if !ok {
			return &dynamodb.BatchWriteItemOutput{}, fmt.Errorf("table not found")
		}

		for _, writeRequest := range writeRequests {
			hashValue := *writeRequest.PutRequest.Item[table.hashKey].S
			rangeValue := writeRequest.PutRequest.Item[table.rangeKey].B
			log.Infof("Write %s/%x", hashValue, rangeValue)

			items := table.items[hashValue]

			// insert in order
			i := sort.Search(len(items), func(i int) bool {
				return bytes.Compare(items[i][table.rangeKey].B, rangeValue) >= 0
			})
			if i >= len(items) || !bytes.Equal(items[i][table.rangeKey].B, rangeValue) {
				items = append(items, nil)
				copy(items[i+1:], items[i:])
			}
			items[i] = writeRequest.PutRequest.Item

			table.items[hashValue] = items
		}
	}
	return &dynamodb.BatchWriteItemOutput{}, nil
}

func (m *mockDynamoDB) Query(input *dynamodb.QueryInput) (*dynamodb.QueryOutput, error) {
	m.mtx.RLock()
	defer m.mtx.RUnlock()

	table, ok := m.tables[*input.TableName]
	if !ok {
		return nil, fmt.Errorf("table not found")
	}

	hashValueCondition, ok := input.KeyConditions[table.hashKey]
	if !ok {
		return &dynamodb.QueryOutput{}, fmt.Errorf("must specify hash value condition")
	}

	hashValue := *hashValueCondition.AttributeValueList[0].S
	items, ok := table.items[hashValue]
	if !ok {
		return &dynamodb.QueryOutput{}, nil
	}

	var found []mockDynamoDBItem
	rangeKeyCondition, ok := input.KeyConditions[table.rangeKey]
	if !ok {
		log.Infof("Lookup %s/* -> *", hashValue)
		found = items
	} else {
		rangeValueStart := rangeKeyCondition.AttributeValueList[0].B
		rangeValueEnd := rangeKeyCondition.AttributeValueList[1].B

		log.Infof("Lookup %s/%x -> %x (%d)", hashValue, rangeValueStart, rangeValueEnd, len(items))

		i := sort.Search(len(items), func(i int) bool {
			return bytes.Compare(items[i][table.rangeKey].B, rangeValueStart) >= 0
		})

		j := sort.Search(len(items), func(i int) bool {
			return bytes.Compare(items[i][table.rangeKey].B, rangeValueEnd) > 0
		})

		log.Infof("  found range [%d:%d]", i, j)
		if i > len(items) || i == j {
			return &dynamodb.QueryOutput{}, nil
		}
		found = items[i:j]
	}

	result := make([]map[string]*dynamodb.AttributeValue, 0, len(found))
	for _, item := range found {
		result = append(result, item)
	}

	return &dynamodb.QueryOutput{
		Items: result,
	}, nil
}
