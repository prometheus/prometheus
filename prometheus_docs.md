# Prometheus Repository Documentation

## Introduction

Prometheus is an open-source systems monitoring and alerting toolkit originally built at SoundCloud. Since its inception in 2012, Prometheus has become a very popular monitoring solution, and it is now a standalone open source project maintained independently of any company. Prometheus joined the Cloud Native Computing Foundation (CNCF) in 2016 as the second hosted project, after Kubernetes.

This document provides a comprehensive overview of the Prometheus repository, its architecture, core components, data flow, and contribution guidelines.

## High-Level Architecture

Prometheus has a modular architecture consisting of several core components that work together to provide a powerful monitoring solution. The main components are:

*   **Prometheus Server:** The core of Prometheus, responsible for scraping and storing time series data.
*   **Client Libraries:** Instrumenting application code to expose metrics.
*   **Push Gateway:** For supporting short-lived jobs.
*   **Exporters:** For services that don't directly expose Prometheus metrics (e.g., HAProxy, StatsD, Graphite).
*   **Alertmanager:** Handles alerts sent by client applications (including the Prometheus server itself).
*   **Service Discovery:** Dynamically discovers targets to scrape.

The Prometheus server scrapes metrics from instrumented jobs, either directly or via an intermediary Push Gateway for short-lived jobs. It stores all scraped samples locally and runs rules over this data to either aggregate and record new time series from existing data or generate alerts. The Alertmanager is then used to handle these alerts.

This document will focus on the core Prometheus server components found within this repository.

## Core Components

This section will detail the core components of the Prometheus server.

### `cmd/`

The `cmd/` directory contains the main applications for Prometheus. The `README.md` in the root directory provides a good overview of the `prometheus` and `promtool` executables.

*   **`cmd/prometheus/main.go`**: This is the entry point for the main Prometheus server.
    *   It initializes and manages the lifecycle of all other components.
    *   Handles command-line flags for configuring the server (e.g., storage path, web listen address, configuration file).
    *   Sets up the HTTP server for the UI, API, and telemetry.
    *   Manages the loading and reloading of the configuration file.
    *   Can run in two modes: "server" (the default) and "agent". Agent mode is a write-only mode optimized for remote write scenarios.
    *   Integrates various managers for different functionalities like scrape, query, rules, and notifier.
    *   Initializes the TSDB (Time Series Database) or WAL (Write-Ahead Log) storage based on the mode.
*   **`cmd/promtool/main.go`**: This is the entry point for the `promtool` command-line utility.
    *   Provides a suite of tools for users and operators to work with Prometheus.
    *   **`check`**:
        *   `check config <config-files...>`: Validates Prometheus configuration files.
        *   `check web-config <web-config-files...>`: Validates web configuration files (for TLS, basic auth).
        *   `check rules <rule-files...>`: Validates rule files syntax and performs linting.
        *   `check metrics`: Lints metrics exposed by an instance (read from stdin).
        *   `check healthy --url=<prometheus-url>`: Checks if the Prometheus server at the given URL is healthy.
        *   `check ready --url=<prometheus-url>`: Checks if the Prometheus server at the given URL is ready to serve traffic.
        *   `check service-discovery <config-file> <job-name>`: Performs service discovery for a given job and reports the results, including relabeling.
    *   **`query`**:
        *   `query instant <server-url> <expr>`: Executes an instant query.
        *   `query range <server-url> <expr>`: Executes a range query.
        *   `query series <server-url> --match=<selector>`: Finds series by label matchers.
        *   `query labels <server-url> <label-name>`: Finds label values for a given label name.
        *   `query analyze --server=<prometheus-url> --type=<metric-type> --match=<selector>`: Analyzes usage patterns of specific metrics.
    *   **`test`**:
        *   `test rules <test-rule-file...>`: Unit tests rule files.
    *   **`tsdb`**: Provides commands for inspecting and managing TSDB data.
        *   `tsdb bench write`: Runs a write performance benchmark.
        *   `tsdb analyze`: Analyzes churn, label pair cardinality, and compaction efficiency.
        *   `tsdb list`: Lists TSDB blocks.
        *   `tsdb dump`: Dumps samples from a TSDB.
        *   `tsdb dump-openmetrics`: Dumps samples from a TSDB into OpenMetrics text format.
        *   `tsdb create-blocks-from openmetrics <input-file> <output-directory>`: Imports samples from OpenMetrics input and produces TSDB blocks.
        *   `tsdb create-blocks-from rules --url=<prometheus-url> <rule-files...>`: Creates blocks of data for new recording rules by backfilling from a live Prometheus instance.
    *   **`debug`**: Fetches debug information from a running Prometheus server.
        *   `debug pprof <server-url>`: Fetches profiling (pprof) debug information.
        *   `debug metrics <server-url>`: Fetches metrics debug information.
        *   `debug all <server-url>`: Fetches all debug information.
    *   **`push metrics`**: Pushes metrics to a Prometheus remote write endpoint (intended for testing).
    *   **`promql`** (experimental):
        *   `promql format <query>`: Formats a PromQL query.
        *   `promql label-matchers set <query> <name> <value>`: Sets a label matcher in a query.
        *   `promql label-matchers delete <query> <name>`: Deletes a label matcher from a query.
    *   Accepts various flags for features like enabling experimental functions or delayed name removal in PromQL.

### `config/`

The `config/` directory is responsible for handling Prometheus configuration files. It defines the structure of the `prometheus.yml` file and provides functions to load, parse, and validate it.

*   **`config/config.go`**:
    *   Defines the Go structs that map to the Prometheus YAML configuration file (e.g., `Config`, `GlobalConfig`, `ScrapeConfig`, `AlertmanagerConfig`, `RemoteWriteConfig`, `RemoteReadConfig`, `RuleFile`, etc.).
    *   Contains default values for various configuration options (e.g., `DefaultGlobalConfig`, `DefaultScrapeConfig`).
    *   Implements `UnmarshalYAML` methods for custom parsing logic and validation of configuration sections.
    *   Handles global overrides for scrape configurations (e.g., scrape interval, timeout).
    *   Validates external labels, rule files, scrape config files, and remote write/read configurations.
    *   Includes logic for expanding environment variables in external labels.
    *   Defines supported scrape protocols (e.g., OpenMetrics, Prometheus text format) and their default order.
    *   Manages OTLP (OpenTelemetry Protocol) configuration, including translation strategies for metric and label names.
    *   Handles storage-related configurations like TSDB settings (e.g., out-of-order time window) and exemplar storage.
    *   Defines tracing configuration options.
*   **`config/reload.go`**:
    *   Provides functionality to generate a checksum for the main configuration file and all files it includes (rule files, scrape config files specified via `rule_files` and `scrape_config_files` globs).
    *   This checksum is used by the Prometheus server (when the `auto-reload-config` feature flag is enabled) to detect changes in configuration files and trigger an automatic reload.
    *   The `GenerateChecksum` function reads the main YAML file, parses it to find references to other files (rule files and scrape config files), and then includes the content of these referenced files in the checksum calculation. This ensures that changes in any of these dependent files also trigger a reload.

