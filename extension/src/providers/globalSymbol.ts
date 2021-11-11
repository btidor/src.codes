import * as vscode from 'vscode';
import SymbolsClient from '../clients/symbols';
import { scoreFuzzy } from '../fuzzyScorer';

export default class GlobalSymbolProvider implements vscode.WorkspaceSymbolProvider {
    private symbolsClient: SymbolsClient;

    constructor(symbolsClient: SymbolsClient) {
        this.symbolsClient = symbolsClient;
    }

    provideWorkspaceSymbols(query: string, _token: vscode.CancellationToken): vscode.ProviderResult<vscode.SymbolInformation[]> {
        return this.symbolsClient.listGlobalSymbols().then(syms => {
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
    }
}
