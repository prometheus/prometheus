## Overview

The `ui` directory contains the following subdirectories:

* `mantine-ui`: The new (3.x) React-based web UI for Prometheus.
* `react-app`: The old (2.x) React-based web UI for Prometheus.
* `modules`: Shared npm modules for PromQL code editing via [CodeMirror](https://codemirror.net/), which are used by both React apps and external consumers (like [Thanos](https://thanos.io/)).
* `static`: The build output directory for both React apps. The files in this directory are compiled into the Prometheus binary unless built-in assets are disabled (see the section on this below).

The directory also contains helper files for building and compiling the UI assets for both React application versions into the Prometheus binary.

Prometheus serves the new UI by default, but you can still use the Prometheus server feature flag `--enable-feature=old-ui` to switch back to the old UI for the time being.

While both the `mantine-ui` and `modules` directories are part of the same shared npm workspace, the old UI in the `react-app` directory has been separated out of the workspace setup, since its dependencies were too incompatible to integrate.

### Pre-requisites

To be able to build either of the React applications, you will need:

* npm >= v10
* node >= v22

### Installing npm dependencies

To install all required [npm](https://www.npmjs.com/) package dependencies and also build the local workspace npm modules, run this command from the root of the repository:

```bash
make ui-build
```

This will run `npm install` both in the main `web/ui` workspace directory, as well as in the `web/ui/react-app` directory, and it will further run `npm run build` in both directories to make sure that both apps and their dependencies are built correctly.

npm consults the `package.json` and `package-lock.json` files for dependencies to install. It creates a `node_modules` directory with all installed dependencies.

**NOTE**: Do not run `npm install` directly in the `mantine-ui` folder or in any sub folder of the `module` directory - dependencies for these should be installed only via the npm workspace setup from `web/ui`.

### Running a local development server

You can start a development server for the new React UI outside of a running Prometheus server by running:

    npm start

(For the old UI, you will have to run the same command from the `react-app` subdirectory.)

This will start the development server on http://localhost:5173/. The page will hot-reload if you make edits to the source code. You will also see any lint errors in the console.

**NOTE**: Hot reloads will only work for code in the `mantine-ui` and `react-app` folders. For changes in the `module` directory (the CodeMirror PromQL editor code) to become visible, you will need to run `npm run build:module` from `web/ui`.

### Proxying API requests to a Prometheus backend server

To do anything useful, the web UI requires a Prometheus backend to fetch and display data from. Due to a proxy configuration in the `mantine-ui/vite.config.ts` file, the development web server proxies any API requests from the UI to `http://localhost:9090`. This allows you to run a normal Prometheus server to handle API requests, while iterating separately on the UI:

    [browser] ----> [localhost:5173 (dev server)] --(proxy API requests)--> [localhost:9090 (Prometheus)]

If you prefer, you can also change the `mantine-ui/vite.config.ts` file to point to a any other Prometheus server. Note that connecting to an HTTPS-based server will require an additional `changeOrigin: true` setting. For example, to connect to the demo server at `https://prometheus.demo.prometheus.io/`, you could change the `vite.config.ts` file to:

```typescript
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// https://vitejs.dev/config/
export default defineConfig({
  base: '',
  plugins: [react()],
  server: {
    proxy: {
      "/api": {
        target: "https://prometheus.demo.prometheus.io/",
        changeOrigin: true,
      },
      "/-/": {
        target: "https://prometheus.demo.prometheus.io/",
        changeOrigin: true,
      },
    },
  },
});
```

### Running tests

To run the tests for the new React app and for all modules:

```bash
npm test
```

(For the old UI, you would have to run the same command from the `react-app` subdirectory.)

To run the tests only for a specific module, change to the module's directory and run `npm test` from there.

By default, `npm test` will run the tests in an interactive watch mode. This means that it will keep running the tests when you change any source files.
To run the tests only once and then exit, use the `CI=true` environment variable:

```bash
CI=true npm test
```

### Building the app for production

To build a production-optimized version of both React app versions to the `static/{react-app,mantine-ui}` output directories, run:

    npm run build

**NOTE:** You will likely not need to do this directly. Instead, this is taken care of by the `build` target in the main Prometheus `Makefile` when building the full binary.

### Upgrading npm dependencies

As this is a monorepo containing multiple npm packages, you will have to upgrade dependencies in every package individually (in all sub folders of `module`, `react-app`, and `mantine-ui`).

Then, run `npm install` in `web/ui` and `web/ui/react-app` directories, but not in the other sub folders / sub packages (this won't produce the desired results due to the npm workspace setup).

### Integration into Prometheus

To build a Prometheus binary that includes a compiled-in version of the production build of both React app versions, change to the
root of the repository and run:

```bash
make build
```

This installs dependencies via npm, builds a production build of both React apps, and then finally compiles in all web assets into the Prometheus binary.

### Serving UI assets from the filesystem

By default, the built web assets are compressed (via the main Makefile) and statically compiled into the Prometheus binary using Go's `embed` package.

During development it can be convenient to tell the Prometheus server to always serve its web assets from the local filesystem (in the `web/ui/static` build output directory) without having to recompile the Go binary. To make this work, remove the `builtinassets` build tag in the `flags` entry in `.promu.yml`, and then run `make build` (or build Prometheus using `go build ./cmd/prometheus`).

Note that in most cases, it is even more convenient to just use the development web server via `npm start` as mentioned above, since serving web assets like this from the filesystem still requires rebuilding those assets via `make ui-build` (or `npm run build`) before they can be served.

### Using prebuilt UI assets

If you are only working on the Prometheus Go backend and don't want to bother with the dependencies or the time required for producing UI builds, you can use the prebuilt web UI assets available with each Prometheus release (`prometheus-web-ui-<version>.tar.gz`). This allows you to skip building the UI from source.

1. Download and extract the prebuilt UI tarball:
   ```bash
   tar -xvf prometheus-web-ui-<version>.tar.gz -C web/ui
   ```

2. Build Prometheus using the prebuilt assets by passing the following parameter
   to `make`:
   ```bash
   make PREBUILT_ASSETS_STATIC_DIR=web/ui/static build
   ```

This will include the prebuilt UI files directly in the Prometheus binary, avoiding the need to install npm or rebuild the frontend from source.
