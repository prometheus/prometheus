var webpack = require('webpack');
var ExtractTextPlugin = require('extract-text-webpack-plugin');
var HtmlWebpackPlugin = require('html-webpack-plugin');
var CopyWebpackPlugin = require('copy-webpack-plugin');

var proxyURL = process.env.proxy || "http://localhost:8080/";
console.log("API requests are forwarded to " + proxyURL);

var webpackConfig = {
    entry: [
        __dirname + '/js/main.js',
        __dirname + '/css/style-loader.js'
    ],
    resolve: {
        modules: ['/node_modules']
    },
    module: {
        rules: [{
            test: /\.js$/,
            exclude: /node_modules/,
            use: 'babel-loader'
        }, {
            test: /\.mustache$/,
            use: 'mustache-loader'
        }, {
            test: /.scss$/,
            use: ExtractTextPlugin.extract({
              fallback: 'style-loader',
              use: 'css-loader?sourceMap!sass-loader'
            })
        }, {
            test: /\.woff2?$|\.ttf$|\.eot$|\.svg|\.png$/,
            // setting publicPath so that go-bindata can find font files 
            use: 'file-loader?name=./fonts/[name].[ext]'
        }]
    },
    output: {
        path: __dirname,
        filename: 'zipkin-ui.min.js'
        // 'publicPath' must not be set here in order to support Zipkin running in any context root.
        // '__webpack_public_path__' has to be set dynamically (see './publicPath.js' module file) as per
        // https://webpack.github.io/docs/configuration.html#output-publicpath
    },
    // devtool: 'source-map',
    plugins: [
        new webpack.ProvidePlugin({
            $: "jquery",
            jQuery: "jquery"
        }),
        new ExtractTextPlugin("zipkin-ui.min.css", {allChunks: true})
    ]
};

module.exports = webpackConfig;
