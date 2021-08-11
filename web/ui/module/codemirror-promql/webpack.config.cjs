// eslint-disable-next-line @typescript-eslint/no-var-requires
const path = require('path');
// eslint-disable-next-line @typescript-eslint/no-var-requires
const HtmlWebpackPlugin = require('html-webpack-plugin');
// eslint-disable-next-line @typescript-eslint/no-var-requires
const { CleanWebpackPlugin } = require('clean-webpack-plugin');

module.exports = {
  mode: 'development',
  entry: path.join(__dirname, '/src/app/app.ts'),
  output: {
    filename: '[name].bundle.js',
    path: path.resolve(__dirname, 'dist'),
  },
  devtool: 'inline-source-map',
  module: {
    rules: [
      {
        test: /\.tsx?$/,
        loader: 'ts-loader',
        exclude: /node_modules/,
      },
    ],
  },
  plugins: [
    new CleanWebpackPlugin({ cleanStaleWebpackAssets: false }),
    new HtmlWebpackPlugin({
      hash: true,
      filename: 'index.html', //relative to root of the application
      path: path.resolve(__dirname, 'dist'),
      template: './src/app/app.html',
    }),
  ],
  resolve: {
    extensions: ['.tsx', '.ts', '.js'],
  },
  devServer: {
    contentBase: './dist',
  },
};
