//Deprecated: in favor of universe.dagger.io/alpha package
package dotnet

import (
	"dagger.io/dagger"
	"universe.dagger.io/x/olli.janatuinen@gmail.com/dotnet"
)

dagger.#Plan & {
	client: filesystem: "./data": read: contents: dagger.#FS

	actions: test: dotnet.#Test & {
		source:  client.filesystem."./data".read.contents
		package: "./Greeting.Tests"
	}
}
