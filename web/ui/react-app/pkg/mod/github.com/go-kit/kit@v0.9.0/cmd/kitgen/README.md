# kitgen
kitgen is an experimental code generation utility that helps with some of the
boilerplate code required to implement the "onion" pattern `go-kit` utilizes.

## Usage
Before using this tool please explore the [testdata]() directory for examples
of the inputs it requires and the outputs that will be produced. _You may not
need this tool._ If you are new to and just learning `go-kit` or if your use
case involves introducing `go-kit` to an existing codebase you are better
suited by slowly building out the "onion" by hand.

Before starting you need to *install* `kitgen` utility — see instructions below.
1. **Define** your service. Create a `.go` file with the definition of your
Service interface and any of the custom types it refers to:
```go
// service.go
package profilesvc // don't forget to name your package

type Service interface {
    PostProfile(ctx context.Context, p Profile) error
    // ...
}
type Profile struct {
    ID        string    `json:"id"`
    Name      string    `json:"name,omitempty"`
    // ...
}
```
2. **Generate** your code. Run the following command:
```sh
kitgen ./service.go
# kitgen has a couple of flags that you may find useful

# keep all code in the root directory
kitgen -repo-layout flat ./service.go

# put generated code elsewhere
kitgen -target-dir ~/Projects/gohome/src/home.com/kitchenservice/brewcoffee
```

## Installation
1. **Fetch** the `inlinefiles` utility. Go generate will use it to create your
code:
```
go get github.com/nyarly/inlinefiles
```
2. **Install** the binary for easy access to `kitgen`. Run the following commands:
```sh
cd $GOPATH/src/github.com/go-kit/kit/cmd/kitgen
go install

# Check installation by running:
kitgen -h
```
