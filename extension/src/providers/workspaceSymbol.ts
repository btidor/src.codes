import * as vscode from 'vscode';
import PackageClient from '../clients/package';
import SymbolsClient from '../clients/symbols';

export default class WorkspaceSymbolProvider implements vscode.WorkspaceSymbolProvider {
    private packageClient: PackageClient;
    private symbolsClient: SymbolsClient;

    constructor(packageClient: PackageClient, symbolsClient: SymbolsClient) {
        this.packageClient = packageClient;
        this.symbolsClient = symbolsClient;
    }

    provideWorkspaceSymbols(query: string, _token: vscode.CancellationToken): vscode.ProviderResult<vscode.SymbolInformation[]> {
        return this.symbolsClient.listGlobalSymbols().then(syms => {
            let results = [];
            for (const symbol of Object.keys(syms)) {
                if (symbol.indexOf(query) >= 0) {
                    for (const location of syms[symbol]) {
                        results.push(new vscode.SymbolInformation(
                            symbol,
                            vscode.SymbolKind.Method, // TODO
                            "", // TODO (?)
                            location,
                        ));
                    }
                }
            }
            return results;
        });
    }
}