The configuration loading process involves:
1.  Reading the main `prometheus.yml` file.
2.  Parsing the YAML content into the `Config` struct.
3.  Applying default values for various options.
4.  Validating the configuration, including syntax and semantic checks (e.g., ensuring job names are unique, scrape timeouts are not greater than scrape intervals).
5.  Resolving relative file paths for rule files, scrape config files, and TLS/authorization credentials based on the directory of the main configuration file.
6.  For scrape configurations, it also loads and validates any scrape configurations specified in separate files via `scrape_config_files`.

Prometheus supports runtime reloading of its configuration. When a reload is triggered (via SIGHUP or the HTTP reload endpoint), it re-reads the configuration file and applies the changes. The `reload.go` functionality enhances this by allowing automatic reloads when file changes are detected.

### `discovery/`

The `discovery/` directory contains the logic for service discovery, enabling Prometheus to dynamically find and monitor targets. The `discovery/README.md` provides a good overview of the design principles for service discovery mechanisms within Prometheus.

*   **Core Concepts**:
    *   **Discoverer Interface (`discovery.go`)**: Defines the interface that all service discovery providers must implement. The key method is `Run(ctx context.Context, up chan<- []*targetgroup.Group)`, which is responsible for sending updates about target groups.
    *   **Target Group (`targetgroup/targetgroup.go`)**: A `TargetGroup` is a collection of targets that share a common set of labels. Each target group has a `Source` field that uniquely identifies it within a discovery provider.
    *   **Config Interface (`discovery.go`)**: Defines the interface for service discovery configurations. Each SD mechanism (e.g., `kubernetes_sd_configs`, `ec2_sd_configs`) implements this. It has a `Name()` method and a `NewDiscoverer()` method to create an instance of the discoverer.
    *   **Registry (`registry.go`)**: Manages the registration of different service discovery `Config` implementations. This allows Prometheus to map configuration blocks (e.g., `kubernetes_sd_configs`) to their respective discovery logic. It uses reflection to handle different configuration structures.
    *   **Manager (`manager.go`)**: The `Manager` is central to service discovery.
        *   It maintains a pool of active discovery providers based on the Prometheus configuration.
        *   It receives `TargetGroup` updates from these providers.
        *   It aggregates updates from all providers and sends them to a `syncCh` channel. This channel is consumed by other components like the scrape manager and notifier manager.
        *   It handles the lifecycle of discovery providers: starting new ones, stopping obsolete ones, and reloading configurations.
        *   It throttles updates using `updatert` to avoid overwhelming consumers with frequent changes.
        *   It exposes metrics about the discovery process (e.g., number of discovered targets, number of failed configs).
        *   The `ApplyConfig` method is crucial for updating the set of active discovery providers when the Prometheus configuration is reloaded. It compares the new configuration with the existing providers, starts new ones, stops old ones, and updates existing ones if their configuration hasn't changed but their set of consumers (e.g., different scrape jobs using the same SD config) has.
        *   It uses a `poolKey` (setName, providerName) to manage targets from different providers and for different "sets" (scrape jobs or alertmanager configs).

*   **Workflow**:
    1.  Prometheus loads its configuration (`prometheus.yml`).
    2.  For each scrape job or Alertmanager configuration, the `discovery.Manager` is given the list of service discovery configurations (e.g., `static_configs`, `kubernetes_sd_configs`).
    3.  The `Manager` looks up the corresponding `Config` implementation in the `registry`.
    4.  It calls `NewDiscoverer()` on the `Config` to create a new `Discoverer` instance for each SD configuration.
    5.  The `Manager` calls the `Run()` method on each `Discoverer`.
    6.  Each `Discoverer` starts its own process (e.g., watching the Kubernetes API, polling the EC2 API, reading a file). When it finds targets, it groups them into `TargetGroup` objects and sends them through the channel provided to `Run()`.
    7.  The `discovery.Manager` receives these `TargetGroup`s via an internal `updater` goroutine for each provider.
    8.  The `updater` stores these target groups in an internal map (`m.targets`), keyed by `poolKey` and the target group's `Source`.
    9.  Periodically, or when triggered by `triggerSend`, the `sender` goroutine in the `Manager` compiles a complete list of all target groups from all active providers.
    10. This aggregated list (a `map[string][]*targetgroup.Group`, where the key is the scrape job name or alertmanager config name) is sent to the `syncCh`.
    11. The scrape manager and notifier manager consume these updates from `syncCh` to get the latest list of targets to scrape or Alertmanagers to notify.

*   **Key Principles from `discovery/README.md`**:
    *   SD mechanisms should be well-established and used across multiple organizations.
    *   They should extract all potentially useful metadata, allowing users to filter and transform it using relabeling. Metadata labels are prefixed with `__meta_<sdname>_<key>`.
    *   `__address__` label is mandatory and should contain the `host:port` of the target.
    *   No business logic or hardcoded assumptions in the SD provider itself; customization should be done via relabeling.
    *   Arrays of tags/metadata should be joined by commas and enclosed in commas (e.g., `,tag1,tag2,`).
    *   Map-like metadata should be exposed as individual labels.
    *   Filtering options offered by the underlying SD mechanism can be exposed as a performance optimization, but relabeling should always be able to achieve the same.
    *   Configuration should come from the Prometheus configuration file, not environment variables or other external files directly read by the SD provider.
    *   Secrets should not be exposed in metadata.

*   The `discovery/` directory contains subdirectories for each specific service discovery mechanism (e.g., `discovery/kubernetes/`, `discovery/ec2/`, `discovery/file/`, etc.). Each of these implements the `Config` and `Discoverer` interfaces for that particular platform.

### `promql/`

The `promql/` directory houses the Prometheus Query Language (PromQL) implementation, including the parser, engine, and available functions.

*   **`promql/parser/parse.go`**:
    *   Defines the PromQL parser. It takes a query string as input and produces an Abstract Syntax Tree (AST).
    *   Uses a lexer (`promql/parser/lex.go`) to tokenize the input string.
    *   The parser is generated using `goyacc` (a Go version of yacc). The grammar is defined in `promql/parser/generated_parser.y`.
    *   Defines the structure of different expression types (e.g., `AggregateExpr`, `BinaryExpr`, `Call`, `MatrixSelector`, `VectorSelector`, `NumberLiteral`, `StringLiteral`).
    *   Performs syntax validation during parsing.
    *   Includes functions for type-checking the AST after parsing to ensure semantic validity (e.g., correct argument types for functions, valid operations between types).
    *   `ParseExpr()` is the main function to parse a PromQL query string into an `Expr` (expression node).
    *   `ParseMetric()` and `ParseMetricSelector()` are used for parsing metric names and label selectors respectively.
    *   Handles operator precedence and associativity.
    *   Supports experimental duration expression parsing via the `ExperimentalDurationExpr` flag.

