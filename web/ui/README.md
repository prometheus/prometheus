## Overview
The `ui` directory contains static files and templates used in the web UI. For
easier distribution they are compressed (c.f. Makefile) and statically compiled
into the Prometheus binary using the embed package.

During development it is more convenient to always use the files on disk to
directly see changes without recompiling.
To make this work, remove the `builtinassets` build tag in the `flags` entry
in `.promu.yml`, and then `make build` (or build Prometheus using
`go build ./cmd/prometheus`).

This will serve all files from your local filesystem. This is for development purposes only.

## React-app

### Introduction

The react application is a monorepo composed by multiple different npm packages. The main one is `react-app` which
contains the code of the react application.

Then you have different npm packages located in the folder `modules`. These packages are supposed to be used by the
react-app and also by others consumers (like Thanos)

### Pre-requisite

To be able to build the react application you need:

* npm >= v7
* node >= v20

### Installing npm dependencies

The React UI depends on a large number of [npm](https://www.npmjs.com/) packages. These are not checked in, so you will
need to move to the directory `web/ui` and then download and install them locally via the npm package manager:

    npm install

npm consults the `package.json` and `package-lock.json` files for dependencies to install. It creates a `node_modules`
directory with all installed dependencies.

**NOTE**: Do not run `npm install` in the `react-app` folder or in any sub folder of the `module` directory.

### Upgrading npm dependencies

As it is a monorepo, when upgrading a dependency, you have to upgrade it in every packages that composed this monorepo (
aka, in all sub folder of `module` and in `react-app`)

Then you have to run the command `npm install` in `web/ui` and not in a sub folder / sub package. It won't simply work.

### Running a local development server

You can start a development server for the React UI outside of a running Prometheus server by running:

    npm start

This will open a browser window with the React app running on http://localhost:3000/. The page will reload if you make
edits to the source code. You will also see any lint errors in the console.

**NOTE**: It will reload only if you change the code in `react-app` folder. Any code changes in the folder `module` is
not considered by the command `npm start`. In order to see the changes in the react-app you will have to
run `npm run build:module`

Due to a `"proxy": "http://localhost:9090"` setting in the `package.json` file, any API requests from the React UI are
proxied to `localhost` on port `9090` by the development server. This allows you to run a normal Prometheus server to
handle API requests, while iterating separately on the UI.

    [browser] ----> [localhost:3000 (dev server)] --(proxy API requests)--> [localhost:9090 (Prometheus)]

### Running tests

To run the test for the react-app and for all modules, you can simply run:

```bash
npm test
```

if you want to run the test only for a specific module, you need to go to the folder of the module and run
again `npm test`.

For example, in case you only want to run the test of the react-app, go to `web/ui/react-app` and run `npm test`

To generate an HTML-based test coverage report, run:

    CI=true npm test:coverage

This creates a `coverage` subdirectory with the generated report. Open `coverage/lcov-report/index.html` in the browser
to view it.

The `CI=true` environment variable prevents the tests from being run in interactive / watching mode.

See the [Create React App documentation](https://create-react-app.dev/docs/running-tests/) for more information about
running tests.

### Building the app for production

To build a production-optimized version of the React app to a `build` subdirectory, run:

    npm run build

**NOTE:** You will likely not need to do this directly. Instead, this is taken care of by the `build` target in the main
Prometheus `Makefile` when building the full binary.

### Integration into Prometheus

To build a Prometheus binary that includes a compiled-in version of the production build of the React app, change to the
root of the repository and run:

    make build

This installs dependencies via npm, builds a production build of the React app, and then finally compiles in all web
assets into the Prometheus binary.
