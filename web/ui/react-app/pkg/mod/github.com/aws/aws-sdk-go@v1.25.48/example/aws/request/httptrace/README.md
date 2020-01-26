# Example

Demonstrates how the Go standard library `httptrace` can be used with the SDK
to collect HTTP request tracing timing using the SDK's API operation methods
like SNS's `PublishWithContext`.

The `trace.go` file demonstrates how the `httptrace` package's `ClientTrace`
can be created to gather timing information from HTTP requests made.

The `logger.go` file demonstrates how the trace information can be combined to
retrieve timing data for the different stages of the request.

The `config.go` file provides additional configuration settings to control how
the HTTP client and its transport is configured. Such as, timeouts, and
keepalive.

## Usage

Run the example providing your SNS topic's ARN as the `-topic` parameter. This
example assumes that the region is provided via the environment variable and
the AWS shared credentials file (~/.aws/credentials)'s `default` provide
provides credentials.

```sh
AWS_REGION=us-west-2 go run -tags example . -topic arn:aws:sns:us-west-2:0123456789:mytopicname
```

Once the example starts you'll be prompted with a `Message:` statement. Input
the message that you'd like to send to the topic on a single line and hit
`enter` to send it.

```
Message: My Really cool Message
```

The example will output the http trace timing information for how long the request took.

```
Latency: 79.863505ms ConnectionReused: false DNSStartAt: 280.452µs DNSDoneAt: 1.526342ms DNSDur: 1.24589ms ConnectStartAt: 1.533484ms ConnectDoneAt: 11.290792ms ConnectDur: 9.757308ms TLSStatAt: 11.331066ms TLSDoneAt: 33.912968ms TLSDur: 22.581902ms RequestWritten 34.951272ms RespFirstByte: 79.534808ms WaitRespFirstByte: 44.583536ms
```