*   **`promql/engine.go`**:
    *   Implements the PromQL query execution engine.
    *   The `Engine` struct manages query execution, including handling timeouts, concurrency, and logging.
    *   `NewEngine()` creates a new query engine with configurable options (logger, max samples, timeout, lookback delta, etc.).
    *   `NewInstantQuery()` and `NewRangeQuery()` methods are used to create executable `Query` objects from a query string and time parameters.
    *   The `exec()` method (internal) is the core of query execution. It walks the AST and evaluates expressions.
    *   It interacts with a `storage.Queryable` to fetch raw time series data.
    *   The `evaluator` struct handles the actual evaluation of expressions at different timestamps (for range queries) or a single timestamp (for instant queries).
    *   **Evaluation Steps**:
        1.  **Query Parsing**: The query string is parsed into an AST (done before reaching the engine's execution logic, typically by `NewInstantQuery` or `NewRangeQuery`).
        2.  **Preprocessing**: The `PreprocessExpr` function wraps step-invariant parts of the query with `StepInvariantExpr` to optimize evaluation by computing them only once. It also resolves `@` modifier timestamps.
        3.  **Series Expansion**: For vector and matrix selectors, the engine fetches the relevant series data from the storage using the `Querier`. This involves resolving label matchers and time ranges.
        4.  **Node Evaluation**: The engine recursively evaluates the AST nodes:
            *   **Literals**: Return their fixed value.
            *   **Selectors (`VectorSelector`, `MatrixSelector`)**: Fetch and return time series data based on label matchers and time ranges. For matrix selectors, it retrieves a range of data points for each selected series.
            *   **Aggregations (`AggregateExpr`)**: Group series based on `by` or `without` clauses and apply aggregation functions (e.g., `sum`, `avg`, `count`).
            *   **Function Calls (`Call`)**: Execute PromQL functions. For functions operating on range vectors (like `rate`, `delta`), the engine evaluates the inner matrix selector and then applies the function to the resulting series.
            *   **Binary Operations (`BinaryExpr`)**: Perform arithmetic or logical operations between scalars, vectors, or a scalar and a vector. Vector matching (one-to-one, many-to-one, one-to-many) is handled here.
            *   **Subqueries (`SubqueryExpr`)**: Evaluate an inner query over a range and at a specified resolution, treating its result as a matrix for further processing.
        5.  **Result Assembly**: The final result is assembled into one of PromQL's value types: `Scalar`, `Vector`, `Matrix`, or `String`.
    *   Manages query lifecycle, including cancellation and statistics collection (timings, samples).
    *   Handles lookback delta for stale series detection.
    *   Implements logic for `@` modifier (evaluating expressions at a specific time) and offset modifiers.
    *   Includes `QueryTracker` for managing active queries and enforcing concurrency limits.
    *   Supports query logging via a `QueryLogger` interface.

*   **`promql/functions.go`**:
    *   Defines the implementations for all built-in PromQL functions (e.g., `rate`, `increase`, `delta`, `sum_over_time`, `avg_over_time`, `histogram_quantile`, `label_replace`, `time`, date functions, math functions).
    *   Each function is a `FunctionCall` type: `func(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations)`.
    *   `vals`: A list of evaluated arguments for the function.
    *   `args`: The original AST nodes of the arguments.
    *   `enh`: An `EvalNodeHelper` containing evaluation context like the current timestamp and reusable buffers.
    *   Functions operate on the evaluated arguments (Vectors, Matrices, Scalars) and return a `Vector` (even for scalar results, which are represented as a single-sample vector).
    *   Many functions that operate over time (e.g., `rate`, `avg_over_time`) take a `Matrix` as input (representing a range vector).
    *   Vectorized math functions (e.g., `abs`, `sqrt`, `ln`) iterate over samples in the input vector and apply the transformation.
    *   Functions like `label_replace` and `label_join` manipulate labels of series.
    *   `histogram_quantile` and `histogram_fraction` handle calculations on Prometheus histograms (both classic and native).
    *   `resets` and `changes` count counter resets or value changes in a range vector.
    *   `predict_linear` performs linear regression.
    *   `time`, `month`, `year`, etc., provide time-based information.

The PromQL engine is designed to be efficient by:
*   Lazy evaluation where possible.
*   Reusing data structures (e.g., point slices via `zeropool`).
*   Optimizing evaluation of step-invariant expressions.
*   Efficiently iterating over series data.

### `storage/`

The `storage/` directory defines interfaces and components related to the storage of time series data. It provides abstractions for how data is appended and queried, supporting both local and remote storage systems.

*   **`storage/interface.go`**:
    *   Defines the core interfaces for Prometheus storage:
        *   `Storage`: The top-level interface combining `SampleAndChunkQueryable`, `Appendable`, `StartTime()`, and `Close()`.
        *   `Appendable`: Provides an `Appender()` method to get a new data appender.
        *   `Appender`: Interface for ingesting data. It includes methods like `Append()`, `AppendExemplar()`, `AppendHistogram()`, `Commit()`, and `Rollback()`.
            *   `SeriesRef` is an opaque reference to a series, potentially speeding up subsequent appends to the same series.
            *   Defines errors like `ErrOutOfOrderSample`, `ErrOutOfBounds`, `ErrDuplicateSampleForTimestamp`.
        *   `Queryable`: Interface for querying sample data (e.g., for PromQL). Returns a `Querier`.
        *   `ChunkQueryable`: Interface for querying raw chunk data. Returns a `ChunkQuerier`.
        *   `Querier`: Provides methods to select series sets (`Select()`) and label values/names.
        *   `ChunkQuerier`: Similar to `Querier` but returns `ChunkSeriesSet`.
        *   `SeriesSet` and `ChunkSeriesSet`: Iterators over a set of series or chunked series.
        *   `Series` and `ChunkSeries`: Represent a single time series, allowing iteration over its samples or chunks.
        *   `ExemplarStorage`, `ExemplarQueryable`, `ExemplarAppender`: Interfaces for exemplar storage and querying.
        *   `MetadataUpdater`: Interface for updating metric metadata.
        *   `CreatedTimestampAppender`: Interface for appending created timestamps for "zero" samples (e.g. from OTLP).
    *   `SelectHints`: A struct that can be passed to `Select` methods to provide optimization hints to the storage (e.g., time range, step, function).

*   **`storage/fanout.go`**:
    *   Implements a `fanout` storage solution. This storage proxies reads and writes to a primary storage and multiple secondary storages.
    *   **Writes (`Appender`)**: Writes are sent to the primary storage first. If successful, they are then sent to all secondary storages. If any secondary storage write fails, the overall commit might still succeed (depending on the error from the primary), but an error will be logged.
    *   **Reads (`Querier`, `ChunkQuerier`)**:
        *   Reads are primarily served by the `primary` storage.
        *   Results from `secondary` storages are merged with the primary results.
        *   Errors from the primary querier are critical. Errors from secondary queriers are treated as warnings, and their results are discarded for that query.
        *   `NewMergeQuerier` and `NewMergeChunkQuerier` are used to combine results from multiple queriers.
    *   The Prometheus server typically uses the local TSDB as the primary storage and remote storage systems (configured via `remote_write` and `remote_read`) as secondary fanouts for reads (though remote write is a separate path, the `storage/remote` package implements the `Storage` interface which can be part of a fanout for reads).

*   **`storage/remote/storage.go`**:
    *   Implements a `Storage` that connects to remote HTTP endpoints for reading and writing time series data, as defined by the `remote_write` and `remote_read` sections in the Prometheus configuration.
    *   **Remote Write**:
        *   Managed by `WriteStorage` (from `storage/remote/write.go`, but `Storage` in `storage.go` acts as the entry point).
        *   Takes samples from an input queue (typically fed by the main Prometheus appender via a fanout mechanism).
        *   Batches samples and sends them to configured `remote_write` URLs using HTTP.
        *   Handles retries, sharding (sending data for different series to different remote write queues/shards for concurrency), and WAL (Write-Ahead Log) for resilience against crashes.
        *   The `ApplyConfig` method updates the set of remote write queues based on the Prometheus configuration. It identifies queues by a hash of their configuration to manage them across reloads.
    *   **Remote Read**:
        *   When a query comes in, the `Querier()` method creates a `MergeQuerier` that includes queriers for each configured `remote_read` endpoint.
        *   `NewSampleAndChunkQueryableClient` creates a queryable that fetches data from a remote read endpoint.
        *   It translates PromQL queries into the remote read request format (Protobuf).
        *   Handles external labels and required matchers for remote reads.
        *   Remote read is generally considered best-effort; failures from one remote read endpoint don't necessarily fail the entire query if other local or remote sources can provide data.

In a typical Prometheus setup:
1.  Data is scraped and appended to the local TSDB (Time Series Database). This local TSDB is the "primary" storage.
2.  If `remote_write` is configured, the `FanoutAppender` also sends the data to the `WriteStorage` in `storage/remote/`, which then forwards it to the remote endpoints.
3.  When a query is executed:
    *   The `FanoutQuerier` is used.
    *   It first queries the local TSDB (primary).
    *   It also queries any configured `remote_read` endpoints (as secondaries).
    *   The results are merged before being processed by the PromQL engine.

This architecture allows Prometheus to have durable local storage while also integrating with various long-term storage solutions or other Prometheus instances via the remote read/write protocols.

### `scrape/`

The `scrape/` directory is responsible for discovering and scraping metrics from configured targets.

*   **`scrape/manager.go`**:
    *   The `Manager` is the central component for managing scrape operations.
    *   It receives target group updates from the `discovery.Manager`.
    *   `ApplyConfig()`: Updates the scrape manager's configuration. It reloads scrape configurations, stops scrape pools for removed jobs, starts new ones for added jobs, and reloads existing ones if their configuration changed.
    *   `Run()`: Starts a loop that listens for target set updates and triggers reloads of scrape pools. It also has a `reloader()` goroutine that periodically calls `reload()` to process pending target updates.
    *   `reload()`: Iterates through the current target sets. If a scrape pool for a job doesn't exist, it creates a new `scrapePool`. It then calls `Sync()` on each `scrapePool` to update its targets.
    *   `sync(targets []*Target)` (on `scrapePool`): This method is called by the manager's `reload` or `Sync` (for target group updates). It takes a list of `Target` objects (derived from `targetgroup.Group` after relabeling), deduplicates them, starts new scrape loops for new targets, and stops loops for targets that disappeared.
    *   Maintains a map of `scrapePools`, one for each scrape job defined in the configuration.
    *   Handles the lifecycle of scrape pools (starting, stopping, reloading).
    *   Calculates a global `offsetSeed` based on external labels and hostname to help distribute scrape load across HA Prometheus setups.

*   **`scrape/scrape.go`**:
    *   Defines the `scrapePool` struct, which manages scraping for a set of targets belonging to a single scrape configuration (job).
        *   Each `scrapePool` has its own `http.Client`.
        *   `Sync(groups []*targetgroup.Group)`: This method is called by the `Manager`. It processes `targetgroup.Group` objects, applies relabeling to discover final targets, and then calls the `scrapePool.sync([]*Target)` method.
        *   `sync(targets []*Target)`: Updates the set of active targets within the pool. It starts new scrape loops for newly added targets and stops loops for targets that are no longer present. It ensures that only one scrape loop runs per unique target (based on its hash).
    *   Defines the `scrapeLoop` struct, which is responsible for repeatedly scraping a single target.
        *   `run()`: The main loop for a target. It waits for the scrape interval (with an initial offset), performs a scrape, reports the results, and then waits for the next interval.
        *   `scrapeAndReport()`: Handles the actual scrape attempt, appends data to storage, and reports scrape metrics (up, scrape_duration_seconds, etc.).
        *   Uses a `scraper` interface (implemented by `targetScraper`) to perform the HTTP request and read the response.
        *   `append()`: Parses the scraped metrics using a `textparse.Parser`, applies metric relabeling, and appends valid samples to the storage via an `storage.Appender`. It also handles staleness markers.
        *   `report()`: Appends scrape-specific metrics like `up`, `scrape_duration_seconds`, `scrape_samples_scraped`.
        *   Manages a `scrapeCache` for each target to store series references and metadata, improving performance by avoiding redundant lookups and label processing.
    *   `targetScraper`: Implements the `scraper` interface.
        *   `scrape()`: Performs the HTTP GET request to the target's metrics endpoint.
        *   `readResponse()`: Reads the HTTP response body, handles gzip compression, and respects body size limits.
    *   **Scrape Process per Target**:
        1.  A `scrapeLoop` is started for each target.
        2.  The loop waits for its calculated scrape interval (including jitter/offset).
        3.  It calls `scraper.scrape()` which makes an HTTP GET request to the target's `/metrics` endpoint (or configured path).
        4.  The response is read and parsed (e.g., Prometheus text format, OpenMetrics).
        5.  Metric relabeling rules (`metric_relabel_configs`) are applied.
        6.  Valid samples are appended to the storage using a `storage.Appender`. This appender might be a `timeLimitAppender` (to reject samples too far in the future) and a `sampleLimitAppender` (to cap the number of samples per scrape).
        7.  Staleness tracking: Compares the set of series from the current scrape with the previous one. If a series is no longer present, a stale marker (a special NaN value) is appended for it.
        8.  "up" metric and other scrape-specific metrics are generated and appended.
    *   Handles `honor_labels` and `honor_timestamps` configuration.
    *   Supports various scrape protocols (OpenMetrics, Prometheus text) and content negotiation.

*   **`scrape/target.go`**:
    *   Defines the `Target` struct, representing a single scrape endpoint.
    *   A `Target` includes:
        *   Its discovered labels (`tLabels` from the target itself, `tgLabels` from the target group).
        *   Its final set of labels after relabeling (`labels`).
        *   The scrape configuration (`scrapeConfig`) it belongs to.
        *   Health state (`HealthGood`, `HealthBad`, `HealthUnknown`).
        *   Information about the last scrape (time, duration, error).
        *   A `MetricMetadataStore` (usually the `scrapeCache`) to store and retrieve metadata (TYPE, HELP, UNIT) for metrics from this target.
    *   `URL()`: Constructs the scrape URL from labels (`__scheme__`, `__address__`, `__metrics_path__`) and configured parameters.
    *   `hash()`: Calculates a hash for the target, used to uniquely identify it.
    *   `offset()`: Calculates the initial scrape offset for a target to spread out load.
    *   `intervalAndTimeout()`: Determines the specific scrape interval and timeout for this target, potentially overridden by labels.
    *   `TargetsFromGroup()`: A helper function that takes a `targetgroup.Group` and a `config.ScrapeConfig` and generates a list of `Target` objects after applying target relabeling rules. Dropped targets are also identified here.
    *   `PopulateLabels()` and `PopulateDiscoveredLabels()`: Handle the process of taking initial labels from service discovery, applying relabeling rules, and adding standard Prometheus labels (like `instance`, `job`).

The scrape component ensures that Prometheus can efficiently and correctly fetch metrics from a dynamic set of targets, handle various data formats, and apply necessary transformations before ingesting the data into its storage.

### `rules/`

The `rules/` directory is responsible for managing recording and alerting rules. It periodically evaluates these rules and takes actions, such as generating new time series for recording rules or sending notifications for alerting rules.

*   **`rules/manager.go`**:
    *   The `Manager` is the core component for managing all rule groups.
    *   `NewManager()`: Creates a new rule manager with options like query function, notification function, storage appendable, external URL, etc.
    *   `Run()`: Starts the rule manager's processing loop.
    *   `Stop()`: Stops the rule manager and all its active rule groups.
    *   `Update()`: Reloads rule configurations from specified file paths. It compares new rule groups with existing ones:
        *   If a group is unchanged, it's kept.
        *   If a group has changed, the old one is stopped, and a new one is started (preserving state if possible).
        *   Old groups no longer in the configuration are stopped.
    *   `LoadGroups()`: Parses rule files (using a `GroupLoader`, by default `FileLoader` which uses `rulefmt.ParseFile`) into `RuleGroup` objects.
        *   It supports loading rules from multiple files and handles rule group intervals (defaulting to the global evaluation interval).
    *   Manages a map of `*Group` objects, keyed by a unique identifier for each group (filename + group name).
    *   `RuleGroups()` and `Rules()`: Provide access to the currently loaded rule groups and individual rules.
    *   `AlertingRules()`: Returns all currently loaded alerting rules.
    *   `SendAlerts()`: A helper function that formats alerts from `AlertingRule` evaluations and sends them via a `Notifier`'s `Send` method.
    *   `RuleConcurrencyController`: Manages how many rules can be evaluated concurrently.
        *   `concurrentRuleEvalController`: Uses a semaphore to limit concurrent rule evaluations based on `max_concurrent_evals`.
        *   `sequentialRuleEvalController`: Evaluates all rules sequentially (default if concurrent rule evaluation is not enabled).
    *   `RuleDependencyController`: Analyzes dependencies between rules within a group (e.g., a recording rule used by an alerting rule).
        *   `AnalyseRules()`: Builds a dependency map and sets dependent/dependency rules on each `Rule` object. This can be used by the `RuleConcurrencyController` to batch rule evaluations safely.

*   **`rules/group.go`** (inferred from `manager.go` and general Prometheus knowledge):
    *   Represents a group of rules as defined in a rule file.
    *   Contains a list of `Rule` (either `AlertingRule` or `RecordingRule`).
    *   Has its own evaluation interval, which overrides the global evaluation interval.
    *   `Eval()`: Iterates through its rules and evaluates them. For alerting rules, it appends resulting samples to a vector. For recording rules, it appends the recorded time series to storage.
    *   Manages the lifecycle (start, stop, run) of its rules.
    *   Keeps track of its last evaluation time and duration.

*   **`rules/alerting.go`**:
    *   Defines `AlertingRule` and associated alert states (`StateInactive`, `StatePending`, `StateFiring`).
    *   `NewAlertingRule()`: Constructs an `AlertingRule`.
    *   `Eval()`:
        1.  Executes the rule's PromQL query at the given timestamp.
        2.  For each resulting vector element (series):
            *   Generates labels for the alert using the rule's labels and the series's labels. Templates in labels and annotations are expanded.
            *   Creates an `Alert` object.
        3.  Compares the currently firing/pending series with the results of the current evaluation:
            *   **New active alerts**: If a series from the query result was not previously active, a new `Alert` is created in `StatePending`. Its `ActiveAt` timestamp is set.
            *   **Existing active alerts**: If a series was already active, its `Value` and `Annotations` are updated.
            *   **Resolved alerts**: If a previously active series is no longer present in the query result, its alert state might change from `StateFiring` or `StatePending` to `StateInactive`. Its `ResolvedAt` timestamp is set.
            *   **Hold Duration**: An alert transitions from `StatePending` to `StateFiring` only after its condition has been true for the `holdDuration` specified in the rule. `FiredAt` is set when it becomes firing.
            *   **Keep Firing For**: If `keep_firing_for` is set, an alert remains in `StateFiring` for this duration even after the condition is no longer met. `KeepFiringSince` tracks when the condition initially resolved.
        4.  Generates synthetic time series for active alerts:
            *   `ALERTS{alertname="<name>", alertstate="pending|firing", ...}`: Value is 1.
            *   `ALERTS_FOR_STATE{alertname="<name>", ...}`: Value is the Unix timestamp when the alert became active in its current state (or pending). This is used to restore `for` state after restarts.
    *   `sample()` and `forStateSample()`: Helper methods to create the synthetic series.
    *   `ActiveAlerts()`: Returns currently active (pending or firing) alerts for the rule.
    *   `sendAlerts()`: Called by the `Manager` to send notifications for alerts that need to be sent (e.g., newly firing, or re-sending based on `resendDelay`).

*   **`rules/recording.go`**:
    *   Defines `RecordingRule`.
    *   `NewRecordingRule()`: Constructs a `RecordingRule`.
    *   `Eval()`:
        1.  Executes the rule's PromQL query at the given timestamp.
        2.  For each resulting vector element (series):
            *   Overrides its metric name with the `record` name specified in the rule.
            *   Adds any labels defined in the rule to the series's label set.
        3.  The resulting vector of series (with new metric names and labels) is returned to the `Group`, which then appends it to the storage.
    *   Ensures that the evaluation does not produce multiple series with the exact same label set after modification, as this is an invalid state.

Rule evaluation is a critical part of Prometheus, allowing users to precompute expensive queries (recording rules) and define alert conditions (alerting rules). The `rules` package manages the complexity of loading, evaluating, and acting upon these rules.

### `notifier/`

The `notifier/` directory is responsible for handling alerts generated by alerting rules and sending them to configured Alertmanager instances.

*   **`notifier/manager.go`**:
    *   The `Manager` struct is the central component for managing notifications.
    *   `NewManager()`: Creates a new notification manager with configurable options (queue capacity, relabel configs, external labels).
    *   `ApplyConfig()`: Updates the manager's configuration, particularly the set of Alertmanager services to send notifications to. It parses `alertmanager_configs` from the main Prometheus configuration.
    *   `Run()`: Starts the main loops for the notifier:
        *   `targetUpdateLoop()`: Receives updates about Alertmanager target groups from the discovery manager.
        *   `sendLoop()`: Continuously consumes alerts from an internal queue and sends them to Alertmanagers.
    *   `Send()`: This method is called by the `rules.Manager` when alerts are produced by alerting rules. It applies alert relabeling (`alert_relabel_configs`) and adds external labels before queueing the alerts.
    *   **Queueing**: Alerts are placed in an internal queue (`n.queue`). If the queue is full, older alerts are dropped. The `MaxBatchSize` option controls how many alerts are sent in a single request to an Alertmanager.
    *   **Dispatching**: The `sendLoop()` picks up batches of alerts from the queue. The `sendAll()` method then attempts to send these alerts to all configured and discovered Alertmanager instances concurrently.
        *   It marshals alerts into the appropriate API format (V2 is default).
        *   For each configured Alertmanager set (a group of Alertmanagers that might be behind a load balancer or share the same configuration), it tries to send the alerts.
        *   If an Alertmanager set has its own `alert_relabel_configs`, those are applied before sending to that specific set.
        *   It tracks whether at least one Alertmanager in each set successfully received the alerts.
    *   `Stop()`: Stops the notification manager, optionally draining the queue of pending notifications.
    *   Maintains metrics related to notifications (e.g., sent, dropped, errors, latency).

*   **`notifier/alertmanager.go`**:
    *   Defines the `alertmanager` interface (internal) and the `alertmanagerSet` struct.
    *   `alertmanagerSet`: Represents a configured set of Alertmanager instances (e.g., those discovered via a single service discovery configuration or defined statically).
        *   It holds the `config.AlertmanagerConfig` and an `http.Client` for communication.
        *   `sync()`: Updates the list of active Alertmanager instances within the set based on target groups provided by service discovery. It applies Alertmanager-specific relabeling.
        *   `configHash()`: Computes a hash of the Alertmanager configuration, used by the `Manager` to identify and reuse Alertmanager sets across reloads if their configuration hasn't changed.
    *   `AlertmanagerFromGroup()`: A utility function to extract `alertmanager` instances from a `targetgroup.Group` based on an `AlertmanagerConfig`. It applies relabeling to discover the actual Alertmanager addresses and paths.
    *   Defines the API path for sending alerts (e.g., `/api/v2/alerts`).

*   **`notifier/alert.go`**:
    *   Defines the `Alert` struct, which is the internal representation of an alert used by the notifier. This struct is slightly different from `rules.Alert` and is tailored for sending to Alertmanager (e.g., includes `GeneratorURL`).
    *   `relabelAlerts()`: A helper function to apply relabeling rules to a slice of alerts. This is used for both global `alert_relabel_configs` and Alertmanager-specific ones.

**Workflow for Sending Alerts**:
1.  Alerting rules are evaluated by the `rules.Manager`.
2.  If an alerting rule fires, `rules.Manager` calls `notifier.Manager.Send()` with the generated `rules.Alert` objects.
3.  The `notifier.Manager` applies global `alert_relabel_configs` and adds `external_labels` to these alerts.
4.  The modified alerts are enqueued in an internal alert queue.
5.  The `sendLoop` in `notifier.Manager` dequeues a batch of alerts.
6.  For each configured Alertmanager group (`alertmanagerSet`):
    *   If the group has specific `alert_relabel_configs`, they are applied to a copy of the alert batch.
    *   The alert batch is marshaled into the JSON format expected by the Alertmanager API (typically V2).
    *   The marshaled payload is sent via HTTP POST to each active Alertmanager instance in that group.
7.  The manager tracks the success/failure of sending alerts to each Alertmanager instance and updates metrics accordingly.

The notifier ensures that alerts are reliably dispatched to Alertmanager instances, respecting configurations for relabeling, external labels, and service discovery of Alertmanagers themselves.

### `web/`

The `web/` directory contains the code for Prometheus's web interface, HTTP API, and related functionalities.

*   **`web/web.go`**:
    *   This is the main entry point for the web component. The `Handler` struct is central here.
    *   `New()`: Initializes the `Handler` with options including query engine, storage, scrape manager, rule manager, notifier, and configuration details.
    *   **Routing**: Sets up all HTTP routes using the `route.Router`. This includes:
        *   `/`: Redirects to the main UI page (e.g., `/graph` or `/classic/graph`, or `/agent` if in agent mode).
        *   `/static/*filepath`: Serves static assets for the UI.
        *   `/metrics`: Exposes Prometheus server's own metrics.
        *   `/federate`: Implements the federation endpoint.
        *   `/consoles/*filepath`: Serves console template files.
        *   `/api/v1/*`: Routes requests to the v1 API handler.
        *   Lifecycle endpoints (`/-/quit`, `/-/reload`, `/-/healthy`, `/-/ready`) if enabled.
        *   Debug endpoints (`/debug/*subpath`) for pprof.
    *   `Run()`: Starts the HTTP server(s) on the configured listen addresses.
    *   `ApplyConfig()`: Updates the web handler's view of the current Prometheus configuration.
    *   `consoles()`: Renders console templates by executing them with data (like path, params, external labels) and helper functions (like `query`).
    *   Manages UI assets:
        *   If `UseLocalAssets` is true, it serves assets from the local filesystem.
        *   Otherwise, it uses `web/ui/embed.go` (when `builtinassets` build tag is used) which embeds assets into the binary. `web/ui/ui.go` (without `builtinassets`) points to local assets for development.
    *   Handles UI redirection for different modes (server vs. agent) and UI versions (old React UI vs. new Mantine UI). The new UI is served from `/static/mantine-ui` and the old one from `/static/react-app`.
    *   `ready` state: The handler can be set to a "ready" or "not ready" state, affecting the `/-/ready` endpoint. This is used to signal if Prometheus is ready to serve traffic.
    *   `globalURLOptions`: Used to construct correct URLs when Prometheus is behind a reverse proxy or has a non-default external URL.

*   **`web/api/v1/api.go`**:
    *   Implements the HTTP API v1 for Prometheus.
    *   `NewAPI()`: Creates an API handler with dependencies like the query engine, storage, scrape manager, etc.
    *   **Endpoints**:
        *   `/api/v1/query`: Executes an instant PromQL query.
        *   `/api/v1/query_range`: Executes a range PromQL query.
        *   `/api/v1/query_exemplars`: Queries exemplars within a time range.
        *   `/api/v1/format_query`: Formats a PromQL query.
        *   `/api/v1/labels`: Returns a list of all label names.
        *   `/api/v1/label/<name>/values`: Returns a list of values for a given label name.
        *   `/api/v1/series`: Finds series by label matchers.
        *   `/api/v1/targets`: Returns information about scrape targets (active and dropped).
        *   `/api/v1/targets/metadata`: Returns metadata for metrics exposed by targets.
        *   `/api/v1/alertmanagers`: Returns an overview of the current Alertmanager discovery configuration.
        *   `/api/v1/alerts`: Returns a list of all active alerts.
        *   `/api/v1/rules`: Returns a list of all loaded rules (recording and alerting).
        *   `/api/v1/status/config`: Returns the current Prometheus configuration.
        *   `/api/v1/status/runtimeinfo`: Returns runtime information about the Prometheus server.
        *   `/api/v1/status/buildinfo`: Returns build information for the Prometheus server.
        *   `/api/v1/status/flags`: Returns the command-line flags Prometheus was started with.
        *   `/api/v1/status/tsdb`: Returns statistics about the TSDB.
        *   `/api/v1/status/walreplay`: Returns WAL replay status.
        *   `/api/v1/admin/tsdb/*`: Admin operations like `delete_series`, `clean_tombstones`, `snapshot` (if admin API is enabled).
        *   `/api/v1/read`: Remote read endpoint (used by other Prometheus servers or agents).
        *   `/api/v1/write`: Remote write endpoint (if `web.enable-remote-write-receiver` is true).
        *   `/api/v1/otlp/v1/metrics`: OTLP metrics ingestion endpoint (if `web.enable-otlp-receiver` is true).
    *   **Response Format**: Most endpoints return JSON. The API uses a common response wrapper (`api.Response`) with fields like `status` (`success` or `error`), `data`, `errorType`, `error`, and `warnings`.
    *   Handles query parameters like `time`, `start`, `end`, `step`, `timeout`, `match[]`, `limit`.
    *   Uses `parseTimeParam` and `parseDuration` for flexible time/duration parsing.
    *   Implements content negotiation for response encoding (JSON by default, supports other codecs if installed, e.g., Protobuf for remote read via `prompb`).
    *   Handles CORS requests.

*   **`web/ui/ui.go` (and `embed.go`)**:
    *   These files (along with the `web/ui/static/` or `web/ui/react-app`/`web/ui/mantine-ui` directories) manage the static assets for the Prometheus web UI.
    *   `ui.go` typically uses `http.Dir` to serve assets from the filesystem when Prometheus is built without embedded assets (for development).
    *   `embed.go` (generated when using the `builtinassets` tag) uses `embed.FS` to bundle UI assets directly into the Prometheus binary. This is the default for release builds.
    *   The UI itself is a React application (older version in `react-app`, newer in `mantine-ui`). The Go code here is primarily concerned with serving the `index.html` and its associated static files (JS, CSS, images).
    *   Placeholders in `index.html` (like `CONSOLES_LINK_PLACEHOLDER`, `TITLE_PLACEHOLDER`) are replaced by the Go server when serving the HTML, injecting dynamic configuration like the consoles path or page title.

The web package provides the primary way for users and other systems to interact with Prometheus, whether it's for querying data, viewing status, or integrating with remote systems.

### `tsdb/`

The `tsdb/` directory contains the core implementation of Prometheus's time series database. The `tsdb/README.md` provides links to more detailed documentation on its format and design.

*   **Core Concepts**:
    *   **Blocks**: Data is stored in persistent blocks on disk, typically spanning a 2-hour time window by default. Each block contains chunks of compressed samples for many series, an index, and tombstones for deleted data.
    *   **Head Block**: The most recent data is held in an in-memory "head" block, which also writes to a Write-Ahead Log (WAL) for durability against crashes.
    *   **Chunks**: Within a block (or the head), samples for a series are grouped into chunks. Chunks are individually encoded and compressed (e.g., XOR encoding for floats, delta encoding for histograms).
        *   `chunkenc/` subdirectory: Contains different chunk encoding implementations.
    *   **Index**: Each block has an index (`index/`) that maps label matchers to series and their chunks within that block. This allows for efficient querying.
    *   **WAL (Write-Ahead Log)**: All incoming samples are first written to the WAL. This ensures that data is not lost if the server crashes before the head block is persisted to disk. During startup, Prometheus replays the WAL to restore the in-memory head block.
    *   **Compaction**: Periodically, the head block is persisted to disk as a new block. Older blocks are compacted together into larger blocks to improve query efficiency and reduce storage overhead. Overlapping blocks can also be compacted.
    *   **Tombstones (`tombstones/`)**: Mark series or ranges of data within a series as deleted. Deletions are processed during compactions.
    *   **Out-of-Order (OOO) Samples**: TSDB supports ingesting samples that are out of chronological order, within a configurable time window. These are typically handled separately in the head block and merged during compaction.

*   **`tsdb/db.go`**:
    *   Defines the `DB` struct, which is the main entry point for interacting with the TSDB.
    *   `Open()`: Opens an existing database or creates a new one. It replays the WAL, loads existing blocks, and initializes the head block.
    *   `Appender()`: Returns an appender for ingesting new data into the head block.
    *   `Querier()` and `ChunkQuerier()`: Return queriers for accessing data from both the head block and persisted blocks.
    *   `Compact()`: Triggers a compaction cycle. This involves persisting the head block and potentially compacting older blocks.
    *   `Delete()`: Marks data for deletion (creates tombstones).
    *   `CleanTombstones()`: Rewrites blocks to physically remove data marked by tombstones.
    *   Manages the lifecycle of blocks, including loading them from disk, tracking their time ranges, and handling retention (deleting blocks older than the retention period or when storage size limits are hit).
    *   Handles WAL truncation and checkpointing.
    *   `reloadBlocks()`: Scans the DB directory for new blocks, reloads block metadata, and removes blocks that are obsolete due_to_compaction or retention.
    *   `Options`: Defines various configurable parameters for the TSDB, such as block duration, retention policies, WAL segment size, etc.

*   **`tsdb/head.go`**:
    *   Defines the `Head` struct, representing the in-memory head block.
    *   `NewHead()`: Creates a new head block, typically replaying data from the WAL and m-mapped head chunks during startup.
    *   `Init()`: Initializes the head, including WAL replay and loading of m-mapped chunks.
    *   `Appender()`: Returns a `headAppender` for writing data into the head.
        *   Incoming samples are added to `memSeries`.
        *   `memSeries` manage their chunks in memory. When a chunk is full or its time range is complete, it's cut and can be m-mapped to disk (in `chunks_head/`) to free up heap memory while still being queryable.
    *   `Querier()`/`Index()`/`Chunks()`: Provide access to the data within the head block for querying.
    *   `gc()`: Performs garbage collection on the head block, removing series that are no longer active and chunks that are too old.
    *   `truncateMemory()`: Removes data older than a certain timestamp from the in-memory part of the head.
    *   `truncateWAL()`: Truncates the WAL by creating checkpoints and removing old WAL segments.
    *   Manages out-of-order sample ingestion via `oooHeadChunk` and a separate WBL (Write-Behind Log) if OOO is enabled.
    *   `mmapHeadChunks()`: Handles writing active head chunks to disk as m-mappable files to reduce memory usage.
    *   `loadChunkSnapshot()` and `performChunkSnapshot()`: Handle saving and restoring in-memory chunk data on shutdown/startup to speed up restarts.

*   **`tsdb/querier.go`**:
    *   Implements querying logic for TSDB blocks and the head.
    *   `blockQuerier` and `blockChunkQuerier`: Implement `storage.Querier` and `storage.ChunkQuerier` for a single block.
    *   `selectSeriesSet()` and `selectChunkSeriesSet()`: Core functions that take label matchers and return a set of series or chunk series.
        *   Use the block's index (`IndexReader`) to find series matching the label selectors (via `PostingsForMatchers`).
        *   Use the `ChunkReader` to access the actual chunk data for these series.
        *   Apply tombstones to filter out deleted data.
        *   Handle time range filtering (mint, maxt) and potentially disable trimming of chunks for performance if `hints.DisableTrimming` is set.
    *   `PostingsForMatchers()`: Converts label matchers into a postings list (a list of series IDs) by querying the index.
    *   `DeletedIterator`: Wraps a chunk iterator to skip over samples that are marked as deleted by tombstones.
    *   The queriers for the head block are similar but operate on in-memory data structures (like `memSeries` and `MemPostings`).
    *   `NewHeadAndOOOQuerier` and `NewHeadAndOOOChunkQuerier`: Special queriers for the head that combine results from the in-order head data and the out-of-order head data.

The TSDB is optimized for write and query performance for time series data. Its on-disk format, indexing, and compaction strategies are designed to handle high ingestion rates and fast queries typical of monitoring workloads. The WAL ensures data durability, and the head block provides fast access to recent data.

## Data Flow and Key Workflows

This section describes the primary data flows within Prometheus.

**1. Metric Scraping and Ingestion:**

*   **Service Discovery (`discovery/`)**: The `discovery.Manager` continuously runs configured service discovery mechanisms (e.g., Kubernetes, EC2, file_sd) for each scrape job. It sends updates of `TargetGroup`s to the `scrape.Manager`.
*   **Target Relabeling (`scrape/target.go`)**: For each `TargetGroup`, the `scrape.Manager` (via `scrapePool.Sync`) applies `relabel_configs` to filter targets and modify their labels. This results in a list of final `Target` objects.
*   **Scraping (`scrape/scrape.go`)**: Each `Target` is managed by a `scrapeLoop`.
    *   The loop waits for the configured `scrape_interval`.
    *   It makes an HTTP GET request to the target's metrics endpoint (e.g., `/metrics`).
    *   The response body is parsed (e.g., Prometheus text format, OpenMetrics) by `textparse.Parser`.
    *   **Metric Relabeling (`metric_relabel_configs`)**: These rules are applied to the scraped samples, potentially filtering or modifying them.
    *   Valid samples, along with their timestamps (either from the target if `honor_timestamps` is true, or the scrape time), are passed to a `storage.Appender`.
*   **Storage Appending (`storage/`, `tsdb/`)**:
    *   The `scrapeLoop` gets an appender from the `scrape.Manager`'s `storage.Appendable` (which is typically a `storage.Fanout` instance).
    *   If `remote_write` is configured, the `FanoutAppender` forwards samples to `storage/remote/WriteStorage`, which queues and sends them to remote endpoints.
    *   Samples are also sent to the local `tsdb.DB`'s appender.
    *   The `tsdb.HeadAppender` writes samples to the in-memory head block and the Write-Ahead Log (WAL).
        *   Series are identified by their label sets. New series are created in the head if not already present.
        *   Samples are added to the appropriate `memSeries` and its current head `memChunk`.
        *   If a chunk is full, it's cut and may later be m-mapped to disk (`chunks_head/`).
    *   Metadata (TYPE, HELP, UNIT) is also processed and stored in the `scrapeCache` and potentially written to the WAL if `appendMetadataToWAL` is enabled.

**2. Rule Evaluation and Alerting:**

*   **Rule Loading (`rules/manager.go`)**: The `rules.Manager` loads rule files. Alerting and recording rules are parsed into `AlertingRule` and `RecordingRule` objects.
*   **Evaluation (`rules/group.go`, `rules/alerting.go`, `rules/recording.go`)**:
    *   Each `RuleGroup` runs its evaluation loop based on its configured interval.
    *   For each rule, its PromQL expression is evaluated by the `promql.Engine` at the current timestamp.
    *   **Recording Rules**: The resulting vector is modified (metric name and labels changed as per rule definition) and appended to storage via the `rules.Manager`'s `Appendable` (again, typically a `FanoutStorage` leading to local TSDB and remote write).
    *   **Alerting Rules**:
        *   The query result is evaluated. Each series in the result that meets the alert condition becomes an active alert instance.
        *   Alerts go through `pending` and `firing` states based on the `for` duration.
        *   Synthetic series (`ALERTS`, `ALERTS_FOR_STATE`) are generated for active alerts and appended to storage.
*   **Notification (`notifier/manager.go`)**:
    *   The `rules.Manager` calls `notifier.Manager.Send()` with the set of active/resolved alerts.
    *   The `notifier.Manager` applies `alert_relabel_configs` and adds `external_labels`.
    *   Alerts are queued and then dispatched to configured Alertmanager instances via HTTP. Service discovery for Alertmanagers is also handled.

**3. Query Processing:**

*   **API Request (`web/api/v1/api.go`)**: A query request (e.g., to `/api/v1/query` or `/api/v1/query_range`) is received.
*   **Query Creation (`promql/engine.go`)**: The `promql.Engine` creates an `InstantQuery` or `RangeQuery` object. This involves parsing the PromQL string into an AST.
*   **Storage Querier (`storage/`, `tsdb/`)**: The engine obtains a `storage.Querier` from the `storage.Queryable` (typically `FanoutStorage`).
    *   The `FanoutQuerier` gets a querier from the local `tsdb.DB` (primary) and any configured `remote_read` endpoints (secondaries).
    *   The `tsdb.DB.Querier()` creates queriers for relevant on-disk blocks and the head block. These are merged.
*   **Execution (`promql/engine.go`)**: The `evaluator` executes the query AST.
    *   **Selector Evaluation**: For `VectorSelector` or `MatrixSelector` nodes, the engine (via the `Querier`) fetches series data (postings lists from the index, then chunk data) from the relevant blocks and the head.
    *   Tombstones are applied to filter out deleted data.
    *   Data is decoded and processed.
    *   **Function/Operator Evaluation**: Functions and operators are applied as per the AST. For range vector functions, this involves iterating over time steps.
*   **Result Formatting (`web/api/v1/api.go`)**: The result (scalar, vector, matrix) is formatted (typically as JSON) and returned in the HTTP response.

**4. Compaction and Retention (`tsdb/db.go`, `tsdb/compact.go`):**

*   **Head Block Persistence**: Periodically (or when the WAL gets large enough), the `tsdb.Head` is compacted into a new persistent block on disk.
*   **Block Compaction**: The `Compactor` component runs in the background.
    *   It identifies blocks that can be compacted together (e.g., non-overlapping blocks within a time range, or overlapping blocks).
    *   It merges the data from these blocks, sorts it, removes redundant data, and writes a new, larger block.
    *   Old blocks are marked for deletion after successful compaction (their ULIDs are added to the `meta.json` of the new block as parents).
*   **Retention**: The `reloadBlocks()` method in `tsdb.DB` is responsible for deleting blocks that are:
    *   Older than the configured retention time.
    *   Exceeding the configured size-based retention limits.
    *   Marked as deletable because they have been compacted into a newer block.

These workflows illustrate how Prometheus components interact to provide its core functionalities of metrics collection, storage, rule processing, alerting, and querying.

## Contribution Guidelines

Prometheus welcomes contributions. Key points from `CONTRIBUTING.md`:

*   **Communication**:
    *   For trivial fixes, create a pull request and mention a maintainer.
    *   For more involved changes, discuss on the [Prometheus developers mailing list](https://groups.google.com/forum/?fromgroups#!forum/prometheus-developers) first.
    *   Check the [non-goals issue](https://github.com/prometheus/docs/issues/149) for areas not being pursued.
*   **Style**:
    *   Follow Go Code Review Comments and Peter Bourgon's "Go: Best Practices for Production Environments".
*   **Developer Certificate of Origin (DCO)**: Sign off on commits.
*   **Claiming Issues**: Comment on an issue to claim it before working on it. `low-hanging-fruit` issues are good for starters.
*   **Development Environment**:
    *   [Gitpod.io](https://gitpod.io/#https://github.com/prometheus/prometheus) can be used for a prebuilt dev environment.
    *   Build from source using Go (version 1.22+) and Node.js (version 22+).
    *   `go build ./cmd/prometheus/` for building.
    *   `make test` to run all tests.
    *   `make lint` to run linters (`golangci-lint`). Use `//nolint` sparingly.
*   **Pull Request Process**:
    *   Branch from `main` and rebase if necessary.
    *   Make small, correct commits.
    *   If review is slow, ping a reviewer or ask on IRC (`#prometheus-dev` on Libera.Chat).
    *   Add relevant tests for bugs or new features.
*   **Dependency Management**: Uses Go modules. Use `go get` to add/update dependencies and `go mod tidy`. Commit changes to `go.mod` and `go.sum`.
*   **PromQL Parser**: The parser grammar is in `promql/parser/generated_parser.y` and built with `make parser` (uses `goyacc`). Debug flags like `yyDebug` and `yyErrorVerbose` can be manually set in the generated Go file for debugging.

The contribution process emphasizes communication, code quality, testing, and adherence to Go best practices.
---
