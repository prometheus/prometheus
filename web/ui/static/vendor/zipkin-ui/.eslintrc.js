module.exports = {
    extends: 'airbnb',
    plugins: [],
    parserOptions: {
      ecmaFeatures: {
        experimentalObjectRestSpread: true
      }
    },
    rules: {
      'comma-dangle': 'off',
      'func-names': 'off',
      'object-curly-spacing': [2, 'never'],
      'space-before-function-paren': [2, 'never'],
      eqeqeq: [2, 'allow-null'],
      'no-else-return': 'off'
    }
  };