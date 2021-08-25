# Working with the React UI

This file explains how to work with the React-based Prometheus UI.

## Introduction

The [React-based](https://reactjs.org/) Prometheus UI was bootstrapped using [Create React App](https://github.com/facebook/create-react-app), a popular toolkit for generating React application setups. You can find general information about Create React App on [their documentation site](https://create-react-app.dev/).

Instead of plain JavaScript, we use [TypeScript](https://www.typescriptlang.org/) to ensure typed code.

## Development environment

To work with the React UI code, you will need to have the following tools installed:

* The [Node.js](https://nodejs.org/) JavaScript runtime.
* The [npm](https://www.npmjs.com/) package manager. Once you installed Node, npm should already be available.
* *Recommended:* An editor with TypeScript, React, and [ESLint](https://eslint.org/) linting support. See e.g. [Create React App's editor setup instructions](https://create-react-app.dev/docs/setting-up-your-editor/). If you are not sure which editor to use, we recommend using [Visual Studio Code](https://code.visualstudio.com/docs/languages/typescript). Make sure that [the editor uses the project's TypeScript version rather than its own](https://code.visualstudio.com/docs/typescript/typescript-compiling#_using-the-workspace-version-of-typescript).

**NOTE**: When using Visual Studio Code, be sure to open the `web/ui/react-app` directory in the editor instead of the root of the repository. This way, the right ESLint and TypeScript configuration will be picked up from the React workspace.

## Installing npm dependencies

The React UI depends on a large number of [npm](https://www.npmjs.com/) packages. These are not checked in, so you will need to download and install them locally via the npm package manager:

    npm install

npm consults the `package.json` and `package-lock.json` files for dependencies to install. It creates a `node_modules` directory with all installed dependencies.

**NOTE**: Remember to change directory to `web/ui/react-app` before running this command and the following commands.

## Running a local development server

You can start a development server for the React UI outside of a running Prometheus server by running:

    npm start

This will open a browser window with the React app running on http://localhost:3000/. The page will reload if you make edits to the source code. You will also see any lint errors in the console.

Due to a `"proxy": "http://localhost:9090"` setting in the `package.json` file, any API requests from the React UI are proxied to `localhost` on port `9090` by the development server. This allows you to run a normal Prometheus server to handle API requests, while iterating separately on the UI.

    [browser] ----> [localhost:3000 (dev server)] --(proxy API requests)--> [localhost:9090 (Prometheus)]

## Running tests

Create React App uses the [Jest](https://jestjs.io/) framework for running tests. To run tests in interactive watch mode:

    npm test

To generate an HTML-based test coverage report, run:

    CI=true npm test --coverage

This creates a `coverage` subdirectory with the generated report. Open `coverage/lcov-report/index.html` in the browser to view it.

The `CI=true` environment variable prevents the tests from being run in interactive / watching mode.

See the [Create React App documentation](https://create-react-app.dev/docs/running-tests/) for more information about running tests.

## Linting

We define linting rules for the [ESLint](https://eslint.org/) linter. We recommend integrating automated linting and fixing into your editor (e.g. upon save), but you can also run the linter separately from the command-line.

To detect and automatically fix lint errors, run:

    npm run lint

This is also available via the `react-app-lint-fix` target in the main Prometheus `Makefile`.

## Building the app for production

To build a production-optimized version of the React app to a `build` subdirectory, run:

    npm run build

**NOTE:** You will likely not need to do this directly. Instead, this is taken care of by the `build` target in the main Prometheus `Makefile` when building the full binary.

## Integration into Prometheus

To build a Prometheus binary that includes a compiled-in version of the production build of the React app, change to the root of the repository and run:

    make build

This installs dependencies via npm, builds a production build of the React app, and then finally compiles in all web assets into the Prometheus binary.
