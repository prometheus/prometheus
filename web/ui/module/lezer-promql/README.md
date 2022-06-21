# lezer-promql

## Overview

This is a PromQL grammar for the [lezer](https://lezer.codemirror.net/) parser system. It is inspired by the initial
grammar coming from [Prometheus](https://github.com/prometheus/prometheus/blob/main/promql/parser/generated_parser.y)
written in yacc.

This library is stable but doesn't provide any guideline of how to use it as it has been integrated
into [codemirror-promql](https://github.com/prometheus/prometheus/blob/main/web/ui/module/codemirror-promql). If you
want to use this library, you perhaps want to actually use **@prometheus-io/codemirror-promql** instead.

**Note**: This library is following the changes of the upstream grammar (i.e. coming from Prometheus) as closely as
possible

## Installation

This package is available as an npm package:

```bash
npm install --save @prometheus-io/lezer-promql
```

**Note**: you will have to manually install the `lezer` dependencies as it is a peer dependency to this package.

```bash
npm install --save @lezer/lr @lezer/highlight
```

## Development

### Building

    npm i
    npm run build

### Testing

    npm run test

## License

The code is licensed under an [Apache 2.0](https://github.com/prometheus/prometheus/blob/main/LICENSE) license.
