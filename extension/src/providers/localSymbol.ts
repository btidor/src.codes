import * as vscode from 'vscode';
import PackageClient from '../clients/package';
import SymbolsClient from '../clients/symbols';
import { scoreFuzzy } from '../fuzzyScorer';

export default class LocalSymbolProvider implements vscode.WorkspaceSymbolProvider {
    private packageClient: PackageClient;
    private symbolsClient: SymbolsClient;

    constructor(packageClient: PackageClient, symbolsClient: SymbolsClient) {
        this.packageClient = packageClient;
        this.symbolsClient = symbolsClient;
    }

    provideWorkspaceSymbols(query: string, _token: vscode.CancellationToken): vscode.ProviderResult<vscode.SymbolInformation[]> {
        return Promise.all(
            // Only search packages corresponding to "known" text documents (~= open
            // tabs) to avoid overwhelming the user with unrelated symbols.
            vscode.workspace.textDocuments.map(
                doc => this.packageClient.parseUri(doc.uri).then(path => path!.pkg)
            )
        ).then(pkgs => {
            return Promise.all(
                [...new Set(pkgs)].map(pkg => {
                    return this.symbolsClient.listPackageSymbols(pkg).then(syms => {
                        let matches: [number, string][] = [];
                        for (const symbol of syms.keys()) {
                            let score = scoreFuzzy(symbol, query, query.toLowerCase(), true);
                            if (score > 0) {
                                matches.push([score, symbol]);
                            }
                        }
                        matches.sort(([a, _x], [b, _y]) => a - b);
                        return matches.slice(0, 100).flatMap(
                            ([_, symbol]) => syms.get(symbol) || []
                        );
                    });
                })
            ).then(res => res.flat());
        });
    }
}
