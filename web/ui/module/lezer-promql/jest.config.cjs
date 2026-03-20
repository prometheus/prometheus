/** @type {import('ts-jest/dist/types').InitialOptionsTsJest} */
module.exports = {
    preset: 'ts-jest',
    extensionsToTreatAsEsm: ['.ts'],
    testEnvironment: 'node',
    globals: {
        'ts-jest': {
            useESM: true,
        },
    },
};
