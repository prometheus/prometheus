/*
Package collins is a client library for our inventory management database
Collins (http://tumblr.github.io/collins/) . It covers the full API and allows
you to manage your assets and their data from Go applications. It is very much
influenced by the beautiful go-github project
(https://github.com/google/go-github).

While we do actively use go-collins at Tumblr and will try to maintain a stable
interface, for now consider this a work-in-progress in that breaking changes may
be made.

Usage

To start querying collins, import go-collins and set up a new `Client`
by using `NewClient()` or `NewClientFromYaml()`. The second function will look
for a `collins.yml` file and use credentials from it, while `NewClient()` takes
credentials as parameters.

	import (
		"fmt"

		"gopkg.in/tumblr/go-collins.v0"
	)

	func main() {
		client, err := collins.NewClient("username", "password", "https://collins.example.com")
		if err != nil {
			fmt.Errorf("Could not set up collins client: %s", err)
		}
		// Use client to interact with collins
	}

In the client struct, there are pointers to services that can be used to talk
to specific parts of the API. The most common use (at least at Tumblr) is
talking to the asset API for things like getting the number of physical CPUs in
an asset:

	asset, _, err := client.Assets.Get("assettag1")
	if err != nil {
		fmt.Errorf("Assets.Get returned error: %s", err)
	}
	fmt.Printf("CPUs: %d\n", len(asset.Hardware.CPUs))

Or finding assets that match certain criteria:

	opts := collins.AssetFindOpts{
		Status: "Unallocated",
	}

	assets, _, err := client.Assets.Find(&opts)
	if err != nil {
		fmt.Errorf("Assets.Find returned error: %s", err)
	}

	fmt.Printf("First unallocated tag: %s\n", assets[0].Metadata.Tag)

Pagination

Some routes in the API (finding assets and fetching logs) are paginated. To
support pagination we include pagination information in the Request struct
returned by the functions. The members are `PreviousPage`, `CurrentPage`,
`NextPage` and `TotalResults`. These together with the `PageOpts` struct can be
used to navigate through the pages.

	opts := collins.AssetFindOpts{
		Status: "Unallocated",
		PageOpts: collins.PageOpts{Page: 0}
	}

	for {
		assets, resp, err := client.Assets.Find(&opts)
		if err != nil {
			fmt.Errorf("Assets.Find returned error: %s", err)
		}

		for _, asset := range assets {
			fmt.Printf("Tag: %s\n", asset.Tag)
		}

		if resp.NextPage == resp.CurrentPage { // No more pages
			break
		} else { // Fetch next page
			opts.PageOpts.Page++
		}
	}
*/
package collins
