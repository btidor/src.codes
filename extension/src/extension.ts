import * as vscode from 'vscode';
import { DistroFS } from './distroFS';

export function activate(context: vscode.ExtensionContext) {
	console.log('Congratulations, your extension "distro-source-explorer" is now active!');

	context.subscriptions.push(
		vscode.workspace.registerFileSystemProvider(
			'distro', new DistroFS(), { isCaseSensitive: true, isReadonly: true },
		),
	);

	context.subscriptions.push(
		vscode.commands.registerCommand('distro-source-explorer.join', _ => {
			vscode.commands.executeCommand(
				'vscode.openFolder', vscode.Uri.parse('distro:/'),
			);
		}),
	);
}

export function deactivate() {}
