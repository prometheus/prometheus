# Europa Universe

## What is Universe?

The Dagger Universe is a catalog of reusable CUE packages, curated by Dagger but possibly authored by third parties.
Most packages in Universe contain reusable actions; some may also contain entire configuration templates.

The import domain for Universe will be `universe.dagger.io`.

## Universe vs other packages

This table compares Dagger core packages, Dagger Universe packages, and the overall CUE package ecosystem.

|   |  *Dagger core* | *Dagger Universe* | *CUE ecosystem* |
|---|----------------|-------------------|-----------------|
| Import path |  `dagger.io` | `universe.dagger.io` | Everything else |
| Purpose |  Access core Dagger features | Safely reuse code from the Dagger community | Reuse any CUE code from anyone |
| Author | Dagger team | Dagger community, curated by Dagger | Anyone |
| Release cycle |    Released with Dagger engine   |  Released continuously | No release cycle |
| Size |  Small  | Large | Very large |
| Growth rate | Grows slowly, with engine features | Grows fast, with Dagger community | Grows even faster, with CUE ecosystem |

## Notable packages

### Docker API

*Import path: [`universe.dagger.io/docker`](./docker)*

The `docker` package is a native Cue API for Docker. You can use it to build, run, push and pull Docker containers directly from Cue.

The Dagger container API defines the following types:

* `#Image`: a container image
* `#Run`: run a command in a container
* `#Push`: upload an image to a repository
* `#Pull`: download an image from a repository
* `#Build`: build an image

### Examples

*Import path: [`universe.dagger.io/examples`](./examples)*

This package contains examples of complete Dagger configurations, including the result of following tutorials in the documentations.

For example, [the todoapp example](./examples/todoapp) corresponds to the [Getting Started tutorial](https://docs.dagger.io/1003/get-started/)

## Coding Style

When contributing, please follow the guidelines from the [Package Coding Style](https://docs.dagger.io/1226/coding-style).

## TODO LIST

* Support native language dev in `docker.#Run` with good DX (Python, Go, Typescript etc.)
* Easy file injection API (`container.#Image.files` ?)
* Use file injection instead of inline for `#Command.script` (to avoid hitting arg character limits)
* Organize universe packages in sub-categories?
