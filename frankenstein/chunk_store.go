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
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/storage/metric"
)

const (
	hashKey    = "h"
	rangeKey   = "r"
	chunkKey   = "c"
	minChunkID = "0000000000000000"
	maxChunkID = "FFFFFFFFFFFFFFFF"
)

var (
	dynamoRequestDuration = prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Namespace: "prometheus",
		Name:      "dynamo_request_duration_seconds",
		Help:      "Time spent doing DynamoDB requests.",
	}, []string{"operation", "status_code"})
	dynamoConsumedCapacity = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "prometheus",
		Name:      "dynamo_consumed_capacity_total",
		Help:      "The capacity units consumed by operation.",
	}, []string{"operation"})
	s3RequestDuration = prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Namespace: "prometheus",
		Name:      "s3_request_duration_seconds",
		Help:      "Time spent doing S3 requests.",
	}, []string{"operation", "status_code"})
)

func init() {
	prometheus.MustRegister(dynamoRequestDuration)
	prometheus.MustRegister(dynamoConsumedCapacity)
	prometheus.MustRegister(s3RequestDuration)
}

// Chunk contains encoded timeseries data
type Chunk struct {
	ID      string       `json:"-"`
	From    model.Time   `json:"from"`
	Through model.Time   `json:"through"`
	Metric  model.Metric `json:"metric"`
	Data    []byte       `json:"-"`
}

// ChunkStore stores and indexes chunks
type ChunkStore interface {
	Put([]Chunk) error
	Get(from, through model.Time, matchers ...*metric.LabelMatcher) ([]Chunk, error)
}

// ChunkStoreConfig specifies config for a ChunkStore
type ChunkStoreConfig struct {
	S3URL       string
	DynamoDBURL string
}

// NewAWSChunkStore makes a new ChunkStore
func NewAWSChunkStore(cfg ChunkStoreConfig) (*AWSChunkStore, error) {
	s3URL, err := url.Parse(cfg.S3URL)
	if err != nil {
		return nil, err
	}

	s3Config, err := awsConfigFromURL(s3URL)
	if err != nil {
		return nil, err
	}

	dynamodbURL, err := url.Parse(cfg.DynamoDBURL)
	if err != nil {
		return nil, err
	}

	dynamoDBConfig, err := awsConfigFromURL(dynamodbURL)
	if err != nil {
		return nil, err
	}

	tableName := strings.TrimPrefix(dynamodbURL.Path, "/")
	bucketName := strings.TrimPrefix(s3URL.Path, "/")

	return &AWSChunkStore{
		dynamodb:   dynamodb.New(session.New(dynamoDBConfig)),
		s3:         s3.New(session.New(s3Config)),
		tableName:  tableName,
		bucketName: bucketName,
	}, nil
}

func awsConfigFromURL(url *url.URL) (*aws.Config, error) {
	if url.User == nil {
		return nil, fmt.Errorf("must specify username & password in URL")
	}
	password, _ := url.User.Password()
	creds := credentials.NewStaticCredentials(url.User.Username(), password, "")
	config := aws.NewConfig().WithCredentials(creds)
	if strings.Contains(url.Host, ".") {
		config = config.WithEndpoint(fmt.Sprintf("http://%s", url.Host)).WithRegion("dummy")
	} else {
		config = config.WithRegion(url.Host)
	}
	return config, nil
}

// AWSChunkStore implements ChunkStore for AWS
type AWSChunkStore struct {
	dynamodb   *dynamodb.DynamoDB
	s3         *s3.S3
	tableName  string
	bucketName string
	cfg        ChunkStoreConfig
}

// CreateTables creates the required tables in DynamoDB.
func (c *AWSChunkStore) CreateTables() error {
	// See if tableName exists.
	resp, err := c.dynamodb.ListTables(&dynamodb.ListTablesInput{
		Limit: aws.Int64(10),
	})
	if err != nil {
		return err
	}
	for _, s := range resp.TableNames {
		if *s == c.tableName {
			return nil
		}
	}

	params := &dynamodb.CreateTableInput{
		TableName: aws.String(c.tableName),
		AttributeDefinitions: []*dynamodb.AttributeDefinition{
			{
				AttributeName: aws.String(hashKey),
				AttributeType: aws.String("S"),
			},
			{
				AttributeName: aws.String(rangeKey),
				AttributeType: aws.String("S"),
			},
		},
		KeySchema: []*dynamodb.KeySchemaElement{
			{
				AttributeName: aws.String(hashKey),
				KeyType:       aws.String("HASH"),
			},
			{
				AttributeName: aws.String(rangeKey),
				KeyType:       aws.String("RANGE"),
			},
		},
		ProvisionedThroughput: &dynamodb.ProvisionedThroughput{
			ReadCapacityUnits:  aws.Int64(10),
			WriteCapacityUnits: aws.Int64(5),
		},
	}
	log.Infof("Creating table %s", c.tableName)
	_, err = c.dynamodb.CreateTable(params)
	return err
}

