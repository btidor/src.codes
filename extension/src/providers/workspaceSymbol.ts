import * as vscode from 'vscode';
import PackageClient from '../clients/package';
import SymbolsClient from '../clients/symbols';
import { scoreFuzzy } from '../fuzzyScorer';

export default class WorkspaceSymbolProvider implements vscode.WorkspaceSymbolProvider {
    private packageClient: PackageClient;
    private symbolsClient: SymbolsClient;

    constructor(packageClient: PackageClient, symbolsClient: SymbolsClient) {
        this.packageClient = packageClient;
        this.symbolsClient = symbolsClient;
    }

    provideWorkspaceSymbols(query: string, _token: vscode.CancellationToken): vscode.ProviderResult<vscode.SymbolInformation[]> {
        return this.symbolsClient.listGlobalSymbols().then(syms => {
            let matches: [number, string][] = [];
            for (const symbol of Object.keys(syms)) {
                let score = scoreFuzzy(symbol, query, query.toLowerCase(), true);
                if (score > 0) {
                    matches.push([score, symbol]);
                }
            }
            matches.sort(([a, _x], [b, _y]) => a - b);
            return matches.slice(0, 100).flatMap(
                ([_, symbol]) => syms[symbol].map(
                    location => new vscode.SymbolInformation(
                        symbol,
                        vscode.SymbolKind.Method, // TODO
                        "",
                        location,
                    )
                )
            );
        });
    }
}
