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

	"cloud.google.com/go/internal/trace"
	bq "google.golang.org/api/bigquery/v2"
)

// QueryConfig holds the configuration for a query job.
type QueryConfig struct {
	// Dst is the table into which the results of the query will be written.
	// If this field is nil, a temporary table will be created.
	Dst *Table

	// The query to execute. See https://cloud.google.com/bigquery/query-reference for details.
	Q string

	// DefaultProjectID and DefaultDatasetID specify the dataset to use for unqualified table names in the query.
	// If DefaultProjectID is set, DefaultDatasetID must also be set.
	DefaultProjectID string
	DefaultDatasetID string

	// TableDefinitions describes data sources outside of BigQuery.
	// The map keys may be used as table names in the query string.
	//
	// When a QueryConfig is returned from Job.Config, the map values
	// are always of type *ExternalDataConfig.
	TableDefinitions map[string]ExternalData

	// CreateDisposition specifies the circumstances under which the destination table will be created.
	// The default is CreateIfNeeded.
	CreateDisposition TableCreateDisposition

	// WriteDisposition specifies how existing data in the destination table is treated.
	// The default is WriteEmpty.
	WriteDisposition TableWriteDisposition

	// DisableQueryCache prevents results being fetched from the query cache.
	// If this field is false, results are fetched from the cache if they are available.
	// The query cache is a best-effort cache that is flushed whenever tables in the query are modified.
	// Cached results are only available when TableID is unspecified in the query's destination Table.
	// For more information, see https://cloud.google.com/bigquery/querying-data#querycaching
	DisableQueryCache bool

	// DisableFlattenedResults prevents results being flattened.
	// If this field is false, results from nested and repeated fields are flattened.
	// DisableFlattenedResults implies AllowLargeResults
	// For more information, see https://cloud.google.com/bigquery/docs/data#nested
	DisableFlattenedResults bool

	// AllowLargeResults allows the query to produce arbitrarily large result tables.
	// The destination must be a table.
	// When using this option, queries will take longer to execute, even if the result set is small.
	// For additional limitations, see https://cloud.google.com/bigquery/querying-data#largequeryresults
	AllowLargeResults bool

	// Priority specifies the priority with which to schedule the query.
	// The default priority is InteractivePriority.
	// For more information, see https://cloud.google.com/bigquery/querying-data#batchqueries
	Priority QueryPriority

	// MaxBillingTier sets the maximum billing tier for a Query.
	// Queries that have resource usage beyond this tier will fail (without
	// incurring a charge). If this field is zero, the project default will be used.
	MaxBillingTier int

	// MaxBytesBilled limits the number of bytes billed for
	// this job.  Queries that would exceed this limit will fail (without incurring
	// a charge).
	// If this field is less than 1, the project default will be
	// used.
	MaxBytesBilled int64

	// UseStandardSQL causes the query to use standard SQL. The default.
	// Deprecated: use UseLegacySQL.
	UseStandardSQL bool

	// UseLegacySQL causes the query to use legacy SQL.
	UseLegacySQL bool

	// Parameters is a list of query parameters. The presence of parameters
	// implies the use of standard SQL.
	// If the query uses positional syntax ("?"), then no parameter may have a name.
	// If the query uses named syntax ("@p"), then all parameters must have names.
	// It is illegal to mix positional and named syntax.
	Parameters []QueryParameter

	// TimePartitioning specifies time-based partitioning
	// for the destination table.
	TimePartitioning *TimePartitioning

	// Clustering specifies the data clustering configuration for the destination table.
	Clustering *Clustering

	// The labels associated with this job.
	Labels map[string]string

	// If true, don't actually run this job. A valid query will return a mostly
	// empty response with some processing statistics, while an invalid query will
	// return the same error it would if it wasn't a dry run.
	//
	// Query.Read will fail with dry-run queries. Call Query.Run instead, and then
	// call LastStatus on the returned job to get statistics. Calling Status on a
	// dry-run job will fail.
	DryRun bool

	// Custom encryption configuration (e.g., Cloud KMS keys).
	DestinationEncryptionConfig *EncryptionConfig

	// Allows the schema of the destination table to be updated as a side effect of
	// the query job.
	SchemaUpdateOptions []string
}

