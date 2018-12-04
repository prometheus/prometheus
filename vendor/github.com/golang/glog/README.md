# glog-gokit

This packages is a replacement for [glog](github.com/golang/glog)
in projects that use the [go-kit logger](https://godoc.org/github.com/go-kit/kit/log).

It is inspired by istio's glog package for zap:
https://github.com/istio/glog

## Usage

Override the official glog package with this one.
This simply replaces the code in `vendor/golang/glog` with the code of this package.

In your `Gopkg.toml`:
```toml
[[override]]
  name = "github.com/golang/glog"
  source = "github.com/kubermatic/glog-gokit"
```

In your `main.go`:
```go
// Import the package like it is original glog
import "github.com/golang/glog"


// Create go-kit logger in your main.go
logger := log.NewLogfmtLogger(log.NewSyncWriter(os.Stdout))
logger = log.With(logger, "ts", log.DefaultTimestampUTC)
logger = log.With(logger, "caller", log.DefaultCaller)
logger = level.NewFilter(logger, level.AllowAll())

// Overriding the default glog with our go-kit glog implementation.
// Thus we need to pass it our go-kit logger object.
glog.SetLogger(logger)
```

Setting the logger to the glog package **MUST** happen before using glog in any package.

## Function Levels

|     glog     | gokit |
| ------------ | ----- |
| Info         | Debug |
| InfoDepth    | Debug |
| Infof        | Debug |
| Infoln       | Debug |
| Warning      | Warn  |
| WarningDepth | Warn  |
| Warningf     | Warn  |
| Warningln    | Warn  |
| Error        | Error |
| ErrorDepth   | Error |
| Errorf       | Error |
| Errorln      | Error |
| Exit         | Error |
| ExitDepth    | Error |
| Exitf        | Error |
| Exitln       | Error |
| Fatal        | Error |
| FatalDepth   | Error |
| Fatalf       | Error |
| Fatalln      | Error |

This table is rather opinionated and build for use with the Kubernetes' [Go client](https://github.com/kubernetes/client-go).
