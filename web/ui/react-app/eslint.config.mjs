// Copyright The Prometheus Authors
// The rule configuration is derived from eslint-config-react-app 7.0.1.
/*
MIT License

Copyright (c) 2013-present, Facebook, Inc.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

import { fixupPluginRules } from '@eslint/compat';
import tsParser from '@typescript-eslint/parser';
import tsPlugin from '@typescript-eslint/eslint-plugin';
import restrictedGlobals from 'confusing-browser-globals';
import importPlugin from 'eslint-plugin-import';
import jsxA11y from 'eslint-plugin-jsx-a11y';
import prettier from 'eslint-plugin-prettier/recommended';
import react from 'eslint-plugin-react';
import reactHooks from 'eslint-plugin-react-hooks';
import globals from 'globals';

export default [
  { ignores: ['**/node_modules/**', 'build/**', 'src/vendor/**'] },
  {
    files: ['**/*.{js,mjs,cjs,jsx,ts,tsx}'],
    // Preserve the legacy app's policy for existing eslint-disable comments.
    linterOptions: { reportUnusedDisableDirectives: false },
    languageOptions: {
      parser: tsParser,
      ecmaVersion: 2018,
      sourceType: 'module',
      parserOptions: { ecmaFeatures: { jsx: true } },
      globals: { ...globals.browser, ...globals.commonjs, ...globals.es2015, ...globals.jest, ...globals.node },
    },
    plugins: {
      react: fixupPluginRules(react),
      import: fixupPluginRules(importPlugin),
      'jsx-a11y': fixupPluginRules(jsxA11y),
      'react-hooks': reactHooks,
      '@typescript-eslint': tsPlugin,
    },
    settings: { react: { version: 'detect' } },
    rules: {
      'no-restricted-globals': ['error', ...restrictedGlobals],
      'react/jsx-uses-vars': 'warn',
      'react/jsx-uses-react': 'warn',
      'array-callback-return': 'warn',
      'default-case': [
        'warn',
        {
          commentPattern: '^no default$',
        },
      ],
      'dot-location': ['warn', 'property'],
      eqeqeq: ['warn', 'smart'],
      'new-parens': 'warn',
      'no-array-constructor': 'warn',
      'no-caller': 'warn',
      'no-cond-assign': ['warn', 'except-parens'],
      'no-const-assign': 'warn',
      'no-control-regex': 'warn',
      'no-delete-var': 'warn',
      'no-dupe-args': 'warn',
      'no-dupe-class-members': 'warn',
      'no-dupe-keys': 'warn',
      'no-duplicate-case': 'warn',
      'no-empty-character-class': 'warn',
      'no-empty-pattern': 'warn',
      'no-eval': 'warn',
      'no-ex-assign': 'warn',
      'no-extend-native': 'warn',
      'no-extra-bind': 'warn',
      'no-extra-label': 'warn',
      'no-fallthrough': 'warn',
      'no-func-assign': 'warn',
      'no-implied-eval': 'warn',
      'no-invalid-regexp': 'warn',
      'no-iterator': 'warn',
      'no-label-var': 'warn',
      'no-labels': [
        'warn',
        {
          allowLoop: true,
          allowSwitch: false,
        },
      ],
      'no-lone-blocks': 'warn',
      'no-loop-func': 'warn',
      'no-mixed-operators': [
        'warn',
        {
          groups: [
            ['&', '|', '^', '~', '<<', '>>', '>>>'],
            ['==', '!=', '===', '!==', '>', '>=', '<', '<='],
            ['&&', '||'],
            ['in', 'instanceof'],
          ],
          allowSamePrecedence: false,
        },
      ],
      'no-multi-str': 'warn',
      'no-global-assign': 'warn',
      'no-unsafe-negation': 'warn',
      'no-new-func': 'warn',
      'no-new-object': 'warn',
      'no-new-symbol': 'warn',
      'no-new-wrappers': 'warn',
      'no-obj-calls': 'warn',
      'no-octal': 'warn',
      'no-octal-escape': 'warn',
      'no-redeclare': 'warn',
      'no-regex-spaces': 'warn',
      'no-restricted-syntax': ['warn', 'WithStatement'],
      'no-script-url': 'warn',
      'no-self-assign': 'warn',
      'no-self-compare': 'warn',
      'no-sequences': 'warn',
      'no-shadow-restricted-names': 'warn',
      'no-sparse-arrays': 'warn',
      'no-template-curly-in-string': 'warn',
      'no-this-before-super': 'warn',
      'no-throw-literal': 'warn',
      'no-undef': 'error',
      'no-unreachable': 'warn',
      'no-unused-expressions': [
        'error',
        {
          allowShortCircuit: true,
          allowTernary: true,
          allowTaggedTemplates: true,
        },
      ],
      'no-unused-labels': 'warn',
      'no-unused-vars': [
        'warn',
        {
          args: 'none',
          ignoreRestSiblings: true,
          caughtErrors: 'none',
        },
      ],
      'no-use-before-define': [
        'warn',
        {
          functions: false,
          classes: false,
          variables: false,
        },
      ],
      'no-useless-computed-key': 'warn',
      'no-useless-concat': 'warn',
      'no-useless-constructor': 'warn',
      'no-useless-escape': 'warn',
      'no-useless-rename': [
        'warn',
        {
          ignoreDestructuring: false,
          ignoreImport: false,
          ignoreExport: false,
        },
      ],
      'no-with': 'warn',
      'no-whitespace-before-property': 'warn',
      'react-hooks/exhaustive-deps': 'warn',
      'require-yield': 'warn',
      'rest-spread-spacing': ['warn', 'never'],
      strict: ['warn', 'never'],
      'unicode-bom': ['warn', 'never'],
      'use-isnan': 'warn',
      'valid-typeof': 'warn',
      'no-restricted-properties': [
        'error',
        {
          object: 'require',
          property: 'ensure',
          message: 'Please use import() instead. More info: https://facebook.github.io/create-react-app/docs/code-splitting',
        },
        {
          object: 'System',
          property: 'import',
          message: 'Please use import() instead. More info: https://facebook.github.io/create-react-app/docs/code-splitting',
        },
      ],
      'getter-return': 'warn',
      'import/first': 'error',
      'import/no-amd': 'error',
      'import/no-anonymous-default-export': 'warn',
      'import/no-webpack-loader-syntax': 'error',
      'react/forbid-foreign-prop-types': [
        'warn',
        {
          allowInPropTypes: true,
        },
      ],
      'react/jsx-no-comment-textnodes': 'warn',
      'react/jsx-no-duplicate-props': 'warn',
      'react/jsx-no-target-blank': 'warn',
      'react/jsx-no-undef': 'error',
      'react/jsx-pascal-case': [
        'warn',
        {
          allowAllCaps: true,
          ignore: [],
        },
      ],
      'react/no-danger-with-children': 'warn',
      'react/no-direct-mutation-state': 'warn',
      'react/no-is-mounted': 'warn',
      'react/no-typos': 'error',
      'react/require-render-return': 'error',
      'react/style-prop-object': 'warn',
      'jsx-a11y/alt-text': 'warn',
      'jsx-a11y/anchor-has-content': 'warn',
      'jsx-a11y/anchor-is-valid': [
        'warn',
        {
          aspects: ['noHref', 'invalidHref'],
        },
      ],
      'jsx-a11y/aria-activedescendant-has-tabindex': 'warn',
      'jsx-a11y/aria-props': 'warn',
      'jsx-a11y/aria-proptypes': 'warn',
      'jsx-a11y/aria-role': [
        'warn',
        {
          ignoreNonDOM: true,
        },
      ],
      'jsx-a11y/aria-unsupported-elements': 'warn',
      'jsx-a11y/heading-has-content': 'warn',
      'jsx-a11y/iframe-has-title': 'warn',
      'jsx-a11y/img-redundant-alt': 'warn',
      'jsx-a11y/no-access-key': 'warn',
      'jsx-a11y/no-distracting-elements': 'warn',
      'jsx-a11y/no-redundant-roles': 'warn',
      'jsx-a11y/role-has-required-aria-props': 'warn',
      'jsx-a11y/role-supports-aria-props': 'warn',
      'jsx-a11y/scope': 'warn',
      'react-hooks/rules-of-hooks': 'error',
    },
  },
  {
    files: ['**/*.{ts,tsx}'],
    // TypeScript rules replace their core equivalents without requiring type-aware linting.
    rules: {
      'default-case': 'off',
      'no-dupe-class-members': 'off',
      'no-undef': 'off',
      '@typescript-eslint/consistent-type-assertions': 'warn',
      'no-array-constructor': 'off',
      '@typescript-eslint/no-array-constructor': 'warn',
      'no-redeclare': 'off',
      '@typescript-eslint/no-redeclare': 'warn',
      'no-use-before-define': 'off',
      '@typescript-eslint/no-use-before-define': [
        'warn',
        {
          functions: false,
          classes: false,
          variables: false,
          typedefs: false,
        },
      ],
      'no-unused-expressions': 'off',
      '@typescript-eslint/no-unused-expressions': [
        'error',
        {
          allowShortCircuit: true,
          allowTernary: true,
          allowTaggedTemplates: true,
        },
      ],
      'no-unused-vars': 'off',
      '@typescript-eslint/no-unused-vars': [
        'warn',
        {
          args: 'none',
          ignoreRestSiblings: true,
          caughtErrors: 'none',
        },
      ],
      'no-useless-constructor': 'off',
      '@typescript-eslint/no-useless-constructor': 'warn',
    },
  },
  prettier,
  {
    rules: {
      '@typescript-eslint/explicit-function-return-type': ['off'],
      'eol-last': ['error', 'always'],
      'object-curly-spacing': ['error', 'always'],
      'prefer-const': 'warn',
      'comma-dangle': [
        'error',
        {
          arrays: 'always-multiline',
          objects: 'always-multiline',
          imports: 'always-multiline',
        },
      ],
    },
  },
];