func (qc *QueryConfig) toBQ() (*bq.JobConfiguration, error) {
	qconf := &bq.JobConfigurationQuery{
		Query:                              qc.Q,
		CreateDisposition:                  string(qc.CreateDisposition),
		WriteDisposition:                   string(qc.WriteDisposition),
		AllowLargeResults:                  qc.AllowLargeResults,
		Priority:                           string(qc.Priority),
		MaximumBytesBilled:                 qc.MaxBytesBilled,
		TimePartitioning:                   qc.TimePartitioning.toBQ(),
		Clustering:                         qc.Clustering.toBQ(),
		DestinationEncryptionConfiguration: qc.DestinationEncryptionConfig.toBQ(),
		SchemaUpdateOptions:                qc.SchemaUpdateOptions,
	}
	if len(qc.TableDefinitions) > 0 {
		qconf.TableDefinitions = make(map[string]bq.ExternalDataConfiguration)
	}
	for name, data := range qc.TableDefinitions {
		qconf.TableDefinitions[name] = data.toBQ()
	}
	if qc.DefaultProjectID != "" || qc.DefaultDatasetID != "" {
		qconf.DefaultDataset = &bq.DatasetReference{
			DatasetId: qc.DefaultDatasetID,
			ProjectId: qc.DefaultProjectID,
		}
	}
	if tier := int64(qc.MaxBillingTier); tier > 0 {
		qconf.MaximumBillingTier = &tier
	}
	f := false
	if qc.DisableQueryCache {
		qconf.UseQueryCache = &f
	}
	if qc.DisableFlattenedResults {
		qconf.FlattenResults = &f
		// DisableFlattenResults implies AllowLargeResults.
		qconf.AllowLargeResults = true
	}
	if qc.UseStandardSQL && qc.UseLegacySQL {
		return nil, errors.New("bigquery: cannot provide both UseStandardSQL and UseLegacySQL")
	}
	if len(qc.Parameters) > 0 && qc.UseLegacySQL {
		return nil, errors.New("bigquery: cannot provide both Parameters (implying standard SQL) and UseLegacySQL")
	}
	ptrue := true
	pfalse := false
	if qc.UseLegacySQL {
		qconf.UseLegacySql = &ptrue
	} else {
		qconf.UseLegacySql = &pfalse
	}
	if qc.Dst != nil && !qc.Dst.implicitTable() {
		qconf.DestinationTable = qc.Dst.toBQ()
	}
	for _, p := range qc.Parameters {
		qp, err := p.toBQ()
		if err != nil {
			return nil, err
		}
		qconf.QueryParameters = append(qconf.QueryParameters, qp)
	}
	return &bq.JobConfiguration{
		Labels: qc.Labels,
		DryRun: qc.DryRun,
		Query:  qconf,
	}, nil
}

