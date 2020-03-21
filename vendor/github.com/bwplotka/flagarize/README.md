# flagarize

[![CI](https://github.com/bwplotka/flagarize/workflows/test/badge.svg)](https://github.com/bwplotka/flagarize/actions?query=workflow%3Atest)

Flagarize your Go struct to initialize your even complex CLI config struct from flags!

## Goals

* Allow flag parsing for any struct field using Go struct tags.
* Minimal dependencies: Only `"gopkg.in/alecthomas/kingpin.v2"`.
* Extensible with [custom types](#custom-type-parsing) and [custom flagarizing](#custom-flags).
* Native supports for all [kingpin](https://github.com/alecthomas/kingpin) flag types and more like [`regexp`](./regexp.go) , [`pathorcontent`](./pathorcontent.go), [`timeorduration`](./timeorduration.go).

## Requirements:

* Go 1.3+
* `gopkg.in/alecthomas/kingpin.v2`


## Usage

For each field of the configuration struct that you want to register as a flag, add ``flagarize:"<key=value,>"`` struct tag.

### Flagarize tags

Flagarize struct tag expected value to be map where key=values are separated by `|` (can be configured via `WithElemSep` function.)

**Available keys:**

* `name`: Name of the flag. If empty field name will be used and parsed to different case (e.g `FooBar` field will be `foo-bar`)
* `help`: Usage description for the flag. If empty, value from string `<FieldName>FlagarizeHelp` field in the same struct will be used.
* `hidden`: Optional. if `true` flag will be hidden.
* `required`: Optional. if `true` flag will be required.
* `default`: Optional. Value will be used as a value if the flag is not specified. Otherwise default value for type will be used.
* `envvar`: Optional. Name of environment variable if needed next to the flag.
* `short`: Optional. Short single character for a flag name alternative.
* `placeholder` Optional. Flag placeholder for expected type.

Short tag example:

```go
type config struct {
   Field1 string `flagarize:"name=case9|help=help|hidden=true|required=true|default=some|envvar=LOL|short=z|placeholder=<something2>"`
}
```

### Supported types

Without extensions flagarize supports all kingpin supported types plus few more. For current supported types it's best to
see `TestFlagarize_OK` unit test [here](flagarize_ext_test.go).

### Example

See below example for usage:

```go
type ComponentAOptions struct {
	Field1 []string `flagarize:"name=a.flag1|help=..."`
}

func ExampleFlagarize() {
	// Create new kingpin app as usual.
	a := kingpin.New(filepath.Base(os.Args[0]), "<Your CLI description>")

	// Define you own config.
	type ConfigForCLI struct {
		Field1 string                   `flagarize:"name=flag1|help=...|default=something"`
		Field2 *url.URL                 `flagarize:"name=flag2|help=...|placeholder=<URL>"`
		Field3 int                      `flagarize:"name=flag3|help=...|default=2144"`
		Field4 flagarize.TimeOrDuration `flagarize:"name=flag4|help=...|default=1m|placeholder=<time or duration>"`

		NotFromFlags int

		ComponentA ComponentAOptions
	}

	// Create new config.
	cfg := &ConfigForCLI{}

	// Flagarize it! (Register flags from config).
	if err := flagarize.Flagarize(a, &cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	// You can define some fields as usual as well.
	var notInConfigField time.Duration
	a.Flag("some-field10", "...").
		DurationVar(&notInConfigField)

	// Parse flags as usual.
	if _, err := a.Parse(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	// Config is filled with flags from value!
	_ = cfg.Field1
}
```

## Production Examples

To see production example see:

 * [Thanos](todo)
 * [Prometheus](https://github.com/prometheus/prometheus/pull/7026)

## But Bartek, such Go projects already exist!

Yes, but not as simple, not focused on kingpin, and they does not allow custom flagarazing!  🤗

## But Bartek, normal flag registration is enough, don't overengineer!

Well, depends. It might get quite weird. Here is how it could look with and without flagarize:

### Without

```go
cfg := struct {
    configFile string

    localStoragePath    string
    notifier            notifier.Options
    notifierTimeout     model.Duration
    forGracePeriod      model.Duration
    outageTolerance     model.Duration
    resendDelay         model.Duration
    web                 web.Options
    tsdb                tsdbOptions
    lookbackDelta       model.Duration
    webTimeout          model.Duration
    queryTimeout        model.Duration
    queryConcurrency    int
    queryMaxSamples     int
    RemoteFlushDeadline model.Duration

    prometheusURL   string
    corsRegexString string

    promlogConfig promlog.Config
}{}

a := kingpin.New(filepath.Base(os.Args[0]), "The Prometheus monitoring server")
a.Version(version.Print("prometheus"))
a.HelpFlag.Short('h')

a.Flag("config.file", "Prometheus configuration file path.").
    Default("prometheus.yml").StringVar(&cfg.configFile)

a.Flag("web.listen-address", "Address to listen on for UI, API, and telemetry.").
    Default("0.0.0.0:9090").StringVar(&cfg.web.ListenAddress)

a.Flag("web.read-timeout",
    "Maximum duration before timing out read of the request, and closing idle connections.").
    Default("5m").SetValue(&cfg.webTimeout)

a.Flag("web.max-connections", "Maximum number of simultaneous connections.").
    Default("512").IntVar(&cfg.web.MaxConnections)

a.Flag("web.external-url",
    "The URL under which Prometheus is externally reachable (for example, if Prometheus is served via a reverse proxy). Used for generating relative and absolute links back to Prometheus itself. If the URL has a path portion, it will be used to prefix all HTTP endpoints served by Prometheus. If omitted, relevant URL components will be derived automatically.").
    PlaceHolder("<URL>").StringVar(&cfg.prometheusURL)

a.Flag("web.route-prefix",
    "Prefix for the internal routes of web endpoints. Defaults to path of --web.external-url.").
    PlaceHolder("<path>").StringVar(&cfg.web.RoutePrefix)

a.Flag("web.user-assets", "Path to static asset directory, available at /user.").
    PlaceHolder("<path>").StringVar(&cfg.web.UserAssetsPath)

a.Flag("web.enable-lifecycle", "Enable shutdown and reload via HTTP request.").
    Default("false").BoolVar(&cfg.web.EnableLifecycle)

a.Flag("web.enable-admin-api", "Enable API endpoints for admin control actions.").
    Default("false").BoolVar(&cfg.web.EnableAdminAPI)

a.Flag("web.console.templates", "Path to the console template directory, available at /consoles.").
    Default("consoles").StringVar(&cfg.web.ConsoleTemplatesPath)

//... Thousands more of that.

if _, err := a.Parse(os.Args[1:]); err != nil {
    fmt.Fprintln(os.Stderr, errors.Wrapf(err, "Error parsing commandline arguments"))
    a.Usage(os.Args[1:])
    os.Exit(2)
}
```

### With flagarize

```go
cfg := struct {
    ConfigFile           string         `flagarize:"name=config.file|help=Prometheus configuration file path.|default=prometheus.yml"`
    ExternalURL          string         `flagarize:"name=web.external-url|help=The URL under which Prometheus is externally reachable (for example, if Prometheus is served via a reverse proxy). Used for generating relative and absolute links back to Prometheus itself. If the URL has a path portion, it will be used to prefix all HTTP endpoints served by Prometheus. If omitted, relevant URL components will be derived automatically.|placeholder=<URL>"`
    StoragePath          string         `flagarize:"name=storage.tsdb.path|help=Base path for metrics storage.|default=data/"`
    RemoteFlushDeadline  model.Duration `flagarize:"name=storage.remote.flush-deadline|help=How long to wait flushing sample on shutdown or config reload.|default=1m|placeholder=<duration>"`
    RulesOutageTolerance model.Duration `flagarize:"name=rules.alert.for-outage-tolerance|help=Max time to tolerate prometheus outage for restoring \"for\" state of alert.|default=1h"`
    RulesForGracePeriod  model.Duration `flagarize:"name=rules.alert.for-grace-period|help=Minimum duration between alert and restored \"for\" state. This is maintained only for alerts with configured \"for\" time greater than grace period.|default=10m"`
    RulesResendDelay     model.Duration `flagarize:"name=rules.alert.resend-delay|help=Minimum amount of time to wait before resending an alert to Alertmanager.|default=1m"`
    LookbackDelta        model.Duration `flagarize:"name=query.lookback-delta|help=The maximum lookback duration for retrieving metrics during expression evaluations and federation.|default=5m"`
    QueryTimeout         model.Duration `flagarize:"name=query.timeout|help=Maximum time a query may take before being aborted.|default=2m"`
    QueryConcurrency     int            `flagarize:"name=query.max-concurrency|help=Maximum number of queries executed concurrently.|default=20"`
    QueryMaxSamples      int            `flagarize:"name=query.max-samples|help=Maximum number of samples a single query can load into memory. Note that queries will fail if they try to load more samples than this into memory, so this also limits the number of samples a query can return.|default=50000000"`

    Web      web.Options
    Notifier notifier.Options
    TSDB     tsdbOptions
    PromLog  promlog.Config
}{}

a := kingpin.New(filepath.Base(os.Args[0]), "The Prometheus monitoring server")
a.Version(version.Print("prometheus"))
a.HelpFlag.Short('h')

if err := flagarize.Flagarize(a, &cfg); err != nil {
    fmt.Fprintln(os.Stderr, err)
    os.Exit(2)
}

if _, err := a.Parse(os.Args[1:]); err != nil {
    fmt.Fprintln(os.Stderr, errors.Wrapf(err, "Error parsing commandline arguments"))
    a.Usage(os.Args[1:])
    os.Exit(2)
}

// Done!
```

Much cleaner (:

## Custom Type Parsing

Flagarize allows parsing of native types like int, string, etc (all that kingpin supports). For custom
types it's enough if your type implements part of `kingping.Value` interface as follows:

```go
// ValueFlagarizer is the simplest way to extend flagarize to parse your custom type.
// If any field has `flagarize:` struct tag and it implements the ValueFlagarizer, this will be
// used by kingping to parse the flag value.
//
// For an example see: `./timeduration.go` or `./regexp.go`
type ValueFlagarizer interface {
	// FlagarizeSetValue is invoked on kinpgin.Parse with the flag value passed as string.
	// It is expected from this method to parse the string to the underlying type.
	// This method has to be a pointer receiver for the method to take effect.
	// Flagarize will return error otherwise.
	Set(s string) error
}
```

## Custom Flags

Sometimes custom parsing is not enough. Sometimes you need to register more flags than just one from
single flagarize definition. To do so the type has to implement following interface:

```go
// Flagarizer is more advanced way to extend flagarize to parse a type. It allows to register
// more than one flag or register them in a custom way. It's ok for a method to register nothing.
// If any field implements `Flagarizer` this method will be invoked even if field does not
// have `flagarize:` struct tag.
//
// If the field implements both ValueFlagarizer and Flagarizer, only Flagarizer will be used.
//
// For an example usage see: `./pathorcontent.go`
type Flagarizer interface {
	// Flagarize is invoked on Flagarize. If field type does not implement custom Flagarizer
	// default one will be used.
	// Tag argument is nil if no `flagarize` struct tag was specified. Otherwise it has parsed
	//`flagarize` struct tag.
	// The ptr argument is an address of the already allocated type, that can be used
	// by FlagRegisterer kingping *Var methods.
	Flagarize(r FlagRegisterer, tag *Tag, ptr unsafe.Pointer) error
}
```
