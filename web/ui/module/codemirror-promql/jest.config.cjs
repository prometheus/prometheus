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
    resolver: './jestResolver.cjs',
    transformIgnorePatterns: ["<rootDir>/../../node_modules/(?!@prometheus-io/lezer-promql)/"]
};
