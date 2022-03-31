/** @type {import('ts-jest/dist/types').InitialOptionsTsJest} */
module.exports = {
    preset: 'ts-jest',
    extensionsToTreatAsEsm: ['.ts'],
    testEnvironment: 'node',
    setupFiles: [
        './setupJest.cjs'
    ],
    globals: {
        'ts-jest': {
            useESM: true,
        },
    },
    moduleNameMapper: {
        'lezer-promql': '<rootDir>/../../node_modules/lezer-promql/dist/index.es.js'
    },
    transformIgnorePatterns: ["<rootDir>/../../node_modules/(?!lezer-promql)/"]
};
