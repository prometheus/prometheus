We use re2c as lexer generator for parse prometheus text metrics format.

## Installation

```bash
apt install re2c
```

## Prometheus lexer code generation

```bash
re2c prometheus/textparse/prometheus/tokenizer.cxx -o prometheus/textparse/prometheus/tokenizer.cpp --no-debug-info --conditions --bit-vectors --no-generation-date --no-version
```

## Open metrics Lexer code generation

```bash
re2c prometheus/textparse/open_metrics/tokenizer.cxx -o prometheus/textparse/open_metrics/tokenizer.cpp --no-debug-info --conditions --bit-vectors --no-generation-date --no-version
```

## Links

[re2c home](https://re2c.org/)

[re2c manual C](https://re2c.org/manual/manual_c.html)

## Benchmark

We have tokenized a 82KB prometheus metrics text file using several lexer generators.

| Generator | Speed (microseconds) |
|-----------|----------------------|
| flex      | ~551                 |
| re2c      | ~76                  |
