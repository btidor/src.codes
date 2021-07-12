import * as vscode from 'vscode';
import { DebianFS, Definer } from './debianFS';

export function activate(context: vscode.ExtensionContext) {
	console.log('Congratulations, your extension "debian-source-explorer" is now active!');

	context.subscriptions.push(
		vscode.workspace.registerFileSystemProvider(
			'debian', new DebianFS(), { isCaseSensitive: true, isReadonly: true },
		),
	);

	context.subscriptions.push(
		vscode.commands.registerCommand('debian-source-explorer.join', _ => {
			vscode.commands.executeCommand(
				'vscode.openFolder', vscode.Uri.parse('debian:/buster/'),
			);
		}),
	);

	context.subscriptions.push(
		vscode.languages.registerDefinitionProvider(
			{ scheme: 'debian' },
			new Definer(),
		),
	);
}

export function deactivate() {}
