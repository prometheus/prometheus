## V2 API for labels and values discovery

* **Owners:**
  * Ismail Simsek [@itsmylife](https://github.com/itsmylife) ismail.simsek@grafana.com
  * Andrew Hall [@tcp13equals2](https://github.com/tcp13equals2) andrew.hall@grafana.com

* **Implementation Status:** Not implemented

* **Related Issues and PRs:**

* **Other docs or links:**
* [Grafana - Performance improvements for high cardinality metrics #100376](https://github.com/grafana/grafana/issues/100376)
* [Grafana - Autocomplete desired API flow](https://mermaid.live/edit#pako:eNrFVNtu00AQ_ZXVPlQgXN9ix4lFgrgJ-lBU1PICQdbGO0lW2LtmL2ncKP_OrlMLo5aiSpXwg6Udj8-ZOWd29rgUFHCOFfw0wEt4x8haknrBkX0aIjUrWUO4Rl8UyLvR10aLUtRNBRru-XpxhohCnw3IFr0h5Q_g1AWPmce3wz2dz4dAObpqG1BogcvGLPAxb5jg8i_OcvTh_RUKSMOCbRxUZAmVCihTpdiCfPVyKYN5I2HFdjMLc1KxmulZHJ4oIXWxbGcreWy5PWG8rAyFQgtNqpmW5rYVy3F6p7T9llQGVI6-deUVRpE1LLCHbo-aVeyGaCa4C_q-_93rSunAcxSloYc2RBW1kBbNkR2GcvzRpiV38uTociOukRYNikMkQZlKK_TCMlo01AF7SLkUB9oLdq-wH5l2unZ4510yYhzpDSAyyENUioaKa_5fpH8aUrFaKehY_07_CJsbkCVw_dvo0qqnHmnxA-Zy2Omhu8-SMHDuVoJQoM-HI_JJWIdcz53FXqdMr9q_blPxVPepMA_qij28lozi3HXv4RpkTdwR7x37AtuJq-3w2cIxhRWxLbu6DvY3uzi-ClH3f0ph1pv-YBpKdL-i-qAb3MuWl_3ZAuB8j3c4j7Kpn4zjcBJFozjM4lHm4RbncRj56SSaJOk4TUfhOMkOHr7pOEM_m0zjcJSMJ3EWjqKph4EyLeT5cUt2y9IWZfcYyLfCcI3zNE0OvwCMfMHW)
* [Grafana - Metrics Browser/Explore desired API flow](https://mermaid.live/edit#pako:eNqllm1v2zYQx7_KgS-KBlVs-UGILMwZ1u4BxZatW7Y3mweDlk4WUUlUSSqJYvi790hKgfNQN23zwokux__d_e581I6lMkOWMI0fWqxT_FHwreLVqgb6abgyIhUNrw38o1E9tr5uRZmhAq7hAo0SqR5Mj31_ePfW-v3ZourgNU_fY51Zo_f0nzbK6fl5r5HAHw3WGqpeeqPk9V0avQ95k0YCv_z0N4x5I8ZX03HvP86ETuUVqu-_26jxeSkqYZZR-ELUadlmuCY3nnHDl0a16EV_lwZBiW1hQObghP9C06qDJK6FKcB0DerAyba1MDoATsVsuBYpiDqXXo3Onx5Ws-s1EvhvV_MKE1ixwphmrSx9bfTaSMPLFfPKNoh1SWVbGyqbBWCDWdNwwNpGo9Heff7fH7MaCUymYRhAwfW6kop0bJH7--goNws8gctCXkMulDYQhXeVvrQS4OROjjXpEktMzUDoUzV9tmsl32D5sGlec_mE4ouhn59g7eUI9YqRSCEzy2rFtOGm1Ws79t5AU9hIUZse5X2I8SHBnJf6OMIY-BUXFLlE8PGfw815PkztebjGB2fGV7wkPt_Kzas4btR-z2gezv0fkbc8Hrboy0BNor5qH-2QkvsK2gFwvAJX93GKXKUFarB9rlYMXubt7W0H2tlPvnJZOI0lCT7A9cSC-NmFq7hxWdARqbrA_l63mm_Rc0plbbioUa0HBzTp6LNrwtVE7l7K98Aq343ZsCvslrgfYb3pDOonuzX_wm7NI3A8Tj2RGSjUbWmODvebUqTvbVN-kzyDC4pErZE1bUdhBC-HJfOV7ZF5rtEt8-ds9SOAiY3deJTety_QGm8e7M8wHLsdWhIDzE6eHvNezE_6r9gl8LZnZI_5-ybDnBNxcNXCKzgccXjlsm74VtTcCGK8pGmsaV7cHrIsiFV_ceUl3ghn7nl2LGBbJTKW2PoCmi5VcfvIdjbRFTMFVtQ7e-v0WdjltKdjdKv_K2U1nFSy3RbDQ9tQ3OFtYjDy1sjLrk6HZxJgyY7dsGQymY2m03C6mJydTeNodhYHrGMJPY_O4sUink_mYRjF0T5gty7khNo2DSezeE7_mi3ixTxgmAkj1YV_oXHvNZQUrXdUb-wNypJZvP8IvLvw7A)
* [Grafana - Metrics Drilldown desired API flow](https://mermaid.live/edit#pako:eNq1Vl2T2jYU_St39LCTTFgwLF7AU7aTdttMp036se1LQ4YRlsCaGsuRZIjL8N9zJdngEJNl28k-7GBb9-uce67ujsSScRIRzd8XPIv5vaArRdezDPAvp8qIWOQ0M_CX5urzt_dKpCmT2wyohtfcKBHr48vPz7_87Sd78veCqxK-o_E_PGP2pT_p_9tI13d3By8R_JrzTAM7xGJUJwtJFfMGb6ThoMQqMSCX0LD7RVIGRuZAYyM2HNY-QW91OIexMIMIXv3wJ_RoLnqbQa862WNCx3LD1bffLFTvLhVrYab9ILjSUpn5opw6x8KUVyKL04LxORpSRg2dGlVwHwidX39az65yH8HbGUmMyefKwq-NnhtpaDojHRdvRtZ8LVU5LzRdcYxnuMZv0O123_kT7ngE_UEQdCCheo7HeQQ2-P60TMzBQhvBQyK3gFXAWmrTis05Ih54ymOjq8NwJvkL8E3pgqen8Hqv0xafVx758Ah8jOyLjKaIfVsTuIB_cFMobBwfDKwpZ7AoXbCGA3jmgNjQFEPCUihtnp-lzjtzzGHv5lJkxlIyI9pQU-i51VOTP5NI1srZuEnYkqb6McbGdSFLqbwYRLaCo84eI81ZnyZ6KVe9hlXPI3UhaU25LJUfM-VZeL1rBy_2tEd2GAz9j7B-EwY37ToInwpqPwRfGdjKqh5o4un6yvaoQ7bj0Pky3i_zPBWukVKDZg3gplVRj4wt6xK2OC41Di_PtKMZloUyCVeu4q0wCfAPQhvbBVUs6kKzryBA_52aOHn7ru3ArqXM_VVjaobB0yV1VM-MiAwj4AXVyvvoHO2PTwW6oQJ_ptzKyvlE9C19rliLLUJe4fvlVhoBZUwYIXGq1N6Rv4qaRXmZSk99QBOUi9Vam1wq1f9N739Wucu7QDi059k950qyAtFoJXv4ZJEPQ6gB0fDME-JugjZxPm_ylErcHn7EectpnPjp4D-c8tCo7hXPuKLYd7jxIH92WreE6dQZTT_F4Gu5P0LqI-D3ZqEt0N3XWxa4WePC-cvHrlQHQNuH5cGfn5g_8xK5wP6wgkLc672pd-gVeFEp5ZpuqeJV89fzyd_ZLzCJFV7aVh8wxVRiBI0d10HtM3W3uUI9bezKWS02pENWSjAS2d2og6NFral9JDub-oygytc4XHC4E8aXtEid3PZohoX_LeW6tlSyWCX1Q5HjslcvzfVLWhj5UGZx_YwOSLQjH0h0G3aHwWQcjkfBzW3_ZjDukJJE16Og3x0Owttw0h9NbsejcN8h_7qQ_e7NKBj1g-Fk0L9B0-GgQzgOCKle-73dre-YFNLB1feyyAyJJvuPHWAAkw)
* [Grafana - Possible streaming API flow](https://mermaid.live/edit#pako:eNqNVU1v2zgQ_SsDXisnlpTEqQ4BEqsttkDT7qq5LHyZSGObXYlUScpJGuS_71CU6tj5qg-mSLwZvnnzwXtR6opEJiz97EiVlEtcGWwWCvjXonGylC0qB_NaEi9o4ZPBJSocTp4i88KjvhndkFtTZyFHh4XuTElPwR62B7_A8j9S1VPs9yK_8Nh-LZw2uGKPAXepHYHekBloRR6UebettlhDChP4oKqJ0xNe2NoQNlKt4Ea6NVzmn4uvl6Ov4GFydpYXGXz68B0OsZWHm-Swxmuq7WElbemvCmgsndwg354X4SD85wU78FFl8FGbGzQV2N-XGq-2dXsOPPqxC79nJyGSvzsydxAYwEYiSEcGWYQ9Jx79high2kEDrwuWa6ilIpAWpKqoZfl9soMoW0LefPI7rAt0bBc_4jpIFkhm8ENfR-zPOuTCikBhQ7bFsQpYIMYHYn9q0wdk5GrtQC9htP2GxlIfQMTKMnUDsmmokqxHffdWAMnLAbS6iqDUyiH7NsyGe-U18i_jnyd-3nqlwWmmbbva2Qi6tvJZvPrrLdrpy7QtmY306rHzVkvOO_je0tVr3F8xGiqsdnDeOV3qpq2JOU5g7pNUAy65FMF2yyV3qq-bIZpgt9NR461X1ltQTaWjCjCweIK_9VGdX2vj5iyq0XVN5gD9fgvlaG6DLvNacxmw_Iq9Sq22mF6k21D8PDfaoXd2MNteCbh_yPYjy8LSEFU-mXCD1tP1CnRuzz58sZQEX8gZWfIg62Tti3EythonelBvx3Q3vR-l4nl17ZO8F8Femg2t2E0EvzR37juo_LKscbWjzTOZftvu-WodghjzH3FF1BDH40wyVJLc0FBk_QAPnxXtDae9w-3Ye3SYFyISKyMrkTnTUSQaMg36rbj34IXgx6Khhcj4s6IlcsEtxEI9sBk_Fv9q3YyWRner9bgJ_TW8cuMhclkXd6oc9-xAZPfiVmSTeDo9SNLZ6TSezpJZcnRyHIk7Pk_ShM-PT2azeJam8TQ-eojEr_7W5GB6Eqez4_fp-9P49Oj0eBYJHkY8qr-Et7Z_cplXP6rmulOOjZL04X-xgmzc)

> TL;DR: This design doc proposes a richer API for searching and discovering metric names, labels, and values to power web applications.

## Why

The existing Prometheus [metadata API endpoints](https://github.com/prometheus/prometheus/blob/main/docs/querying/api.md#querying-metadata) are used by web applications (including Grafana) to query series metadata, such as label names and label values.

These endpoints are heavily relied upon by the Grafana web application for core discovery workflows, including:

* **Autocomplete** — triggered as users type queries
* **Metrics Explorer / Builder** — a click-based interface for browsing metrics and label dimensions
* **Metrics Drilldown** — dynamically generating metric panels by iterating over label values

As environments have grown in series count, label counts, cardinality, and exploratory user activity, and as the Grafana web application itself has grown in complexity, the existing Prometheus endpoints have increasingly become a limiting factor.

This manifests as performance bottlenecks in the Grafana UI and constrains the implementation of user-requested discovery features, particularly those that require iterative or interactive exploration of large metadata sets.

Grafana has attempted to mitigate these limitations through a range of client-side optimizations and workarounds (see [#100376](https://github.com/grafana/grafana/issues/100376)
). However, these approaches add complexity and cannot fully address the underlying scalability and efficiency constraints of the current metadata APIs.

### Pitfalls of the current solution

The current metadata APIs provide limited server-side filtering, ordering, and aggregation capabilities. As a result, client applications are often required to retrieve large, unfiltered datasets and then perform filtering, sorting, and relevance ranking locally.

In addition, clients must frequently combine data from multiple API endpoints to assemble the information needed for a single user interaction or view.

In practice, this leads to:
* unnecessarily **large response payloads**
* **increased backend load**
* a degraded user experience, as **clients must wait** for full responses before rendering meaningful results

For interactive discovery workflows, this significantly increases the time-to-first-result presented to the user and constrains the ability to build responsive, incremental interfaces.

Some specific examples highlighted by Grafana web application developers include;

* **Limited client-side filtering**: `match[]` only filters series on the server. For label-name autocomplete clients must download all label names and then filter locally, which is slow at scale.
* **Limited matching capabilities**: `match[]` supports regex matching of label values, but does not support fuzzy or typo-tolerant matching that discovery UIs require (e.g., user types container.memory and expects container_memory).
* **Unordered `limit` results**: `limit` returns an arbitrary subset of values rather than returning the most relevant or most frequent values, which causes drilldown UIs to display incomplete or unrepresentative results.
* **No metadata about further results**: responses do not indicate whether `limit` truncated the result set or how many total values exist, preventing the UI from making informed choices (show "more" affordance, refine query, etc.).
* **Fragmented calls for full context**: clients must call multiple endpoints (labels, metric metadata, series queries) to assemble a page of enriched metric information, increasing latency and backend load.
* **No streaming / incremental delivery**: the API returns the full result set only after query completion; clients cannot render partial results quickly for improved interactivity and low latency interactions.

## Goals

Effective data queries cannot be written if what needs to be queried cannot be identified first. Improving metric name, label name and label value discovery reduces Mean Time to Execute Query (MTEQ). This can be rephrased as "users spend less time hunting for the right metrics and more time analyzing data."

* **Faster metric name and label identification** - users spend less time hunting for the right metrics and can execute queries sooner, reducing the time from "I need to monitor X" to "I'm looking at data for X"
* **Better autocomplete performance** - latency-sensitive autocomplete becomes usable at scale by fetching only relevant results instead of downloading large (43MB+) datasets just to filter client-side
* **Scalable high-cardinality handling** - server-side filtering, streaming enables users to browse thousands of labels/values without timeouts, incomplete data, or memory exhaustion
* **Reduced bandwidth/resource consumption** - server-side filtering, streaming eliminates wasteful large responses, and enriched responses eliminate additional API calls reducing overall latency for the user
* **Improved AI/ML agent efficiency** - metadata-rich APIs and streaming capabilities enable AI agents (like Grafana Assistant) to make informed decisions, process data progressively, and stop early when confident, reducing redundant API calls and improving response times

### Audience

* Operators seeking to discover metrics, labels and values - operators seeking to create new queries, perform adhoc investigations, create new rule/alerts, cardinality exploration etc

## Non-Goals

* Deprecating / replacing any existing Prometheus API endpoints
* Addressing OpenTelemetry hidden label limitations. Resource attributes stored in `target_info` rather than on every series and therefore not surfaced in the existing labels' endpoint.
* Supporting pagination of API responses.
* Augmentation of frequency and cardinality on labels/values has been removed from this proposal. It is noted that the response format does allow for additional enrichments to be added at a future time.

## How

A new set of `search` endpoints are proposed.

* /api/v1/search/metric_names
* /api/v1/search/label_names
* /api/v1/search/label_values

Each endpoint allows for the specific searching, filtering, sorting of metric names, label names and label values respectively.

### Implementation notes

* the existing Prometheus API labels/values parameter set is re-used in each endpoint, with additional parameters introduced to deliver the desired functionality.
* the response format allows for enriched data to be added to each metric name, label name or label value record - rather than just a collection of strings.
* the response is NDJSON (`application/x-ndjson`), allowing for a streamed chunked encoding response.
* the existing Prometheus API response status, errors, warnings messages should be maintained. Note their placement in the response has been adapted for the streaming response format.

### `GET /api/v1/search/metric_names`

An endpoint specific to searching for metric names (\_\_name__ values) and obtaining an enriched record for each metric name.

#### Request

**Method:** `GET` or `POST`

`POST` is recommended when query parameters may exceed URL length limits.

**Path:** `/api/v1/search/metric_names`

**Query parameters:**

| Name               | Type                     | Required | Default     | Description                                                                  |
|--------------------|--------------------------|----------|-------------|------------------------------------------------------------------------------|
| `match[]`          | string / selectors       | No       |             | Series selector - as per existing labels/values endpoints.                   |
| `search[]`         | []string                 | No       |             | The search strings to be used for matching metric names.                     |
| `fuzz_threshold`   | int [0..100]             | No       | 0           | Set the fuzzy match threshold.                                               |
| `fuzz_alg`         | string                   | No       | jarowinkler | Select the fuzzy match algorithm.                                            |
| `case_sensitive`   | bool                     | No       | true        | Toggle case sensitivity in string matching.                                  |
| `sort_by`          | alpha / score            | No       |             | Request how matching metrics should be sorted in the response.               |
| `sort_dir`         | asc / dsc                | No       | asc         | Request the ordering of the sort. Only valid when `sort_by` is set.          |
| `include_metadata` | bool                     | No       | false       | Request metric metadata (units, type, description).                          |
| `include_score`    | bool                     | No       | false       | Request the fuzz search score to be returned for each result.                |
| `start`            | rfc3339 / unix_timestamp | No       |             | As per the existing labels/values endpoint.                                  |
| `end`              | rfc3339 / unix_timestamp | No       |             | As per the existing labels/values endpoint.                                  |
| `limit`            | int >= 0                 | No       | 100         | The maximum number of results to return after any ordering has been applied. |
| `batch_size`       | int > 0                  | No       | 100         | The desired number of records per batch.                                     |

**Notes:**

***search***

The given `search` values to be used to match metric names.

Multiple search values can be specified.

A match is found if the metric name contains one of these search values or if a fuzzy match between the metric name and one of these search terms meets the given `fuzz_threshold`.

If `search[]` is omitted or empty, all metric names are matched.

The `match[]` param can be used as an alternative to using the `search` or it can be used in conjunction with the `search`.

For example;
* `match[]={cluster=prod}&search[]=` - find me all metric names which have a `cluster=prod` label
* `match[]={cluster=prod}&search[]=cpu` - find me all metric names which contain `cpu` and which have a `cluster=prod` label
* `match[]={cluster=prod}&search[]=cpu&search[]=mem` - find me all metric names which contain `cpu` or `mem` and which have a `cluster=prod` label

***fuzz_threshold***

The fuzz matching score must meet or exceed this threshold to be considered a match.

A value of 0 (default) disables any fuzz matching.

A Jaro-Winkler returns a score between 0..1 which can be easily scaled to 0..100. mimir vs mimer = 0.953. A `fuzz_threshold` of 95 or below would allow this match.

***fuzz_alg***

Allow the client to select which fuzzy algorithm is used. Noting that the selection must be supported by the server.

The initial algorithms to be supported are `subsequence` or `jarowinkler`.

The default algorithm is `jarowinkler`.

It is proposed that the available fuzzy algorithms be exposed via the `/features` endpoint.

***sort_by***

* **alpha** - metric names are sorted by alphabetical order.
* **score** - metric names are sorted by the matching fuzz score. This can only be used with a `search[]` and `fuzz_threshold` being set.

Note that `sort_by` is optional, and it is valid for there to be no sorting requested. This allows the server to return / stream results back immediately without the need for any server-side buffering.

***batch_size***

The desired size of each batch of results sent in each response chunk.

***start/end***

It is proposed that these could have default values which align to a reasonable look-back period.

Ideally this would fall within the WAL. A default look-back of 1 hour is proposed if no start/end is specified.

In the scenario of start being set but no end, end should default to now (plus a small sane addition to compensate for clock drift and execution delay).

In the scenario of end being set but no start, the start should default to the 1 hour range (as above).

***include_***

The implementation of `include_*` should use `json:"omitempty"` to not serialize the enriched attributes if they have not been requested / initialised

#### Response

**Status codes:**

Use existing Prometheus API status codes.

**Body (JSON):**

* Content-Type: application/x-ndjson; charset=utf-8

##### Example of NDJSON batched result set - no include_* flags set

```ndjson
{
    "results": [
        { "name": "go_gc_duration_seconds" },
        { "name": "go_gc_duration_seconds_count" },
        { "name": "go_gc_duration_seconds_sum" }
    ]
}

{
    "results": [
        { "name": "go_gc_gogc_percent" },
        { "name": "go_gc_gomemlimit_bytes" }
    ]
}

{
    "status": "success",
    "has_more": false
}
```

##### Example of NDJSON batched result set - with include_* flags set

Note on the inclusion of metadata. Although the existing `/api/v1/metadata` endpoint accepts a `limit_per_metric` flag, the proposal here is to return a single metadata record.

The record included in this response would be the same as the first record returned in the current `/api/v1/metadata` endpoint (for a given metric).

```ndjson
{
    "results": [
        {
          "name": "activity_tracker_failed_total",
          "score": 0.76,
          "cardinality": 10,
          "type": "counter",
          "help": "How many times has activity tracker failed to insert new activity",
          "unit": ""
        },
        {
          "name": "activity_tracker_free_slots",
           "score": 0.50,
          "cardinality": 50,
          "type": "gauge",
          "help": "Number of free slots in activity file.",
          "unit": ""
        }
    ]
}

{
    "results": [
        ...
    ]
}

{
    "status": "success",
    "has_more": false
}
```

##### Example no results matching

```ndjson
{
    "results": [ ],
}

{
    "status": "success",
    "has_more": false
}
```

##### Example API error

As per existing endpoints.

```ndjson
{
    "status": "error",
    "errorType": "bad_data",
    "error": "1:4: parse error: unexpected \"(\"",
}
```

##### Example of has_more

Note - The `has_more` flag could replace the need for the existing Prometheus `results truncated due to limit` warning;

```ndjson
{
    "results": [
        {
          "name": "activity_tracker_failed_total",
        },
        {
          "name": "activity_tracker_free_slots",
        }
    ]
}

{
    "results": [
        ...
    ]
}

{
    "status": "success",
    "has_more": true
}
```

##### Example of warnings

Warnings can be added to either to an individual batch or to the overall request.

As per the existing labels/values endpoints the `warnings` attribute is omitted in the response if there are none.

If informational messages are also required they could be added in a similar way to a batch or the end message.

```ndjson
{
    "results": [
        {
          "name": "activity_tracker_failed_total",
        },
        {
          "name": "activity_tracker_free_slots",
        }
    ],
    "warnings": [
      "some_warning_relevant_to_this_batch"
    ]
}

{
    "results": [
        ...
    ]
}

{
    "status": "success",
    "has_more": true,
    "warnings": [
      "some_other_warning_relevant_to_the_overall_request"
    ]
}
```

### `GET /api/v1/search/label_names`

An endpoint specific to searching for label names.

#### Request

**Method:** `GET` or `POST`

`POST` is recommended when `match[]` selectors may exceed URL length limits.

**Path:** `/api/v1/search/label_names`

**Query parameters:**

| Name             | Type                     | Required | Default     | Description                                                     |
|------------------|--------------------------|----------|-------------|-----------------------------------------------------------------|
| `match[]`        | string / selectors       | No       |             | Series selector - as per existing labels/values endpoints.      |
| `search[]`       | []string                 | No       |             | The search strings to be used for matching against label names. |
| `fuzz_threshold` | int [0..100]             | No       | 0           | As per above endpoint.                                          |
| `fuzz_alg`       | string                   | No       | jarowinkler | Select the fuzzy match algorithm.                               |
| `case_sensitive` | bool                     | No       | true        | As per above endpoint.                                          |
| `sort_by`        | alpha / score            | No       |             | As per above endpoint.                                          |
| `sort_dir`       | asc / dsc                | No       | asc         | As per above endpoint.                                          |
| `include_score`  | bool                     | No       | false       | Request the fuzz search score to be returned for each result.   |
| `start`          | rfc3339 / unix_timestamp | No       |             | As per above endpoint.                                          |
| `end`            | rfc3339 / unix_timestamp | No       |             | As per above endpoint.                                          |
| `limit`          | int >= 0                 | No       | 100         | As per above endpoint.                                          |
| `batch_size`     | int = 0                  | No       | 100         | As per above endpoint.                                          |

#### Response

**Status codes:**

Use existing Prometheus API status codes.

**Body (JSON):**

* Content-Type: application/x-ndjson; charset=utf-8

##### Example of NDJSON batched result set - no include_* flags set

```ndjson
{
    "results": [
        { "name": "cluster" },
        { "name": "container" },
        { "name": "instance" }
    ]
}

{
    "results": [
        { "name": "job" },
        { "name": "namespace" }
    ]
}

{
    "status": "success",
    "has_more": false
}
```

### `GET /api/v1/search/label_values`

An endpoint specific to searching for label values.

#### Request

**Method:** `GET` or `POST`

`POST` is recommended when `match[]` selectors may exceed URL length limits.

**Path:** `/api/v1/search/label_values`

**Query parameters:**

| Name             | Type                     | Required | Default     | Description                                                      |
|------------------|--------------------------|----------|-------------|------------------------------------------------------------------|
| `match[]`        | string / selectors       | No       |             | Series selector - as per existing labels/values endpoints.       |
| `label`          | string                   | Yes      |             | The label the user is requesting values for.                     |
| `search[]`       | []string                 | No       |             | The search strings to be used for matching against label values. |
| `fuzz_threshold` | int [0..100]             | No       | 0           | As per above endpoint.                                           |
| `fuzz_alg`       | string                   | No       | jarowinkler | Select the fuzzy match algorithm.                                |
| `case_sensitive` | bool                     | No       | true        | As per above endpoint.                                           |
| `sort_by`        | alpha / score            | No       |             | As per above endpoint.                                           |
| `sort_dir`       | asc / dsc                | No       | asc         | As per above endpoint.                                           |
| `include_score`  | bool                     | No       | false       | Request the fuzz search score to be returned for each result.    |
| `start`          | rfc3339 / unix_timestamp | No       |             | As per above endpoint.                                           |
| `end`            | rfc3339 / unix_timestamp | No       |             | As per above endpoint.                                           |
| `limit`          | int >= 0                 | No       | 100         | As per above endpoint.                                           |
| `batch_size`     | int > 0                  | No       | 100         | As per above endpoint.                                           |

**Notes:**
- The `label` parameter has been deliberately added as a required `query` parameter to avoid issues with the existing [values](https://github.com/prometheus/prometheus/blob/main/docs/querying/api.md#querying-label-values) endpoint which requires the label to be included as a path parameter.

#### Response

**Status codes:**

Use existing Prometheus API status codes.

**Body (JSON):**

* Content-Type: application/x-ndjson; charset=utf-8

##### Example of NDJSON batched result set - no include_* flags set

```ndjson
{
    "results": [
        { "name": "cluster1" },
        { "name": "cluster2" },
        { "name": "cluster3" }
    ]
}

{
    "results": [
        { "name": "cluster4" },
        { "name": "cluster5" }
    ]
}

{
    "status": "success",
    "has_more": false
}
```

### Interface changes

The following presents a proposal for storage querier interface changes to support this API implementation.

The existing Labels/Values API leverages the [storage.LabelQuerier](https://github.com/prometheus/prometheus/blob/main/storage/interface.go#L175-L189) interface.

Additional parameters are passed to the search functions using the [storage.LabelHints](https://github.com/prometheus/prometheus/blob/main/storage/interface.go#L254-L257) structure.

```go
// Filter determines whether a value should be included in results.
// Returns (accepted, score) where score is used for relevance ranking.
// Score should be in range [0.0, 1.0] where 1.0 is perfect match.
type Filter interface {
	Accept(value string) (accepted bool, score float64)
}

// Comparator defines ordering for search results.
// This allows sorting by value, score, or any combination.
type Comparator interface {
	Compare(a, b SearchResult) int
}

// SearchHints configures search operations with filtering and scoring.
// Unlike LabelHints, SearchHints is specifically designed for search APIs
// that need relevance scoring and ranking.
type SearchHints struct {
	// Filter determines which values to include and their relevance scores.
	// A nil Filter accepts all values with score 1.0.
	Filter Filter

	// Limit is the maximum number of results to return.
	// Use 0 to disable limiting.
	Limit int

	// CompareFunc is used for ordering results.
	// It receives full SearchResult values, allowing comparison by value,
	// score, or any combination.
	// A nil value means alphabetical ordering by value.
	CompareFunc Comparator
}

// SearchResult represents a single search result with its relevance score.
type SearchResult struct {
	// Value is the label name or label value.
	Value string

	// Score represents relevance, with 1.0 being a perfect match.
	// Score range is [0.0, 1.0].
	Score float64
}

// SearcherValueSet is an iterator returned from the Searcher label/value search functions.
type SearcherValueSet interface {
	Next() bool
	At() SearchResult
	Warnings() annotations.Annotations
	Err() error
	Close()
}

// Searcher provides search capabilities with relevance scoring.
// This interface is designed for autocomplete and search UIs that need
// to rank results by relevance rather than just filter them.
type Searcher interface {
	// SearchLabelNames returns label names matching the search criteria.
	// Results include relevance scores based on the Filter.
	SearchLabelNames(ctx context.Context, hints *SearchHints, matchers ...*labels.Matcher) (SearcherValueSet, error)

	// SearchLabelValues returns label values for the given label name.
	// Results include relevance scores based on the Filter.
	SearchLabelValues(ctx context.Context, name string, hints *SearchHints, matchers ...*labels.Matcher) (SearcherValueSet, error)
}
```

### Extensibility for Mimir, Thanos, Cortex

The introduction of returning collections of records rather than collections of strings allows for different implementations to provide additional record decorations.

This would allow each implementation to provide a custom set of additional request parameters which results in additional data in the response.

To avoid conflicts, the response record format could be extended to include an `extensions` map which returns the custom fields. The `extensions` would be omitted completely in the pure Prometheus implementation.

```json
{
  "results": [
    {
      "name": "cluster1",
      "frequency": 1003,
      "extensions": {
        "mimir": {
          "some_mimir_specific_attribute": 10,
          "another_mimir_specific_attribute" : 5,
          ...
        }
      }
    },
    {
      "name": "cluster2",
      "frequency": 4,
      "extensions": {
        ...
      }
    }
  ]
}
```

The request to enable the extension could be implicit in the requested params. For instance the params could be `mimir.include_active_series=true`.

To be more standards compliant, the extension could be requested in via the `Accept` header. ie `Accept: application/x-ndjson; charset=utf-8; extensions=mimir`. This would also be reflected in the returned `Content-Type`.

If required for client side applications, a simple endpoint which returns the supported extensions and their data model could be exposed. Noting that the existing Prometheus `/api/v1/features` could also be used.

*GET /api/v1/search/extensions*

```json
{
  "mimir": {
    "some_mimir_specific_attribute": {
      "type": "int",
      "help": "some string explaining the field"
    },
    ...
  }
}
```

If required, these extensions could also be versioned.

### Testing and verification

It should be possible to validate these new endpoints via comparing to the existing endpoints.

Queries to existing endpoints can be constructed to extract a full set of labels and values and this data can be client side processed and compared to the equivalent new endpoint responses.

### Migration

These are new endpoints and do not change or alter any existing functionality. No migration is required.

### Known unknowns

* confirm feasibility of including frequency and cardinality in these API responses - confirmed as not feasible for this initial version
* confirm requirement of supporting cursor based pagination for these new endpoints - confirmed as not required
* confirm any performance / response time constraints for these new endpoints
* specific choice of fuzzy search algorithm - confirmed
* specific implementation of the search result ordering for auto-complete scenarios - confirmed

## Alternatives

### 1. Can we not just use the existing Prometheus endpoints?

For large scale environments even with client-side optimizations;

* High-cardinality scenarios remain challenging (43MB responses, timeouts)
* OpenTelemetry adoption increases frequency of these scenarios
* Frontend complexity grows (caching, throttling, workarounds)
* User experience compromises persist (limited search, arbitrary limits)

### 2. Can we extend/enhance the existing Prometheus endpoints?

The existing API design and response format is constrained.

The existing response format returns collections of strings which does not support additional record enrichment.

Although the existing endpoints could be adapted to (optionally) return streaming/batched results, apply filtering and sorting etc - these would all be significant internal functional changes to these endpoints.

### 3. Why do we need a 3rd endpoint specific to metric names - it's just a special case of \_*name*_?

Separating this endpoint allows for the additional enrichment options to be included in both the parameters and response.

Although these could be applied to the values endpoint, it would result in the values endpoint have different behaviours (input parameters and response format) depending on the label name.

A dedicated endpoint also allows for future enrichments to be added which are only relevant to metric names.

## Action Plan

The tasks to do in order to migrate to the new idea.

* [X] Confirm requirement for supporting pagination or not - not required
* [ ] Finalize proposal based on community feedback
* [ ] Create Prometheus implementation issue for `/api/v1/search/metric_names`
* [ ] Create Prometheus implementation issue for `/api/v1/search/label_names`
* [ ] Create Prometheus implementation issue for `/api/v1/search/label_values`
* [ ] Modify existing Prometheus `/features` endpoint to expose supported fuzzy algorithm choices
* [ ] Implement endpoints in Prometheus
* [ ] Integrate new endpoints into Prometheus UI
* [ ] Update Prometheus documentation
