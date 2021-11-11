import * as vscode from 'vscode';
import PackageClient from '../clients/package';
import SymbolsClient from '../clients/symbols';

export default class GlobalDefinitionProvider implements vscode.DefinitionProvider {
    private symbolsClient: SymbolsClient;

    constructor(symbolsClient: SymbolsClient) {
        this.symbolsClient = symbolsClient;
    }

    provideDefinition(document: vscode.TextDocument, position: vscode.Position, _token: vscode.CancellationToken): vscode.ProviderResult<vscode.Definition | vscode.LocationLink[]> {
        const wordRange = document.getWordRangeAtPosition(position);
        if (wordRange) {
            const word = document.getText(wordRange);
            return this.symbolsClient.listGlobalSymbols().then(
                syms => syms[word].map(info => info.location)
            );
        } else {
            throw new Error("Could not find word at given position");
        }
    }
}
