// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

const path = require('path');

// @prometheus-io/codemirror-promql is consumed via pnpm's "link:" protocol, so
// it is a symlink into the workspace and carries its own node_modules. Without
// deduplication, its transitive @codemirror/* and @lezer/* imports resolve to
// the workspace copy while react-app uses its own isolated copy. That loads two
// instances of @codemirror/state and breaks instanceof checks at runtime
// ("Unrecognized extension value in extension set"). Force these packages to
// resolve to react-app's single copy.
const singletons = [
  '@codemirror/state',
  '@codemirror/view',
  '@codemirror/language',
  '@codemirror/commands',
  '@codemirror/search',
  '@codemirror/autocomplete',
  '@codemirror/lint',
  '@lezer/common',
  '@lezer/highlight',
  '@lezer/lr',
];

function override(config) {
  config.resolve = config.resolve || {};
  config.resolve.alias = Object.assign(
    {},
    config.resolve.alias,
    Object.fromEntries(singletons.map((pkg) => [pkg, path.resolve(__dirname, 'node_modules', pkg)]))
  );
  // The aliases above resolve to absolute node_modules paths, which Create
  // React App's ModuleScopePlugin rejects as imports outside src/. Drop it so
  // the deduplicating aliases take effect.
  config.resolve.plugins = (config.resolve.plugins || []).filter(
    (plugin) => !(plugin.constructor && plugin.constructor.name === 'ModuleScopePlugin')
  );
  return config;
}

module.exports = {
  webpack: override,
  jest: (config) => {
    // Rewired concatenates arrays, so replace CRA's blanket dependency exclusion.
    config.transformIgnorePatterns = require('./package.json').jest.transformIgnorePatterns;
    // Match webpack's single CodeMirror instance for linked workspace packages.
    Object.assign(config.moduleNameMapper, Object.fromEntries(singletons.map((pkg) => [`^${pkg}$`, require.resolve(pkg)])));
    const requireFromScripts = require('module').createRequire(require.resolve('react-scripts/package.json'));
    config.moduleNameMapper['^.+\\.module\\.(css|sass|scss)$'] = requireFromScripts.resolve('identity-obj-proxy');
    return config;
  },
  devServer: (createConfig) => (proxy, allowedHost) => {
    const config = createConfig(proxy, allowedHost);
    const { https, onBeforeSetupMiddleware, onAfterSetupMiddleware } = config;
    delete config.https;
    delete config.onBeforeSetupMiddleware;
    delete config.onAfterSetupMiddleware;
    config.server = https ? { type: 'https', options: https === true ? {} : https } : 'http';

    // Preserve CRA's before/after middleware ordering with webpack-dev-server 5.
    const requireFromScripts = require('module').createRequire(require.resolve('react-scripts/package.json'));
    const express = requireFromScripts('express');
    config.setupMiddlewares = (middlewares, devServer) => {
      const before = express.Router();
      const after = express.Router();
      onBeforeSetupMiddleware({ ...devServer, app: before });
      onAfterSetupMiddleware({ ...devServer, app: after });
      return [before, ...middlewares, after];
    };
    return config;
  },
};