func bqToQueryConfig(q *bq.JobConfiguration, c *Client) (*QueryConfig, error) {
	qq := q.Query
	qc := &QueryConfig{
		Labels:                      q.Labels,
		DryRun:                      q.DryRun,
		Q:                           qq.Query,
		CreateDisposition:           TableCreateDisposition(qq.CreateDisposition),
		WriteDisposition:            TableWriteDisposition(qq.WriteDisposition),
		AllowLargeResults:           qq.AllowLargeResults,
		Priority:                    QueryPriority(qq.Priority),
		MaxBytesBilled:              qq.MaximumBytesBilled,
		UseLegacySQL:                qq.UseLegacySql == nil || *qq.UseLegacySql,
		TimePartitioning:            bqToTimePartitioning(qq.TimePartitioning),
		Clustering:                  bqToClustering(qq.Clustering),
		DestinationEncryptionConfig: bqToEncryptionConfig(qq.DestinationEncryptionConfiguration),
		SchemaUpdateOptions:         qq.SchemaUpdateOptions,
	}
	qc.UseStandardSQL = !qc.UseLegacySQL

	if len(qq.TableDefinitions) > 0 {
		qc.TableDefinitions = make(map[string]ExternalData)
	}
	for name, qedc := range qq.TableDefinitions {
		edc, err := bqToExternalDataConfig(&qedc)
		if err != nil {
			return nil, err
		}
		qc.TableDefinitions[name] = edc
	}
	if qq.DefaultDataset != nil {
		qc.DefaultProjectID = qq.DefaultDataset.ProjectId
		qc.DefaultDatasetID = qq.DefaultDataset.DatasetId
	}
	if qq.MaximumBillingTier != nil {
		qc.MaxBillingTier = int(*qq.MaximumBillingTier)
	}
	if qq.UseQueryCache != nil && !*qq.UseQueryCache {
		qc.DisableQueryCache = true
	}
	if qq.FlattenResults != nil && !*qq.FlattenResults {
		qc.DisableFlattenedResults = true
	}
	if qq.DestinationTable != nil {
		qc.Dst = bqToTable(qq.DestinationTable, c)
	}
	for _, qp := range qq.QueryParameters {
		p, err := bqToQueryParameter(qp)
		if err != nil {
			return nil, err
		}
		qc.Parameters = append(qc.Parameters, p)
	}
	return qc, nil
}

// QueryPriority specifies a priority with which a query is to be executed.
type QueryPriority string

const (
	// BatchPriority specifies that the query should be scheduled with the
	// batch priority.  BigQuery queues each batch query on your behalf, and
	// starts the query as soon as idle resources are available, usually within
	// a few minutes. If BigQuery hasn't started the query within 24 hours,
	// BigQuery changes the job priority to interactive. Batch queries don't
	// count towards your concurrent rate limit, which can make it easier to
	// start many queries at once.
	//
	// More information can be found at https://cloud.google.com/bigquery/docs/running-queries#batchqueries.
	BatchPriority QueryPriority = "BATCH"
	// InteractivePriority specifies that the query should be scheduled with
	// interactive priority, which means that the query is executed as soon as
	// possible. Interactive queries count towards your concurrent rate limit
	// and your daily limit. It is the default priority with which queries get
	// executed.
	//
	// More information can be found at https://cloud.google.com/bigquery/docs/running-queries#queries.
	InteractivePriority QueryPriority = "INTERACTIVE"
)

// A Query queries data from a BigQuery table. Use Client.Query to create a Query.
type Query struct {
	JobIDConfig
	QueryConfig
	client *Client
}

// Query creates a query with string q.
// The returned Query may optionally be further configured before its Run method is called.
func (c *Client) Query(q string) *Query {
	return &Query{
		client:      c,
		QueryConfig: QueryConfig{Q: q},
	}
}

// Run initiates a query job.
func (q *Query) Run(ctx context.Context) (j *Job, err error) {
	ctx = trace.StartSpan(ctx, "cloud.google.com/go/bigquery.Query.Run")
	defer func() { trace.EndSpan(ctx, err) }()

	job, err := q.newJob()
	if err != nil {
		return nil, err
	}
	j, err = q.client.insertJob(ctx, job, nil)
	if err != nil {
		return nil, err
	}
	return j, nil
}

func (q *Query) newJob() (*bq.Job, error) {
	config, err := q.QueryConfig.toBQ()
	if err != nil {
		return nil, err
	}
	return &bq.Job{
		JobReference:  q.JobIDConfig.createJobRef(q.client),
		Configuration: config,
	}, nil
}

// Read submits a query for execution and returns the results via a RowIterator.
// It is a shorthand for Query.Run followed by Job.Read.
func (q *Query) Read(ctx context.Context) (*RowIterator, error) {
	job, err := q.Run(ctx)
	if err != nil {
		return nil, err
	}
	return job.Read(ctx)
}
