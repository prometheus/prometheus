// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0.

const assert = require('node:assert/strict');
const fs = require('node:fs');
const { createRequire } = require('node:module');
const os = require('node:os');
const path = require('node:path');
const { test } = require('node:test');
const { ESLint } = require('eslint');
const ESLintPlugin = require('eslint-webpack-plugin');
const override = require('../config-overrides');

const app = path.resolve(__dirname, '..');
const cra = createRequire(require.resolve('react-scripts/package.json'));
const webpack = cra('webpack');
const configPath = cra.resolve('react-scripts/config/webpack.config');

test('flat configuration preserves the app lint rules', async (t) => {
  const eslint = new ESLint({ cwd: app });
  const cases = [
    { name: 'valid JSX', code: 'export const Good = () => <div />;\n', rule: null },
    {
      name: 'undefined component',
      code: 'export const Bad = () => <Missing />;\n',
      rule: 'react/jsx-no-undef',
      severity: 2,
    },
    {
      name: 'conditional hook',
      code: "import { useState } from 'react';\nexport function Bad({ flag }: { flag: boolean }) { if (flag) useState(0); return null; }\n",
      rule: 'react-hooks/rules-of-hooks',
      severity: 2,
    },
    {
      name: 'image description',
      code: 'export const Bad = () => <img src="image.png" />;\n',
      rule: 'jsx-a11y/alt-text',
      severity: 1,
    },
    {
      name: 'import order',
      code: "const value = 1;\nimport React from 'react';\nexport default value;\n",
      rule: 'import/first',
      severity: 2,
    },
    {
      name: 'unused local',
      code: 'export function value() { const unused = 1; return 2; }\n',
      rule: '@typescript-eslint/no-unused-vars',
      severity: 1,
    },
    { name: 'formatting', code: 'export const value=1;\n', rule: 'prettier/prettier', severity: 2 },
    { name: 'unused directive', code: '/* eslint-disable no-eval */\nexport const value = 1;\n', rule: null },
    {
      name: 'unused catch variable',
      code: "export function recover() {\n  try {\n    JSON.parse('{}');\n  } catch (error) {\n    return null;\n  }\n}\n",
      rule: null,
    },
  ];
  for (const fixture of cases) {
    await t.test(fixture.name, async () => {
      const [result] = await eslint.lintText(fixture.code, { filePath: path.join(app, 'src/lint-fixture.tsx') });
      if (fixture.rule) {
        assert.ok(
          result.messages.some((message) => message.ruleId === fixture.rule && message.severity === fixture.severity),
          JSON.stringify(result.messages)
        );
      } else {
        assert.deepEqual(result.messages, []);
      }
    });
  }
  const [javascript] = await eslint.lintText(cases.at(-1).code, { filePath: path.join(app, 'src/lint-fixture.js') });
  assert.deepEqual(javascript.messages, []);
  assert.equal(await eslint.isPathIgnored(path.join(app, 'src/vendor/jquery.js')), true);
});

// CRA reads these flags when its configuration module is loaded.
function createConfig(mode, flags = {}) {
  const values = {
    NODE_ENV: mode,
    BABEL_ENV: mode,
    ESLINT_NO_DEV_ERRORS: undefined,
    DISABLE_ESLINT_PLUGIN: undefined,
    DISABLE_NEW_JSX_TRANSFORM: undefined,
    ...flags,
  };
  const previous = Object.fromEntries(Object.keys(values).map((key) => [key, process.env[key]]));
  try {
    for (const [key, value] of Object.entries(values)) {
      if (value === undefined) delete process.env[key];
      else process.env[key] = value;
    }
    delete require.cache[configPath];
    return override(cra(configPath)(mode));
  } finally {
    for (const [key, value] of Object.entries(previous)) {
      if (value === undefined) delete process.env[key];
      else process.env[key] = value;
    }
  }
}

function createCompiler(mode, entry, output, flags, eslintOptions) {
  const config = createConfig(mode, flags);
  const plugins = config.plugins.filter((plugin) =>
    ['ESLintWebpackPlugin', 'ESLintWarningsPlugin'].includes(plugin.constructor.name)
  );
  const linters = plugins.filter((plugin) => plugin instanceof ESLintPlugin);
  assert.equal(linters.length, 1);
  Object.assign(linters[0].options, { cacheLocation: path.join(output, '.eslintcache') }, eslintOptions);
  return webpack({
    mode,
    context: app,
    entry,
    output: { path: output, filename: 'bundle.js' },
    optimization: { minimize: false },
    plugins,
  });
}