func bigBuckets(from, through model.Time) []int64 {
	var (
		secondsInHour = int64(time.Hour / time.Second)
		fromHour      = from.Unix() / secondsInHour
		throughHour   = through.Unix() / secondsInHour
		result        []int64
	)
	for i := fromHour; i <= throughHour; i++ {
		result = append(result, i)
	}
	return result
}

// Put implements ChunkStore
func (c *AWSChunkStore) Put(chunks []Chunk) error {
	// TODO: parallelise
	for _, chunk := range chunks {
		err := timeRequest("Put", s3RequestDuration, func() error {
			var err error
			_, err = c.s3.PutObject(&s3.PutObjectInput{
				Body:   bytes.NewReader(chunk.Data),
				Bucket: aws.String(c.bucketName),
				Key:    aws.String(chunk.ID),
			})
			return err
		})
		if err != nil {
			return err
		}
	}

	writeReqs := []*dynamodb.WriteRequest{}
	for _, chunk := range chunks {
		metricName, ok := chunk.Metric[model.MetricNameLabel]
		if !ok {
			return fmt.Errorf("no MetricNameLabel for chunk")
		}
		// TODO compression
		chunkValue, err := json.Marshal(chunk)
		if err != nil {
			return err
		}

		for _, hour := range bigBuckets(chunk.From, chunk.Through) {
			hashValue := fmt.Sprintf("%d:%s", hour, metricName)

			for label, value := range chunk.Metric {
				// TODO escaping

				rangeValue := fmt.Sprintf("%s=%s:%s", label, value, chunk.ID)
				writeReqs = append(writeReqs, &dynamodb.WriteRequest{
					PutRequest: &dynamodb.PutRequest{
						Item: map[string]*dynamodb.AttributeValue{
							hashKey:  {S: aws.String(hashValue)},
							rangeKey: {S: aws.String(rangeValue)},
							chunkKey: {B: chunkValue},
						},
					},
				})
			}
		}
	}

	var resp *dynamodb.BatchWriteItemOutput
	err := timeRequest("BatchWriteItem", dynamoRequestDuration, func() error {
		var err error
		resp, err = c.dynamodb.BatchWriteItem(&dynamodb.BatchWriteItemInput{
			RequestItems:           map[string][]*dynamodb.WriteRequest{c.tableName: writeReqs},
			ReturnConsumedCapacity: aws.String(dynamodb.ReturnConsumedCapacityTotal),
		})
		return err
	})
	for _, cc := range resp.ConsumedCapacity {
		dynamoConsumedCapacity.WithLabelValues("PutItem").
			Add(float64(*cc.CapacityUnits))
	}
	return err
}

// Get implements ChunkStore
func (c *AWSChunkStore) Get(from, through model.Time, matchers ...*metric.LabelMatcher) ([]Chunk, error) {
	chunks, err := c.lookupChunks(from, through, matchers)
	if err != nil {
		return nil, err
	}
	return c.fetchChunkData(chunks)
}

func (c *AWSChunkStore) lookupChunks(from, through model.Time, matchers []*metric.LabelMatcher) ([]Chunk, error) {
	var metricName model.LabelValue
	for _, matcher := range matchers {
		if matcher.Name == model.MetricNameLabel && matcher.Type == metric.Equal {
			metricName = matcher.Value
			continue
		}
	}
	if metricName == "" {
		return nil, fmt.Errorf("no matcher for MetricNameLabel")
	}

	incomingChunkSets := make(chan []Chunk)
	incomingErrors := make(chan error)
	buckets := bigBuckets(from, through)
	for _, hour := range buckets {
		// TODO: probably better to abort everything using contexts on the first error.
		go c.lookupChunksFor(hour, metricName, matchers, incomingChunkSets, incomingErrors)
	}

	chunkSets := []Chunk{}
	errors := []error{}
	for i := 0; i < len(buckets); i++ {
		select {
		case chunkSet := <-incomingChunkSets:
			chunkSets = append(chunkSets, chunkSet...)
		case err := <-incomingErrors:
			errors = append(errors, err)
		}
	}
	if len(errors) > 0 {
		return nil, errors[0]
	}
	return chunkSets, nil
}

