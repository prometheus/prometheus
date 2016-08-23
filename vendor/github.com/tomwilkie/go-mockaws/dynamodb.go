package mockaws

import (
	"bytes"
	"fmt"
	"log"
	"sort"
	"sync"

	"github.com/aws/aws-sdk-go/service/dynamodb"
)

type MockDynamoDB struct {
	mtx    sync.RWMutex
	tables map[string]mockDynamoDBTable
}

type mockDynamoDBTable struct {
	hashKey  string
	rangeKey string
	items    map[string][]mockDynamoDBItem
}

type mockDynamoDBItem map[string]*dynamodb.AttributeValue

func NewMockDynamoDB() *MockDynamoDB {
	return &MockDynamoDB{
		tables: map[string]mockDynamoDBTable{},
	}
}

func (m *MockDynamoDB) CreateTable(input *dynamodb.CreateTableInput) (*dynamodb.CreateTableOutput, error) {
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

func (m *MockDynamoDB) ListTables(*dynamodb.ListTablesInput) (*dynamodb.ListTablesOutput, error) {
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

func (m *MockDynamoDB) BatchWriteItem(input *dynamodb.BatchWriteItemInput) (*dynamodb.BatchWriteItemOutput, error) {
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
			log.Printf("Write %s/%x", hashValue, rangeValue)

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

func (m *MockDynamoDB) Query(input *dynamodb.QueryInput) (*dynamodb.QueryOutput, error) {
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
		log.Printf("Lookup %s/* -> *", hashValue)
		found = items
	} else {
		rangeValueStart := rangeKeyCondition.AttributeValueList[0].B
		rangeValueEnd := rangeKeyCondition.AttributeValueList[1].B

		log.Printf("Lookup %s/%x -> %x (%d)", hashValue, rangeValueStart, rangeValueEnd, len(items))

		i := sort.Search(len(items), func(i int) bool {
			return bytes.Compare(items[i][table.rangeKey].B, rangeValueStart) >= 0
		})

		j := sort.Search(len(items), func(i int) bool {
			return bytes.Compare(items[i][table.rangeKey].B, rangeValueEnd) > 0
		})

		log.Printf("  found range [%d:%d]", i, j)
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
