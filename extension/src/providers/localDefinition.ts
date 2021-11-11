import * as vscode from 'vscode';
import PackageClient from '../clients/package';
import SymbolsClient from '../clients/symbols';

export default class LocalDefinitionProvider implements vscode.DefinitionProvider {
    private packageClient: PackageClient;
    private symbolsClient: SymbolsClient;

    constructor(packageClient: PackageClient, symbolsClient: SymbolsClient) {
        this.packageClient = packageClient;
        this.symbolsClient = symbolsClient;
    }

    provideDefinition(document: vscode.TextDocument, position: vscode.Position, _token: vscode.CancellationToken): vscode.ProviderResult<vscode.Definition | vscode.LocationLink[]> {
        const wordRange = document.getWordRangeAtPosition(position);
        if (wordRange) {
            const word = document.getText(wordRange);
            return this.packageClient.parseUri(document.uri).then(path => {
                if (path) {
                    return this.symbolsClient.listPackageSymbols(path.pkg).then(
                        syms => (syms.get(word) || []).map(info => info.location)
                    );
                } else {
                    throw new Error("Tried to provide definition in workspace root?!");
                }
            });
        } else {
            throw new Error("Could not find word at given position");
        }
    }
}
