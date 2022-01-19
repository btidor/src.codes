import * as vscode from 'vscode';

import FileSystemProvider from './providers/fileSystem';
import FileSearchProvider from './providers/fileSearch';
import LocalDefinitionProvider from './providers/localDefinition';
import GlobalDefinitionProvider from './providers/globalDefinition';
import PackageClient from './clients/package';
import FileClient from './clients/file';
import SymbolsClient from './clients/symbols';
import FzfClient from './clients/fzf';
import GlobalSymbolProvider from './providers/globalSymbol';
import LocalSymbolProvider from './providers/localSymbol';
import TextSearchProvider from './providers/textSearch';
import GrepClient from './clients/grep';

export function activate(context: vscode.ExtensionContext) {
	const config = {
		scheme: 'srccodes',
		distribution: 'impish',

		meta: vscode.Uri.parse('https://meta.src.codes'),
		ls: vscode.Uri.parse('https://ls.src.codes'),
		cat: vscode.Uri.parse('https://cat.src.codes'),
		fzf: vscode.Uri.parse('https://fzf.src.codes'),
		grep: vscode.Uri.parse('https://grep.src.codes'),
	};

	const packageClient = new PackageClient(config);
	const fileClient = new FileClient(config);
	const fzfClient = new FzfClient(config);
	const grepClient = new GrepClient(config);
	const symbolsClient = new SymbolsClient(config);

	context.subscriptions.push(
		vscode.workspace.registerFileSystemProvider(
			config.scheme, new FileSystemProvider(packageClient, fileClient), { isCaseSensitive: true, isReadonly: true },
		),
		vscode.workspace.registerFileSearchProvider(
			config.scheme, new FileSearchProvider(fzfClient),
		),
		vscode.workspace.registerTextSearchProvider(
			config.scheme, new TextSearchProvider(grepClient),
		),
		vscode.languages.registerDefinitionProvider(
			{ scheme: config.scheme }, new LocalDefinitionProvider(packageClient, symbolsClient),
		),
		vscode.languages.registerDefinitionProvider(
			{ scheme: config.scheme }, new GlobalDefinitionProvider(symbolsClient),
		),
		vscode.languages.registerWorkspaceSymbolProvider(
			new LocalSymbolProvider(packageClient, symbolsClient),
		),
		vscode.languages.registerWorkspaceSymbolProvider(
			new GlobalSymbolProvider(symbolsClient),
		),
		vscode.commands.registerCommand('src-codes-explore', _ => {
			vscode.commands.executeCommand(
				'vscode.openFolder', vscode.Uri.from({
					scheme: config.scheme,
					path: '/' + config.distribution,
				}),
			);
		}),
	);

	console.warn("Hello from srccodes!");
}

export function deactivate() { }
