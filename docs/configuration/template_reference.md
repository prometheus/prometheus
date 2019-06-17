---
title: Template reference
sort_rank: 5
---

# Template reference

Prometheus supports templating in the annotations and labels of alerts,
as well as in served console pages. Templates have the ability to run
queries against the local database, iterate over data, use conditionals,
format data, etc. The Prometheus templating language is based on the [Go
templating](https://golang.org/pkg/text/template/) system.

## Data Structures

The primary data structure for dealing with time series data is the sample, defined as:

```go
type sample struct {
        Labels map[string]string
        Value  float64
}
```

The metric name of the sample is encoded in a special `__name__` label in the `Labels` map.

`[]sample` means a list of samples.

`interface{}` in Go is similar to a void pointer in C.

## Functions

In addition to the [default
functions](https://golang.org/pkg/text/template/#hdr-Functions) provided by Go
templating, Prometheus provides functions for easier processing of query
results in templates.

If functions are used in a pipeline, the pipeline value is passed as the last argument.

### Queries

| Name          | Arguments     | Returns  | Notes    |
| ------------- | ------------- | -------- | -------- |
| query         | query string  | []sample | Queries the database, does not support returning range vectors.  |
| first         | []sample      | sample   | Equivalent to `index a 0`  |
| label         | label, sample | string   | Equivalent to `index sample.Labels label`  |
| value         | sample        | float64  | Equivalent to `sample.Value`  |
| sortByLabel   | label, []samples | []sample | Sorts the samples by the given label. Is stable.  |

`first`, `label` and `value` are intended to make query results easily usable in pipelines.

### Numbers

| Name          | Arguments     | Returns |  Notes    |
| ------------- | --------------| --------| --------- |
| humanize      | number        | string  | Converts a number to a more readable format, using [metric prefixes](https://en.wikipedia.org/wiki/Metric_prefix).
| humanize1024  | number        | string  | Like `humanize`, but uses 1024 as the base rather than 1000. |
| humanizeDuration | number     | string  | Converts a duration in seconds to a more readable format. |
| humanizePercentage | number   | string  | Converts a ratio value to a fraction of 100. |
| humanizeTimestamp | number    | string  | Converts a Unix timestamp in seconds to a more readable format. |

Humanizing functions are intended to produce reasonable output for consumption
by humans, and are not guaranteed to return the same results between Prometheus
versions.

### Strings

| Name          | Arguments     | Returns |    Notes    |
| ------------- | ------------- | ------- | ----------- |
| title         | string        | string  | [strings.Title](https://golang.org/pkg/strings/#Title), capitalises first character of each word.|
| toUpper       | string        | string  | [strings.ToUpper](https://golang.org/pkg/strings/#ToUpper), converts all characters to upper case.|
| toLower       | string        | string  | [strings.ToLower](https://golang.org/pkg/strings/#ToLower), converts all characters to lower case.|
| match         | pattern, text | boolean | [regexp.MatchString](https://golang.org/pkg/regexp/#MatchString) Tests for a unanchored regexp match. |
| reReplaceAll  | pattern, replacement, text | string | [Regexp.ReplaceAllString](https://golang.org/pkg/regexp/#Regexp.ReplaceAllString) Regexp substitution, unanchored. |
| graphLink  | expr | string | Returns path to graph view in the [expression browser](https://prometheus.io/docs/visualization/browser/) for the expression. |
| tableLink  | expr | string | Returns path to tabular ("Console") view in the [expression browser](https://prometheus.io/docs/visualization/browser/) for the expression. |

### Others

| Name          | Arguments     | Returns |    Notes    |
| ------------- | ------------- | ------- | ----------- |
| args          | []interface{} | map[string]interface{} | This converts a list of objects to a map with keys arg0, arg1 etc. This is intended to allow multiple arguments to be passed to templates. |
| tmpl          | string, []interface{} | nothing  | Like the built-in `template`, but allows non-literals as the template name. Note that the result is assumed to be safe, and will not be auto-escaped. Only available in consoles. |
| safeHtml      | string        | string  | Marks string as HTML not requiring auto-escaping. |

## Template type differences

Each of the types of templates provide different information that can be used to
parameterize templates, and have a few other differences.

### Alert field templates

`.Value`, `.Labels`, and `ExternalLabels` contain the alert value, the alert
labels, and the globally configured external labels, respectively. They are
also exposed as the `$value`, `$labels`, and `$externalLabels` variables for
convenience.

### Console templates

Consoles are exposed on `/consoles/`, and sourced from the directory pointed to
by the `-web.console.templates` flag.

Console templates are rendered with
[html/template](https://golang.org/pkg/html/template/), which provides
auto-escaping. To bypass the auto-escaping use the `safe*` functions.,

URL parameters are available as a map in `.Params`. To access multiple URL
parameters by the same name, `.RawParams` is a map of the list values for each
parameter. The URL path is available in `.Path`, excluding the `/consoles/`
prefix. The globally configured external labels are available as
`.ExternalLabels`. There are also convenience variables for all four:
`$rawParams`, `$params`, `$path`, and `$externalLabels`.

Consoles also have access to all the templates defined with `{{define
"templateName"}}...{{end}}` found in `*.lib` files in the directory pointed to
by the `-web.console.libraries` flag. As this is a shared namespace, take care
to avoid clashes with other users. Template names beginning with `prom`,
`_prom`, and `__` are reserved for use by Prometheus, as are the functions
listed above.
