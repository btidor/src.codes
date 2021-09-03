import * as vscode from 'vscode';

import SourceCodesFilesystem from './filesystem';

export function activate(context: vscode.ExtensionContext) {
	context.subscriptions.push(
		vscode.workspace.registerFileSystemProvider(
			'srccodes', new SourceCodesFilesystem('hirsute'), { isCaseSensitive: true, isReadonly: true },
		),
	);

	context.subscriptions.push(
		vscode.commands.registerCommand('src-codes-explore', _ => {
			vscode.commands.executeCommand(
				'vscode.openFolder', vscode.Uri.parse('srccodes:/hirsute (Ubuntu 21.04)'),
			);
		}),
	);
}

export function deactivate() { }
