# Kingpin - A Go (golang) command line and flag parser
[![](https://godoc.org/github.com/alecthomas/kingpin?status.svg)](http://godoc.org/github.com/alecthomas/kingpin) [![Build Status](https://travis-ci.org/alecthomas/kingpin.svg?branch=master)](https://travis-ci.org/alecthomas/kingpin) [![Gitter chat](https://badges.gitter.im/alecthomas.png)](https://gitter.im/alecthomas/Lobby)



<!-- MarkdownTOC -->

- [Overview](#overview)
- [Features](#features)
- [User-visible changes between v1 and v2](#user-visible-changes-between-v1-and-v2)
  - [Flags can be used at any point after their definition.](#flags-can-be-used-at-any-point-after-their-definition)
  - [Short flags can be combined with their parameters](#short-flags-can-be-combined-with-their-parameters)
- [API changes between v1 and v2](#api-changes-between-v1-and-v2)
- [Versions](#versions)
  - [V2 is the current stable version](#v2-is-the-current-stable-version)
  - [V1 is the OLD stable version](#v1-is-the-old-stable-version)
- [Change History](#change-history)
- [Examples](#examples)
  - [Simple Example](#simple-example)
  - [Complex Example](#complex-example)
- [Reference Documentation](#reference-documentation)
  - [Displaying errors and usage information](#displaying-errors-and-usage-information)
  - [Sub-commands](#sub-commands)
  - [Custom Parsers](#custom-parsers)
  - [Repeatable flags](#repeatable-flags)
  - [Boolean Values](#boolean-values)
  - [Default Values](#default-values)
  - [Place-holders in Help](#place-holders-in-help)
  - [Consuming all remaining arguments](#consuming-all-remaining-arguments)
  - [Bash/ZSH Shell Completion](#bashzsh-shell-completion)
  - [Supporting -h for help](#supporting--h-for-help)
  - [Custom help](#custom-help)

<!-- /MarkdownTOC -->

## Overview

Kingpin is a [fluent-style](http://en.wikipedia.org/wiki/Fluent_interface),
type-safe command-line parser. It supports flags, nested commands, and
positional arguments.

Install it with:

    $ go get gopkg.in/alecthomas/kingpin.v2

It looks like this:

```go
var (
  verbose = kingpin.Flag("verbose", "Verbose mode.").Short('v').Bool()
  name    = kingpin.Arg("name", "Name of user.").Required().String()
)

func main() {
  kingpin.Parse()
  fmt.Printf("%v, %s\n", *verbose, *name)
}
```

More [examples](https://github.com/alecthomas/kingpin/tree/master/_examples) are available.

Second to parsing, providing the user with useful help is probably the most
important thing a command-line parser does. Kingpin tries to provide detailed
contextual help if `--help` is encountered at any point in the command line
(excluding after `--`).

## Features

- Help output that isn't as ugly as sin.
- Fully [customisable help](#custom-help), via Go templates.
- Parsed, type-safe flags (`kingpin.Flag("f", "help").Int()`)
- Parsed, type-safe positional arguments (`kingpin.Arg("a", "help").Int()`).
- Parsed, type-safe, arbitrarily deep commands (`kingpin.Command("c", "help")`).
- Support for required flags and required positional arguments (`kingpin.Flag("f", "").Required().Int()`).
- Support for arbitrarily nested default commands (`command.Default()`).
- Callbacks per command, flag and argument (`kingpin.Command("c", "").Action(myAction)`).
- POSIX-style short flag combining (`-a -b` -> `-ab`).
- Short-flag+parameter combining (`-a parm` -> `-aparm`).
- Read command-line from files (`@<file>`).
- Automatically generate man pages (`--help-man`).

## User-visible changes between v1 and v2

### Flags can be used at any point after their definition.

Flags can be specified at any point after their definition, not just
*immediately after their associated command*. From the chat example below, the
following used to be required:

```
$ chat --server=chat.server.com:8080 post --image=~/Downloads/owls.jpg pics
```

But the following will now work:

```
$ chat post --server=chat.server.com:8080 --image=~/Downloads/owls.jpg pics
```

### Short flags can be combined with their parameters

Previously, if a short flag was used, any argument to that flag would have to
be separated by a space. That is no longer the case.

## API changes between v1 and v2

- `ParseWithFileExpansion()` is gone. The new parser directly supports expanding `@<file>`.
- Added `FatalUsage()` and `FatalUsageContext()` for displaying an error + usage and terminating.
- `Dispatch()` renamed to `Action()`.
- Added `ParseContext()` for parsing a command line into its intermediate context form without executing.
- Added `Terminate()` function to override the termination function.
- Added `UsageForContextWithTemplate()` for printing usage via a custom template.
- Added `UsageTemplate()` for overriding the default template to use. Two templates are included:
    1. `DefaultUsageTemplate` - default template.
    2. `CompactUsageTemplate` - compact command template for larger applications.

## Versions

Kingpin uses [gopkg.in](https://gopkg.in/alecthomas/kingpin) for versioning.

The current stable version is [gopkg.in/alecthomas/kingpin.v2](https://gopkg.in/alecthomas/kingpin.v2). The previous version, [gopkg.in/alecthomas/kingpin.v1](https://gopkg.in/alecthomas/kingpin.v1), is deprecated and in maintenance mode.

### [V2](https://gopkg.in/alecthomas/kingpin.v2) is the current stable version

Installation:

```sh
$ go get gopkg.in/alecthomas/kingpin.v2
```

### [V1](https://gopkg.in/alecthomas/kingpin.v1) is the OLD stable version

Installation:

```sh
$ go get gopkg.in/alecthomas/kingpin.v1
```

## Change History

- *2015-09-19* -- Stable v2.1.0 release.
    - Added `command.Default()` to specify a default command to use if no other
      command matches. This allows for convenient user shortcuts.
    - Exposed `HelpFlag` and `VersionFlag` for further customisation.
    - `Action()` and `PreAction()` added and both now support an arbitrary
      number of callbacks.
    - `kingpin.SeparateOptionalFlagsUsageTemplate`.
    - `--help-long` and `--help-man` (hidden by default) flags.
    - Flags are "interspersed" by default, but can be disabled with `app.Interspersed(false)`.
    - Added flags for all simple builtin types (int8, uint16, etc.) and slice variants.
    - Use `app.Writer(os.Writer)` to specify the default writer for all output functions.
    - Dropped `os.Writer` prefix from all printf-like functions.

- *2015-05-22* -- Stable v2.0.0 release.
    - Initial stable release of v2.0.0.
    - Fully supports interspersed flags, commands and arguments.
    - Flags can be present at any point after their logical definition.
    - Application.Parse() terminates if commands are present and a command is not parsed.
    - Dispatch() -> Action().
    - Actions are dispatched after all values are populated.
    - Override termination function (defaults to os.Exit).
    - Override output stream (defaults to os.Stderr).
    - Templatised usage help, with default and compact templates.
    - Make error/usage functions more consistent.
    - Support argument expansion from files by default (with @<file>).
    - Fully public data model is available via .Model().
    - Parser has been completely refactored.
    - Parsing and execution has been split into distinct stages.
    - Use `go generate` to generate repeated flags.
    - Support combined short-flag+argument: -fARG.

- *2015-01-23* -- Stable v1.3.4 release.
    - Support "--" for separating flags from positional arguments.
    - Support loading flags from files (ParseWithFileExpansion()). Use @FILE as an argument.
    - Add post-app and post-cmd validation hooks. This allows arbitrary validation to be added.
    - A bunch of improvements to help usage and formatting.
    - Support arbitrarily nested sub-commands.

- *2014-07-08* -- Stable v1.2.0 release.
    - Pass any value through to `Strings()` when final argument.
      Allows for values that look like flags to be processed.
    - Allow `--help` to be used with commands.
    - Support `Hidden()` flags.
    - Parser for [units.Base2Bytes](https://github.com/alecthomas/units)
      type. Allows for flags like `--ram=512MB` or `--ram=1GB`.
    - Add an `Enum()` value, allowing only one of a set of values
      to be selected. eg. `Flag(...).Enum("debug", "info", "warning")`.

- *2014-06-27* -- Stable v1.1.0 release.
    - Bug fixes.
    - Always return an error (rather than panicing) when misconfigured.
    - `OpenFile(flag, perm)` value type added, for finer control over opening files.
    - Significantly improved usage formatting.

- *2014-06-19* -- Stable v1.0.0 release.
    - Support [cumulative positional](#consuming-all-remaining-arguments) arguments.
    - Return error rather than panic when there are fatal errors not caught by
      the type system. eg. when a default value is invalid.
    - Use gokpg.in.

- *2014-06-10* -- Place-holder streamlining.
    - Renamed `MetaVar` to `PlaceHolder`.
    - Removed `MetaVarFromDefault`. Kingpin now uses [heuristics](#place-holders-in-help)
      to determine what to display.

## Examples

### Simple Example

Kingpin can be used for simple flag+arg applications like so:

```
$ ping --help
usage: ping [<flags>] <ip> [<count>]

Flags:
  --debug            Enable debug mode.
  --help             Show help.
  -t, --timeout=5s   Timeout waiting for ping.

Args:
  <ip>        IP address to ping.
  [<count>]   Number of packets to send
$ ping 1.2.3.4 5
Would ping: 1.2.3.4 with timeout 5s and count 0
```

From the following source:

```go
package main

import (
  "fmt"

  "gopkg.in/alecthomas/kingpin.v2"
)

var (
  debug   = kingpin.Flag("debug", "Enable debug mode.").Bool()
  timeout = kingpin.Flag("timeout", "Timeout waiting for ping.").Default("5s").OverrideDefaultFromEnvar("PING_TIMEOUT").Short('t').Duration()
  ip      = kingpin.Arg("ip", "IP address to ping.").Required().IP()
  count   = kingpin.Arg("count", "Number of packets to send").Int()
)

func main() {
  kingpin.Version("0.0.1")
  kingpin.Parse()
  fmt.Printf("Would ping: %s with timeout %s and count %d\n", *ip, *timeout, *count)
}
```

### Complex Example

Kingpin can also produce complex command-line applications with global flags,
subcommands, and per-subcommand flags, like this:

```
$ chat --help
usage: chat [<flags>] <command> [<flags>] [<args> ...]

A command-line chat application.

Flags:
  --help              Show help.
  --debug             Enable debug mode.
  --server=127.0.0.1  Server address.

Commands:
  help [<command>]
    Show help for a command.

  register <nick> <name>
    Register a new user.

  post [<flags>] <channel> [<text>]
    Post a message to a channel.

$ chat help post
usage: chat [<flags>] post [<flags>] <channel> [<text>]

Post a message to a channel.

Flags:
  --image=IMAGE  Image to post.

Args:
  <channel>  Channel to post to.
  [<text>]   Text to post.

$ chat post --image=~/Downloads/owls.jpg pics
...
```

From this code:

```go
package main

import (
  "os"
  "strings"
  "gopkg.in/alecthomas/kingpin.v2"
)

var (
  app      = kingpin.New("chat", "A command-line chat application.")
  debug    = app.Flag("debug", "Enable debug mode.").Bool()
  serverIP = app.Flag("server", "Server address.").Default("127.0.0.1").IP()

  register     = app.Command("register", "Register a new user.")
  registerNick = register.Arg("nick", "Nickname for user.").Required().String()
  registerName = register.Arg("name", "Name of user.").Required().String()

  post        = app.Command("post", "Post a message to a channel.")
  postImage   = post.Flag("image", "Image to post.").File()
  postChannel = post.Arg("channel", "Channel to post to.").Required().String()
  postText    = post.Arg("text", "Text to post.").Strings()
)

func main() {
  switch kingpin.MustParse(app.Parse(os.Args[1:])) {
  // Register user
  case register.FullCommand():
    println(*registerNick)

  // Post message
  case post.FullCommand():
    if *postImage != nil {
    }
    text := strings.Join(*postText, " ")
    println("Post:", text)
  }
}
```

## Reference Documentation

### Displaying errors and usage information

Kingpin exports a set of functions to provide consistent errors and usage
information to the user.

Error messages look something like this:

    <app>: error: <message>

The functions on `Application` are:

Function | Purpose
---------|--------------
`Errorf(format, args)` | Display a printf formatted error to the user.
`Fatalf(format, args)` | As with Errorf, but also call the termination handler.
`FatalUsage(format, args)` | As with Fatalf, but also print contextual usage information.
`FatalUsageContext(context, format, args)` | As with Fatalf, but also print contextual usage information from a `ParseContext`.
`FatalIfError(err, format, args)` | Conditionally print an error prefixed with format+args, then call the termination handler

There are equivalent global functions in the kingpin namespace for the default
`kingpin.CommandLine` instance.

### Sub-commands

Kingpin supports nested sub-commands, with separate flag and positional
arguments per sub-command. Note that positional arguments may only occur after
sub-commands.

For example:

```go
var (
  deleteCommand     = kingpin.Command("delete", "Delete an object.")
  deleteUserCommand = deleteCommand.Command("user", "Delete a user.")
  deleteUserUIDFlag = deleteUserCommand.Flag("uid", "Delete user by UID rather than username.")
  deleteUserUsername = deleteUserCommand.Arg("username", "Username to delete.")
  deletePostCommand = deleteCommand.Command("post", "Delete a post.")
)

func main() {
  switch kingpin.Parse() {
  case "delete user":
  case "delete post":
  }
}
```

### Custom Parsers

Kingpin supports both flag and positional argument parsers for converting to
Go types. For example, some included parsers are `Int()`, `Float()`,
`Duration()` and `ExistingFile()` (see [parsers.go](./parsers.go) for a complete list of included parsers).

Parsers conform to Go's [`flag.Value`](http://godoc.org/flag#Value)
interface, so any existing implementations will work.

For example, a parser for accumulating HTTP header values might look like this:

```go
type HTTPHeaderValue http.Header

func (h *HTTPHeaderValue) Set(value string) error {
  parts := strings.SplitN(value, ":", 2)
  if len(parts) != 2 {
    return fmt.Errorf("expected HEADER:VALUE got '%s'", value)
  }
  (*http.Header)(h).Add(parts[0], parts[1])
  return nil
}

func (h *HTTPHeaderValue) String() string {
  return ""
}
```

As a convenience, I would recommend something like this:

```go
func HTTPHeader(s Settings) (target *http.Header) {
  target = &http.Header{}
  s.SetValue((*HTTPHeaderValue)(target))
  return
}
```

You would use it like so:

```go
headers = HTTPHeader(kingpin.Flag("header", "Add a HTTP header to the request.").Short('H'))
```

### Repeatable flags

Depending on the `Value` they hold, some flags may be repeated. The
`IsCumulative() bool` function on `Value` tells if it's safe to call `Set()`
multiple times or if an error should be raised if several values are passed.

The built-in `Value`s returning slices and maps, as well as `Counter` are
examples of `Value`s that make a flag repeatable.

### Boolean values

Boolean values are uniquely managed by Kingpin. Each boolean flag will have a negative complement:
`--<name>` and `--no-<name>`.

### Default Values

The default value is the zero value for a type. This can be overridden with
the `Default(value...)` function on flags and arguments. This function accepts
one or several strings, which are parsed by the value itself, so they *must*
be compliant with the format expected.

### Place-holders in Help

The place-holder value for a flag is the value used in the help to describe
the value of a non-boolean flag.

The value provided to PlaceHolder() is used if provided, then the value
provided by Default() if provided, then finally the capitalised flag name is
used.

Here are some examples of flags with various permutations:

    --name=NAME           // Flag(...).String()
    --name="Harry"        // Flag(...).Default("Harry").String()
    --name=FULL-NAME      // flag(...).PlaceHolder("FULL-NAME").Default("Harry").String()

### Consuming all remaining arguments

A common command-line idiom is to use all remaining arguments for some
purpose. eg. The following command accepts an arbitrary number of
IP addresses as positional arguments:

    ./cmd ping 10.1.1.1 192.168.1.1

Such arguments are similar to [repeatable flags](#repeatable-flags), but for
arguments. Therefore they use the same `IsCumulative() bool` function on the
underlying `Value`, so the built-in `Value`s for which the `Set()` function
can be called several times will consume multiple arguments.

To implement the above example with a custom `Value`, we might do something
like this:

```go
type ipList []net.IP

func (i *ipList) Set(value string) error {
  if ip := net.ParseIP(value); ip == nil {
    return fmt.Errorf("'%s' is not an IP address", value)
  } else {
    *i = append(*i, ip)
    return nil
  }
}

func (i *ipList) String() string {
  return ""
}

func (i *ipList) IsCumulative() bool {
  return true
}

func IPList(s Settings) (target *[]net.IP) {
  target = new([]net.IP)
  s.SetValue((*ipList)(target))
  return
}
```

And use it like so:

```go
ips := IPList(kingpin.Arg("ips", "IP addresses to ping."))
```

### Bash/ZSH Shell Completion

By default, all flags and commands/subcommands generate completions 
internally.

Out of the box, CLI tools using kingpin should be able to take advantage 
of completion hinting for flags and commands. By specifying 
`--completion-bash` as the first argument, your CLI tool will show 
possible subcommands. By ending your argv with `--`, hints for flags 
will be shown.

To allow your end users to take advantage you must package a 
`/etc/bash_completion.d` script with your distribution (or the equivalent 
for your target platform/shell). An alternative is to instruct your end 
user to source a script from their `bash_profile` (or equivalent).

Fortunately Kingpin makes it easy to generate or source a script for use
with end users shells. `./yourtool --completion-script-bash` and 
`./yourtool --completion-script-zsh` will generate these scripts for you.

**Installation by Package**

For the best user experience, you should bundle your pre-created 
completion script with your CLI tool and install it inside 
`/etc/bash_completion.d` (or equivalent). A good suggestion is to add 
this as an automated step to your build pipeline, in the implementation 
is improved for bug fixed.

**Installation by `bash_profile`**

Alternatively, instruct your users to add an additional statement to 
their `bash_profile` (or equivalent):

```
eval "$(your-cli-tool --completion-script-bash)"
```

Or for ZSH

```
eval "$(your-cli-tool --completion-script-zsh)"
```

#### Additional API
To provide more flexibility, a completion option API has been
exposed for flags to allow user defined completion options, to extend
completions further than just EnumVar/Enum. 


**Provide Static Options**

When using an `Enum` or `EnumVar`, users are limited to only the options 
given. Maybe we wish to hint possible options to the user, but also 
allow them to provide their own custom option. `HintOptions` gives
this functionality to flags.

```
app := kingpin.New("completion", "My application with bash completion.")
app.Flag("port", "Provide a port to connect to").
    Required().
    HintOptions("80", "443", "8080").
    IntVar(&c.port)
```

**Provide Dynamic Options**
Consider the case that you needed to read a local database or a file to 
provide suggestions. You can dynamically generate the options

```
func listHosts() []string {
  // Provide a dynamic list of hosts from a hosts file or otherwise
  // for bash completion. In this example we simply return static slice.

  // You could use this functionality to reach into a hosts file to provide
  // completion for a list of known hosts.
  return []string{"sshhost.example", "webhost.example", "ftphost.example"}
}

app := kingpin.New("completion", "My application with bash completion.")
app.Flag("flag-1", "").HintAction(listHosts).String()
```

**EnumVar/Enum**
When using `Enum` or `EnumVar`, any provided options will be automatically
used for bash autocompletion. However, if you wish to provide a subset or 
different options, you can use `HintOptions` or `HintAction` which will override
the default completion options for `Enum`/`EnumVar`.


**Examples**
You can see an in depth example of the completion API within 
`examples/completion/main.go`


### Supporting -h for help

`kingpin.CommandLine.HelpFlag.Short('h')`

### Custom help

Kingpin v2 supports templatised help using the text/template library (actually, [a fork](https://github.com/alecthomas/template)).

You can specify the template to use with the [Application.UsageTemplate()](http://godoc.org/gopkg.in/alecthomas/kingpin.v2#Application.UsageTemplate) function.

There are four included templates: `kingpin.DefaultUsageTemplate` is the default,
`kingpin.CompactUsageTemplate` provides a more compact representation for more complex command-line structures,
`kingpin.SeparateOptionalFlagsUsageTemplate` looks like the default template, but splits required
and optional command flags into separate lists, and `kingpin.ManPageTemplate` is used to generate man pages.

See the above templates for examples of usage, and the the function [UsageForContextWithTemplate()](https://github.com/alecthomas/kingpin/blob/master/usage.go#L198) method for details on the context.

#### Default help template

```
$ go run ./examples/curl/curl.go --help
usage: curl [<flags>] <command> [<args> ...]

An example implementation of curl.

Flags:
  --help            Show help.
  -t, --timeout=5s  Set connection timeout.
  -H, --headers=HEADER=VALUE
                    Add HTTP headers to the request.

Commands:
  help [<command>...]
    Show help.

  get url <url>
    Retrieve a URL.

  get file <file>
    Retrieve a file.

  post [<flags>] <url>
    POST a resource.
```

#### Compact help template

```
$ go run ./examples/curl/curl.go --help
usage: curl [<flags>] <command> [<args> ...]

An example implementation of curl.

Flags:
  --help            Show help.
  -t, --timeout=5s  Set connection timeout.
  -H, --headers=HEADER=VALUE
                    Add HTTP headers to the request.

Commands:
  help [<command>...]
  get [<flags>]
    url <url>
    file <file>
  post [<flags>] <url>
```
