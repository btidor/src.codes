import * as vscode from 'vscode';

import SourceCodesFilesystem from './filesystem';
import SourceCodesFileSearchProvider from './filesearch';
import SourceCodesDefinitionProvider from './definition';

const FS_SCHEME = 'srccodes';
const DISTRIBUTION = 'impish';

export function activate(context: vscode.ExtensionContext) {
	console.warn("Hello from srccodes!");

	context.subscriptions.push(
		vscode.workspace.registerFileSystemProvider(
			FS_SCHEME, new SourceCodesFilesystem(DISTRIBUTION), { isCaseSensitive: true, isReadonly: true },
		),
	);

	context.subscriptions.push(
		vscode.workspace.registerFileSearchProvider(
			FS_SCHEME, new SourceCodesFileSearchProvider(DISTRIBUTION),
		),
	);

	context.subscriptions.push(
		vscode.languages.registerDefinitionProvider(
			{ scheme: FS_SCHEME }, new SourceCodesDefinitionProvider(DISTRIBUTION),
		)
	);

	context.subscriptions.push(
		vscode.commands.registerCommand('src-codes-explore', _ => {
			vscode.commands.executeCommand(
				'vscode.openFolder', vscode.Uri.from({
					scheme: FS_SCHEME,
					path: '/' + DISTRIBUTION,
				}),
			);
		}),
	);
}

export function deactivate() { }
