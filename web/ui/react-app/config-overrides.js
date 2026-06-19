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

module.exports = function override(config) {
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
};
