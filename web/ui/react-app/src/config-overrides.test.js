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

const overrides = require('../config-overrides');

describe('development server compatibility', () => {
  const originalSetImmediate = global.setImmediate;
  afterEach(() => {
    if (originalSetImmediate === undefined) {
      delete global.setImmediate;
    } else {
      global.setImmediate = originalSetImmediate;
    }
  });

  it.each([
    [false, 'http'],
    [true, { type: 'https', options: {} }],
    [
      { key: 'key', cert: 'cert' },
      { type: 'https', options: { key: 'key', cert: 'cert' } },
    ],
  ])('preserves HTTPS configuration %p', (https, expected) => {
    const config = overrides.devServer(() => ({ https }))();
    expect(config.server).toEqual(expected);
    expect(config).not.toHaveProperty('https');
    expect(config).not.toHaveProperty('onBeforeSetupMiddleware');
    expect(config).not.toHaveProperty('onAfterSetupMiddleware');
  });

  it('runs CRA middleware before and after the development server middleware', async () => {
    const calls = [];
    global.setImmediate = require('timers').setImmediate;
    const middleware = (name) => (req, res, next) => {
      calls.push(name);
      next();
    };
    const config = overrides.devServer(() => ({
      onBeforeSetupMiddleware: ({ app }) => app.use(middleware('before')),
      onAfterSetupMiddleware: ({ app }) => app.use(middleware('after')),
    }))();
    for (const handler of config.setupMiddlewares([middleware('webpack')], {})) {
      await new Promise((resolve, reject) => {
        handler({ method: 'GET', url: '/' }, {}, (err) => (err ? reject(err) : resolve()));
      });
    }
    expect(calls).toEqual(['before', 'webpack', 'after']);
  });
});

it('transforms ESM dependencies without disabling CSS mocks', () => {
  const config = overrides.jest({ transformIgnorePatterns: ['node_modules'], moduleNameMapper: {} });
  const ignored = (file) => config.transformIgnorePatterns.some((pattern) => new RegExp(pattern).test(file));
  expect(ignored('/node_modules/.pnpm/htmlparser2@12.0.0/node_modules/htmlparser2/dist/index.js')).toBe(false);
  expect(
    ignored('/node_modules/.pnpm/@marijn+find-cluster-break@1.0.4/node_modules/@marijn/find-cluster-break/src/index.js')
  ).toBe(false);
  expect(ignored('/node_modules/react/index.js')).toBe(true);
  expect(ignored('/node_modules/bootstrap/dist/css/bootstrap.css')).toBe(false);
});
