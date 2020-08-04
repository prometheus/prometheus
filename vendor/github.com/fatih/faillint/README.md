# faillint [![](https://github.com/fatih/faillint/workflows/build/badge.svg)](https://github.com/fatih/faillint/actions)

Faillint is a simple Go linter that fails when a specific set of import paths
or exported path's functions, constant, vars or types are used. It's meant to be
used in CI/CD environments to catch rules you want to enforce in your projects.

As an example, you could enforce the usage of `github.com/pkg/errors` instead
of the `errors` package. To prevent the usage of the `errors` package, you can
configure `faillint` to fail whenever someone imports the `errors` package in
this case. To make sure `fmt.Errorf` is not used for creating errors as well,
you can configure to fail on such single function of `fmt` as well.

![faillint](https://user-images.githubusercontent.com/438920/74105802-f7158300-4b15-11ea-8e23-16be5cd3b971.gif)

## Install

```bash
go get github.com/fatih/faillint
```

## Usage

`faillint` works on a file, directory or a Go package:

```sh
$ faillint -paths "errors,fmt.{Errorf}" foo.go # pass a file
$ faillint -paths "errors,fmt.{Errorf}" ./...  # recursively analyze all files
$ faillint -paths "errors,fmt.{Errorf}" github.com/fatih/gomodifytags # or pass a package
```

By default, `faillint` will not check any import paths. You need to explicitly
define it with the `-paths` flag, which is comma-separated list. Some examples are:

```
# Fail if the errors package is used.
-paths "errors"

# Fail if the old context package is imported.
-paths "golang.org/x/net/context"

# Fail both on stdlib log and errors package to enforce other internal libraries.
-paths "log,errors"

# Fail if any of Print, Printf of Println function were used from fmt library.
-paths "fmt.{Print,Printf,Println}"

# Fail if the package is imported including sub paths starting with
  "golang.org/x/net/". In example: `golang.org/x/net/context`, 
  `golang.org/x/net/nettest`, .nettest`, ...
-paths "golang.org/x/net/..."
```

If you have a preferred import path to suggest, append the suggestion after a `=` character:

```
# Fail if the errors package is used and suggest to use github.com/pkg/errors.
-paths "errors=github.com/pkg/errors"

# Fail for the old context import path and suggest to use the stdlib context.
-paths "golang.org/x/net/context=context"

# Fail both on stdlib log and errors package to enforce other libraries.
-paths "log=go.uber.org/zap,errors=github.com/pkg/errors"

# Fail on fmt.Errorf and suggest the Errorf function from github.compkg/errors instead.
-paths "fmt.{Errorf}=github.com/pkg/errors.{Errorf}"
```

### Ignoring problems

If you want to ignore a problem reported by `faillint` you can add a lint directive based on [staticcheck](https://staticcheck.io)'s design.

#### Line-based lint directives

Line-based lint directives can be applied to imports or functions you want to tolerate.  The format is,

```go
//lint:ignore faillint reason
```

For example,

```go
package a

import (
        //lint:ignore faillint Whatever your reason is.
        "errors"
        "fmt" //lint:ignore faillint Whatever your reason is.
)

func foo() error {
        //lint:ignore faillint Whatever your reason is.
        return errors.New("bar!")
}
```

#### File-based lint directives

File-based lint directives can be applied to ignore `faillint` problems in a whole file.  The format is,

```go
//lint:file-ignore faillint reason
```

This may be placed anywhere in the file but conventionally it should be placed at, or near, the top of the file.

For example,

```go
//lint:file-ignore faillint This file should be ignored by faillint.

package a

import (
        "errors"
)

func foo() error {
        return errors.New("bar!")
}
```

## Example

Assume we have the following file:

```go
package a

import (
        "errors"
)

func foo() error {
        return errors.New("bar!")
}
```

Let's run `faillint` to check if `errors` import is used and report it:

```
$ faillint -paths "errors=github.com/pkg/errors" a.go
a.go:4:2: package "errors" shouldn't be imported, suggested: "github.com/pkg/errors"
```

## The need for this tool?

Most of these checks should be probably detected during the review cycle. But
it's totally normal to accidentally import them (we're all humans in the end).

Second, tools like `goimports` favors certain packages. As an example going
forward if you decided to use `github.com/pkg/errors` in you project, and write
`errors.New()` in a new file, `goimports` will automatically import the
`errors` package (and not `github.com/pkg/errors`). The code will perfectly
compile. `faillint` would be able to detect and report it to you.

## Credits

This tool is built on top of the excellent `go/analysis` package that makes it
easy to write custom analyzers in Go. If you're interested in writing a tool,
check out my **[Using go/analysis to write a custom
linter](https://arslan.io/2019/06/13/using-go-analysis-to-write-a-custom-linter/)**
blog post.

Part of the code is modified and based on [astutil.UsesImport](https://pkg.go.dev/golang.org/x/tools/go/ast/astutil?tab=doc#UsesImport)
