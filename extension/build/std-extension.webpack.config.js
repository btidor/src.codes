//@ts-check

'use strict';

const path = require('path');

/**@type {import('webpack').Configuration}*/
const config = {
	context: path.dirname(__dirname),
	target: 'node',
	mode: 'none',

	entry: {
		'extension': './src/extension.ts',
	},
	resolve: {
		extensions: ['.ts', '.js']
	},
	module: {
		rules: [{
			test: /\.ts$/,
			exclude: /node_modules/,
			use: [{
				loader: 'ts-loader'
			}]
		}]
	},
	externals: {
		vscode: 'commonjs vscode'
	},
	output: {
		filename: '[name].js',
		path: path.resolve(__dirname, '../dist/std'),
		libraryTarget: 'commonjs2'
	},
	devtool: 'nosources-source-map'
};
module.exports = config;