func (c *AWSChunkStore) lookupChunksFor(hour int64, metricName model.LabelValue, matchers []*metric.LabelMatcher, incomingChunkSets chan []Chunk, incomingErrors chan error) {
	var chunkSets [][]Chunk
	for _, matcher := range matchers {
		// TODO build support for other matchers
		if matcher.Type != metric.Equal {
			incomingErrors <- fmt.Errorf("%s matcher not supported yet", matcher.Type)
			return
		}

		// TODO escaping - this will break if label values contain the separator (:)
		hashValue := fmt.Sprintf("%d:%s", hour, metricName)
		rangeMinValue := fmt.Sprintf("%s=%s:%s", matcher.Name, matcher.Value, minChunkID)
		rangeMaxValue := fmt.Sprintf("%s=%s:%s", matcher.Name, matcher.Value, maxChunkID)

		var resp *dynamodb.QueryOutput
		err := timeRequest("Query", dynamoRequestDuration, func() error {
			var err error
			resp, err = c.dynamodb.Query(&dynamodb.QueryInput{
				TableName: aws.String(c.tableName),
				KeyConditions: map[string]*dynamodb.Condition{
					hashKey: {
						AttributeValueList: []*dynamodb.AttributeValue{
							{S: aws.String(hashValue)},
						},
						ComparisonOperator: aws.String("EQ"),
					},
					rangeKey: {
						AttributeValueList: []*dynamodb.AttributeValue{
							{S: aws.String(rangeMinValue)},
							{S: aws.String(rangeMaxValue)},
						},
						ComparisonOperator: aws.String("BETWEEN"),
					},
				},
				ReturnConsumedCapacity: aws.String(dynamodb.ReturnConsumedCapacityTotal),
			})
			return err
		})
		if resp.ConsumedCapacity != nil {
			dynamoConsumedCapacity.WithLabelValues("Query").
				Add(float64(*resp.ConsumedCapacity.CapacityUnits))
		}
		if err != nil {
			log.Errorf("Error querying DynamoDB: %v", err)
			incomingErrors <- err
			return
		}

		chunkSet := []Chunk{}
		for _, item := range resp.Items {
			rangeValue := item[rangeKey].S
			if rangeValue == nil {
				log.Errorf("Invalid item: %v", item)
				incomingErrors <- err
				return
			}
			parts := strings.SplitN(*rangeValue, ":", 2)
			if len(parts) != 2 {
				log.Errorf("Invalid item: %v", item)
				incomingErrors <- err
				return
			}
			chunkValue := item[chunkKey].B
			if rangeValue == nil {
				log.Errorf("Invalid item: %v", item)
				incomingErrors <- err
				return
			}
			chunk := Chunk{
				ID: parts[1],
			}
			if err := json.Unmarshal(chunkValue, &chunk); err != nil {
				log.Errorf("Invalid item: %v", item)
				incomingErrors <- err
				return
			}
			chunkSet = append(chunkSet, chunk)
		}
		chunkSets = append(chunkSets, chunkSet)
	}
	incomingChunkSets <- nWayIntersect(chunkSets)
}

func (c *AWSChunkStore) fetchChunkData(chunkSet []Chunk) ([]Chunk, error) {
	incomingChunks := make(chan Chunk)
	incomingErrors := make(chan error)
	for _, chunk := range chunkSet {
		go func(chunk Chunk) {
			var resp *s3.GetObjectOutput
			err := timeRequest("Get", s3RequestDuration, func() error {
				var err error
				resp, err = c.s3.GetObject(&s3.GetObjectInput{
					Bucket: aws.String(c.bucketName),
					Key:    aws.String(chunk.ID),
				})
				return err
			})
			if err != nil {
				incomingErrors <- err
				return
			}
			var buf bytes.Buffer
			if _, err := buf.ReadFrom(resp.Body); err != nil {
				incomingErrors <- err
				return
			}
			chunk.Data = buf.Bytes()
			incomingChunks <- chunk
		}(chunk)
	}

	chunks := []Chunk{}
	errors := []error{}
	for i := 0; i < len(chunkSet); i++ {
		select {
		case chunk := <-incomingChunks:
			chunks = append(chunks, chunk)
		case err := <-incomingErrors:
			errors = append(errors, err)
		}
	}
	if len(errors) > 0 {
		return nil, errors[0]
	}
	return chunks, nil
}

func nWayIntersect(sets [][]Chunk) []Chunk {
	l := len(sets)
	switch l {
	case 0:
		return []Chunk{}
	case 1:
		return sets[0]
	case 2:
		var (
			left, right = sets[0], sets[1]
			i, j        = 0, 0
			result      = []Chunk{}
		)
		for i < len(left) && j < len(right) {
			if left[i].ID == right[j].ID {
				result = append(result, left[i])
			}

			if left[i].ID < right[j].ID {
				i++
			} else {
				j++
			}
		}
		return result
	default:
		var (
			split = l / 2
			left  = nWayIntersect(sets[:split])
			right = nWayIntersect(sets[split:])
		)
		return nWayIntersect([][]Chunk{left, right})
	}
}
