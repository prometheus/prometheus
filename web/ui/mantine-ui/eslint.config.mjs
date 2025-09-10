import { fixupConfigRules } from '@eslint/compat';
import reactRefresh from 'eslint-plugin-react-refresh';
import globals from 'globals';
import tsParser from '@typescript-eslint/parser';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import js from '@eslint/js';
import { FlatCompat } from '@eslint/eslintrc';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const compat = new FlatCompat({
    baseDirectory: __dirname,
    recommendedConfig: js.configs.recommended,
    allConfig: js.configs.all
});

export default [{
    ignores: ['**/dist', '**/.eslintrc.cjs'],
}, ...fixupConfigRules(compat.extends(
    'eslint:recommended',
    'plugin:@typescript-eslint/recommended',
    'plugin:react-hooks/recommended',
)), {
        plugins: {
            'react-refresh': reactRefresh,
        },

        languageOptions: {
            globals: {
                ...globals.browser,
            },

            parser: tsParser,
        },

        rules: {
            'react-refresh/only-export-components': ['warn', {
                allowConstantExport: true,
            }],

            // Disable the base rule as it can report incorrect errors
            'no-unused-vars': 'off',

            // Use the TypeScript-specific rule for unused vars
            '@typescript-eslint/no-unused-vars': ['warn', {
                argsIgnorePattern: '^_',
                varsIgnorePattern: '^_',
                caughtErrorsIgnorePattern: '^_',
            }],

            'prefer-const': ['error', {
                destructuring: 'all',
            }],
        },
    },
    // Override for Node.js-based config files
    {
        files: ['postcss.config.cjs'],  // Specify any other config files
        languageOptions: {
            ecmaVersion: 2021,  // Optional, set ECMAScript version
            sourceType: 'script',  // For CommonJS (non-ESM) modules
            globals: {
              module: 'readonly',
              require: 'readonly',
              process: 'readonly',
              __dirname: 'readonly',  // Include other Node.js globals if needed
            },
        },
    },
];