test('webpack preserves lint failures, warnings, and development options', { timeout: 30000 }, async (t) => {
  const input = fs.mkdtempSync(path.join(app, 'src/.eslint-test-'));
  const output = fs.mkdtempSync(path.join(os.tmpdir(), 'prometheus-eslint-'));
  t.after(() => {
    fs.rmSync(input, { recursive: true, force: true });
    fs.rmSync(output, { recursive: true, force: true });
  });
  const entry = path.join(input, 'entry.js');
  for (const mode of ['production', 'development']) {
    for (const fixture of [
      { name: 'valid', code: 'export const value = 1;\n', errors: false, warnings: false },
      { name: 'error', code: 'export const value = missing;\n', errors: true, warnings: false },
      {
        name: 'warning',
        code: 'export function value() {\n  const unused = 1;\n  return 2;\n}\n',
        errors: false,
        warnings: true,
      },
    ]) {
      await t.test(`${mode}: ${fixture.name}`, async () => {
        fs.writeFileSync(entry, fixture.code);
        const compiler = createCompiler(mode, entry, output);
        try {
          const { error, stats } = await new Promise((resolve) => compiler.run((error, stats) => resolve({ error, stats })));
          assert.equal(Boolean(error || stats.hasErrors()), fixture.errors);
          if (fixture.errors) assert.match(String(error || JSON.stringify(stats.toJson().errors)), /no-undef/);
          else assert.equal(stats.hasWarnings(), fixture.warnings);
        } finally {
          await new Promise((resolve, reject) => compiler.close((error) => (error ? reject(error) : resolve())));
        }
      });
    }
  }
  await t.test('development can downgrade lint errors without hiding configuration failures', async () => {
    fs.writeFileSync(entry, 'export const value = missing;\n');
    for (const invalidConfig of [false, true]) {
      const compiler = createCompiler(
        'development',
        entry,
        output,
        { ESLINT_NO_DEV_ERRORS: 'true' },
        invalidConfig ? { overrideConfigFile: path.join(input, 'missing.config.mjs') } : {}
      );
      try {
        const { error, stats } = await new Promise((resolve) => compiler.run((error, stats) => resolve({ error, stats })));
        assert.equal(Boolean(error || stats.hasErrors()), invalidConfig);
        if (!invalidConfig) {
          assert.equal(stats.hasWarnings(), true);
          assert.match(JSON.stringify(stats.toJson().warnings), /no-undef/);
        }
      } finally {
        await new Promise((resolve, reject) => compiler.close((error) => (error ? reject(error) : resolve())));
      }
    }
  });
  const disabled = createConfig('development', { DISABLE_ESLINT_PLUGIN: 'true' });
  assert.equal(
    disabled.plugins.some((plugin) => plugin instanceof ESLintPlugin),
    false
  );
  const classic = createConfig('production', { DISABLE_NEW_JSX_TRANSFORM: 'true' });
  const { overrideConfigFile, overrideConfig } = classic.plugins.find((plugin) => plugin instanceof ESLintPlugin).options;
  const eslint = new ESLint({ cwd: app, overrideConfigFile, overrideConfig });
  const [result] = await eslint.lintText('export const Component = () => <div />;\n', {
    filePath: path.join(app, 'src/lint-fixture.tsx'),
  });
  assert.ok(result.messages.some((message) => message.ruleId === 'react/react-in-jsx-scope' && message.severity === 2));
});

test('webpack watch reports and clears lint errors after edits', { timeout: 30000 }, async (t) => {
  const input = fs.mkdtempSync(path.join(app, 'src/.eslint-test-'));
  const output = fs.mkdtempSync(path.join(os.tmpdir(), 'prometheus-eslint-watch-'));
  t.after(() => {
    fs.rmSync(input, { recursive: true, force: true });
    fs.rmSync(output, { recursive: true, force: true });
  });
  const entry = path.join(input, 'entry.js');
  const sources = ['export const value = 1;\n', 'export const value = missing;\n', 'export const value = 2;\n'];
  fs.writeFileSync(entry, sources[0]);
  const compiler = createCompiler('development', entry, output);
  let watching;
  try {
    await new Promise((resolve, reject) => {
      let step = 0;
      watching = compiler.watch({ aggregateTimeout: 20, poll: 50 }, (error, stats) => {
        try {
          assert.equal(Boolean(error || stats.hasErrors()), step === 1);
          if (step === 1) assert.match(String(error || JSON.stringify(stats.toJson().errors)), /no-undef/);
          if (++step === sources.length) resolve();
          else setImmediate(() => fs.writeFileSync(entry, sources[step]));
        } catch (failure) {
          reject(failure);
        }
      });
      t.signal.addEventListener('abort', () => reject(t.signal.reason), { once: true });
    });
  } finally {
    if (watching) await new Promise((resolve, reject) => watching.close((error) => (error ? reject(error) : resolve())));
    await new Promise((resolve, reject) => compiler.close((error) => (error ? reject(error) : resolve())));
  }
});
