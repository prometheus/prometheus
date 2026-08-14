import { fixupConfigRules } from '@eslint/compat';
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
    ignores: ['**/dist', '**/.eslintrc.cjs', 'node_modules/**'],
}, ...fixupConfigRules(compat.extends(
    'eslint:recommended',
    'plugin:@typescript-eslint/recommended',
    'plugin:prettier/recommended',
)), {
        languageOptions: {
            parser: tsParser,
        },
        rules: {
            '@typescript-eslint/explicit-function-return-type': ['off'],
            'eol-last': [
                'error',
                'always',
            ],
            'object-curly-spacing': [
                'error',
                'always',
            ],
            'prefer-const': 'warn',
            'comma-dangle': [
                'error',
                {
                    'arrays': 'always-multiline',
                    'objects': 'always-multiline',
                    'imports': 'always-multiline',
                },
	    ],
        },
    },
];
