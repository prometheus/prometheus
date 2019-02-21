---
title: Template examples
sort_rank: 4
---

# Template examples

Prometheus supports templating in the annotations and labels of alerts,
as well as in served console pages. Templates have the ability to run
queries against the local database, iterate over data, use conditionals,
format data, etc. The Prometheus templating language is based on the [Go
templating](https://golang.org/pkg/text/template/) system.

## Simple alert field templates

```
alert: InstanceDown
expr: up == 0
for: 5m
labels:
  severity: page
annotations:
  summary: "Instance {{$labels.instance}} down"
  description: "{{$labels.instance}} of job {{$labels.job}} has been down for more than 5 minutes."
```

Alert field templates will be executed during every rule iteration for each
alert that fires, so keep any queries and templates lightweight. If you have a
need for more complicated templates for alerts, it is recommended to link to a
console instead.

## Simple iteration

This displays a list of instances, and whether they are up:

```go
{{ range query "up" }}
  {{ .Labels.instance }} {{ .Value }}
{{ end }}
```

The special `.` variable contains the value of the current sample for each loop iteration.

## Display one value

```go
{{ with query "some_metric{instance='someinstance'}" }}
  {{ . | first | value | humanize }}
{{ end }}
```

Go and Go's templating language are both strongly typed, so one must check that
samples were returned to avoid an execution error. For example this could
happen if a scrape or rule evaluation has not run yet, or a host was down.

The included `prom_query_drilldown` template handles this, allows for
formatting of results, and linking to the [expression browser](https://prometheus.io/docs/visualization/browser/).

## Using console URL parameters

```go
{{ with printf "node_memory_MemTotal{job='node',instance='%s'}" .Params.instance | query }}
  {{ . | first | value | humanize1024}}B
{{ end }}
```

If accessed as `console.html?instance=hostname`, `.Params.instance` will evaluate to `hostname`.

## Advanced iteration

```html
<table>
{{ range printf "node_network_receive_bytes{job='node',instance='%s',device!='lo'}" .Params.instance | query | sortByLabel "device"}}
  <tr><th colspan=2>{{ .Labels.device }}</th></tr>
  <tr>
    <td>Received</td>
    <td>{{ with printf "rate(node_network_receive_bytes{job='node',instance='%s',device='%s'}[5m])" .Labels.instance .Labels.device | query }}{{ . | first | value | humanize }}B/s{{end}}</td>
  </tr>
  <tr>
    <td>Transmitted</td>
    <td>{{ with printf "rate(node_network_transmit_bytes{job='node',instance='%s',device='%s'}[5m])" .Labels.instance .Labels.device | query }}{{ . | first | value | humanize }}B/s{{end}}</td>
  </tr>{{ end }}
<table>
```

Here we iterate over all network devices and display the network traffic for each.

As the `range` action does not specify a variable, `.Params.instance` is not
available inside the loop as `.` is now the loop variable.

## Defining reusable templates

Prometheus supports defining templates that can be reused. This is particularly
powerful when combined with
[console library](template_reference.md#console-templates) support, allowing
sharing of templates across consoles.

```go
{{/* Define the template */}}
{{define "myTemplate"}}
  do something
{{end}}

{{/* Use the template */}}
{{template "myTemplate"}}
```

Templates are limited to one argument. The `args` function can be used to wrap multiple arguments.

```go
{{define "myMultiArgTemplate"}}
  First argument: {{.arg0}}
  Second argument: {{.arg1}}
{{end}}
{{template "myMultiArgTemplate" (args 1 2)}}
```
