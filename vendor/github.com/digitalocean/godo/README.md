# Godo

[![Build Status](https://travis-ci.org/digitalocean/godo.svg)](https://travis-ci.org/digitalocean/godo)
[![GoDoc](https://godoc.org/github.com/digitalocean/godo?status.svg)](https://godoc.org/github.com/digitalocean/godo)

Godo is a Go client library for accessing the DigitalOcean V2 API.

You can view the client API docs here: [http://godoc.org/github.com/digitalocean/godo](http://godoc.org/github.com/digitalocean/godo)

You can view DigitalOcean API docs here: [https://developers.digitalocean.com/documentation/v2/](https://developers.digitalocean.com/documentation/v2/)

## Install
```sh
go get github.com/digitalocean/godo@vX.Y.Z
```

where X.Y.Z is the [version](https://github.com/digitalocean/godo/releases) you need.

or
```sh
go get github.com/digitalocean/godo
```
for non Go modules usage or latest version.

## Usage

```go
import "github.com/digitalocean/godo"
```

Create a new DigitalOcean client, then use the exposed services to
access different parts of the DigitalOcean API.

### Authentication

Currently, Personal Access Token (PAT) is the only method of
authenticating with the API. You can manage your tokens
at the DigitalOcean Control Panel [Applications Page](https://cloud.digitalocean.com/settings/applications).

You can then use your token to create a new client:

```go
package main

import (
    "github.com/digitalocean/godo"
)

func main() {
    client := godo.NewFromToken("my-digitalocean-api-token")
}
```

If you need to provide a `context.Context` to your new client, you should use [`godo.NewClient`](https://godoc.org/github.com/digitalocean/godo#NewClient) to manually construct a client instead.

## Examples


To create a new Droplet:

```go
dropletName := "super-cool-droplet"

createRequest := &godo.DropletCreateRequest{
    Name:   dropletName,
    Region: "nyc3",
    Size:   "s-1vcpu-1gb",
    Image: godo.DropletCreateImage{
        Slug: "ubuntu-14-04-x64",
    },
}

ctx := context.TODO()

newDroplet, _, err := client.Droplets.Create(ctx, createRequest)

if err != nil {
    fmt.Printf("Something bad happened: %s\n\n", err)
    return err
}
```

### Pagination

If a list of items is paginated by the API, you must request pages individually. For example, to fetch all Droplets:

```go
func DropletList(ctx context.Context, client *godo.Client) ([]godo.Droplet, error) {
    // create a list to hold our droplets
    list := []godo.Droplet{}

    // create options. initially, these will be blank
    opt := &godo.ListOptions{}
    for {
        droplets, resp, err := client.Droplets.List(ctx, opt)
        if err != nil {
            return nil, err
        }

        // append the current page's droplets to our list
        for _, d := range droplets {
            list = append(list, d)
        }

        // if we are at the last page, break out the for loop
        if resp.Links == nil || resp.Links.IsLastPage() {
            break
        }

        page, err := resp.Links.CurrentPage()
        if err != nil {
            return nil, err
        }

        // set the page we want for the next request
        opt.Page = page + 1
    }

    return list, nil
}
```

## Versioning

Each version of the client is tagged and the version is updated accordingly.

To see the list of past versions, run `git tag`.


## Documentation

For a comprehensive list of examples, check out the [API documentation](https://developers.digitalocean.com/documentation/v2/).

For details on all the functionality in this library, see the [GoDoc](http://godoc.org/github.com/digitalocean/godo) documentation.


## Contributing

We love pull requests! Please see the [contribution guidelines](CONTRIBUTING.md).
