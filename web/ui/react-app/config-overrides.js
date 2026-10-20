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
const ESLintPlugin = require('eslint-webpack-plugin');

class ESLintWarningsPlugin {
  constructor(formatter) {
    const format = typeof formatter === 'function' ? formatter : require(formatter);
    this.messages = new Set();
    this.format = (results) => {
      const message = format(results);
      this.messages.add(`[eslint] ${message}`);
      return message;
    };
  }

  apply(compiler) {
    compiler.hooks.thisCompilation.tap('ESLintWarningsPlugin', () => this.messages.clear());
    compiler.hooks.afterCompile.tap('ESLintWarningsPlugin', (compilation) => {
      compilation.errors = compilation.errors.filter((error) => {
        // Only downgrade formatted diagnostics, preserving configuration failures.
        if (error.name !== 'ESLintError' || !this.messages.has(error.message)) return true;
        compilation.warnings.push(error);
        return false;
      });
    });
  }
}

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
  // CRA's bundled plugin only supports legacy ESLint options. Preserve its
  // build behavior while selecting the app's ESLint and flat configuration.
  config.plugins = (config.plugins || []).flatMap((plugin) => {
    if (plugin.constructor.name !== 'ESLintWebpackPlugin') {
      return plugin;
    }
    const { extensions, formatter, failOnError, context, cache, cacheLocation, cwd, baseConfig } = plugin.options;
    const warnings = !failOnError && new ESLintWarningsPlugin(formatter);
    const eslint = new ESLintPlugin({
      extensions,
      formatter: warnings ? warnings.format : formatter,
      // Compilation errors still fail CRA builds. Throwing a fatal error here
      // would stop webpack's watcher before it can detect a corrected file.
      failOnError: false,
      context,
      cache,
      cacheLocation,
      cwd,
      eslintPath: require.resolve('eslint'),
      configType: 'flat',
      overrideConfigFile: path.resolve(__dirname, 'eslint.config.mjs'),
      // CRA requires React in scope when the classic JSX transform is selected.
      overrideConfig: { rules: baseConfig.rules },
    });
    // CRA's ESLINT_NO_DEV_ERRORS flag downgrades lint diagnostics, while
    // plugin v6's failOnError only controls whether to abort compilation.
    return warnings ? [eslint, warnings] : eslint;
  });
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

override.jest = function overrideJest(config) {
  config.moduleNameMapper = Object.assign(
    {},
    config.moduleNameMapper,
    Object.fromEntries(singletons.map((pkg) => [`^${pkg}$`, `<rootDir>/node_modules/${pkg}`]))
  );
  // Jest 27 does not honor conditional CommonJS exports for these dependencies,
  // so transpile their ESM entrypoints while leaving other node modules untouched.
  config.transformIgnorePatterns = [
    '<rootDir>/node_modules/(?!(@prometheus-io/(codemirror-promql|lezer-promql)|\\.pnpm/(@marijn\\+find-cluster-break|dom-serializer|domelementtype|domhandler|domutils|entities|htmlparser2)@)).+\\.(js|jsx|mjs|cjs|ts|tsx)$',
    '^.+\\.module\\.(css|sass|scss)$',
  ];
  return config;
};

module.exports = override;
