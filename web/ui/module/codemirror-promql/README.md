CodeMirror-promql
=================

This project provides a mode for [CodeMirror](https://codemirror.net/6/) that handles syntax highlighting, linting
and autocompletion for PromQL ([Prometheus Query Language](https://prometheus.io/docs/introduction/overview/)).

![preview](https://user-images.githubusercontent.com/4548045/95660829-d5e4b680-0b2a-11eb-9ecb-41dca6396273.gif)

## Installation

This mode is available as a npm package:

```bash
npm install --save @prometheus-io/codemirror-promql
```

**Note:** You will have to manually install different packages that are part
of [CodeMirror](https://codemirror.net/6/), as they are a peer dependency to this package. Here are the different
packages you need to install:

* **@codemirror/autocomplete**
* **@codemirror/language**
* **@codemirror/lint**
* **@codemirror/state**
* **@codemirror/view**
* **@lezer/common**

```bash
npm install --save @codemirror/autocomplete @codemirror/language @codemirror/lint @codemirror/state @codemirror/view @lezer/common
```

**Note 2**: that's the minimum required to install the lib. You would probably need to install as well the dependency
**@codemirror/basic-setup** to ease the setup of codeMirror itself:

```bash
npm install --save @codemirror/basic-setup
```

## Usage

As the setup of the PromQL language can a bit tricky in CMN, this lib provides a class `PromQLExtension`
which is here to help you to configure the different extensions we aim to provide.

### Default setup

If you want to enjoy about the different features provided without taking too much time to understand how to configure
them, then the easiest way is this one:

```typescript
import {PromQLExtension} from '@prometheus-io/codemirror-promql';
import {basicSetup} from '@codemirror/basic-setup';
import {EditorState} from '@codemirror/state';
import {EditorView} from '@codemirror/view';

const promQL = new PromQLExtension()
new EditorView({
    state: EditorState.create({
        extensions: [basicSetup, promQL.asExtension()],
    }),
    // eslint-disable-next-line @typescript-eslint/no-non-null-assertion
    // tslint:disable-next-line:no-non-null-assertion
    parent: document.getElementById('editor')!,
});
```

Using the default setup will activate:

* syntax highlighting
* an offline autocompletion that will suggest PromQL keywords such as functions / aggregations, depending on the
  context.
* an offline linter that will display PromQL syntax errors (which is closer to what Prometheus returns)

### Deactivate autocompletion - linter

In case you would like to deactivate the linter and/or the autocompletion it's simple as that:

```typescript
const promQL = new PromQLExtension().activateLinter(false).activateCompletion(false) // here the linter and the autocomplete are deactivated
```

### Linter

There is no particular configuration today for the linter. Feel free to file an issue if you have some use cases that
would require configurability.

### Autocompletion

The autocompletion feature provides multiple different parameters that can be used to adapt this lib to your
environment.

#### maxMetricsMetadata

`maxMetricsMetadata` is the maximum number of metrics in Prometheus for which metadata is fetched. If the number of
metrics exceeds this limit, no metric metadata is fetched at all.

By default, the limit is 10 000 metrics.

Use it cautiously. A high value of this limit can cause a crash of your browser due to too many data fetched.

```typescript
const promQL = new PromQLExtension().setComplete({maxMetricsMetadata: 10000})
```

#### Connect the autocompletion extension to a remote Prometheus server

Connecting the autocompletion extension to a remote Prometheus server will provide autocompletion of metric names, label
names, and label values.

##### Use the default Prometheus client

###### Prometheus URL

If you want to use the default Prometheus client provided by this lib, you have to provide the url used to contact the
Prometheus server.

Note: this is the only mandatory parameter in case you want to use the default Prometheus client. Without this
parameter, the rest of the config will be ignored, and the Prometheus client won't be initialized.

```typescript
const promQL = new PromQLExtension().setComplete({remote: {url: 'https://prometheus.land'}})
```

###### Override FetchFn

In case your Prometheus server is protected and requires a special HTTP client, you can override the function `fetchFn`
that is used to perform any required HTTP request.

```typescript
const promQL = new PromQLExtension().setComplete({remote: {fetchFn: myHTTPClient}})
```

###### Duration to use for looking back when retrieving metrics / labels

If you are a bit familiar with the Prometheus API, you are aware that you can pass a time interval that is used to tell
to Prometheus which period of time you are looking for when retrieving metadata (like metrics / labels).

In case you would like to provide your own duration, you can override the variable `lookbackInterval`. By default, the
value is `12 * 60 * 60 * 1000` (12h). The value must be defined in **milliseconds**.

```typescript
const promQL = new PromQLExtension().setComplete({remote: {lookbackInterval: 12 * 60 * 60 * 1000}})
```

###### Error Handling

You can set up your own error handler to catch any HTTP error that can occur when the PrometheusClient is contacting
Prometheus.

```typescript
const promQL = new PromQLExtension().setComplete({remote: {httpErrorHandler: (error: any) => console.error(error)}})
```

###### HTTP method used

By default, the Prometheus client will use the HTTP method `POST` when contacting Prometheus for the
endpoints `/api/v1/labels` and `/api/v1/series`.

You can change it to use the HTTP method `GET` if you prefer.

```typescript
const promQL = new PromQLExtension().setComplete({remote: {httpMethod: 'GET'}})
```

###### Override the API Prefix

The default Prometheus Client, when building the query to get data from Prometheus, is using an API prefix which is by
default `/api/v1`.

You can override this value like this:

```typescript
const promql = new PromQLExtension().setComplete({remote: {apiPrefix: '/my/api/prefix'}})
```

###### Cache

The default client has an embedded cache that is used to store the different metrics and labels retrieved from a remote
Prometheus server.

###### Max Age

The data are stored in the cache for a limited amount of time defined by the variable `maxAge` which is by default 5
minutes. The value must be defined in **milliseconds**.

```typescript
const promQL = new PromQLExtension().setComplete({remote: {cache: {maxAge: 5 * 60 * 1000}}})
```

###### Initial Metric List

The cache can be initialized with a list of metric names. It is useful when you already have the list of the metrics
somewhere else in your application, and you would like to share this list with the embedded Prometheus client
of `codemirror-promql`.

Note: keep in mind that this list will be kept into the embedded Prometheus client until the time defined by `maxAge`.

```typescript
const promQL = new PromQLExtension().setComplete({
    remote: {
        cache: {
            initialMetricList: [
                'ALERTS',
                'ALERTS_FOR_STATE',
                'alertmanager_alerts',
                'alertmanager_alerts_invalid_total',
                'alertmanager_alerts_received_total',
            ]
        }
    }
})
```

##### Override the default Prometheus client

In case you are not satisfied by our default Prometheus client, you can still provide your own. It has to implement the
interface [PrometheusClient](https://github.com/prometheus/codemirror-promql/blob/main/src/client/prometheus.ts#L24-L39)
.

```typescript
const promQL = new PromQLExtension().setComplete({remote: MyPrometheusClient})
```

#### Provide your own implementation of the autocompletion

In case you would like to provide you own implementation of the autocompletion, you can simply do it like that:

```typescript
const promQL = new PromQLExtension().setComplete({completeStrategy: myCustomImpl})
```

Note: In case this parameter is provided, then the rest of the configuration is ignored.

### Example

* [ReactJS example](https://github.com/prometheus/prometheus/blob/431ea75a11ca165dad9dd5d629b3cf975f4c186b/web/ui/react-app/src/pages/graph/CMExpressionInput.tsx)
* [Angular example](https://github.com/perses/perses/blob/28b3bdac88b0ed7a4602f9c91106442eafcb6c34/internal/api/front/perses/src/app/project/prometheusrule/promql-editor/promql-editor.component.ts)

## License

The code is licensed under an [Apache 2.0](https://github.com/prometheus/prometheus/blob/main/LICENSE) license.
