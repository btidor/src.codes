import * as vscode from 'vscode';
import GrepClient from '../clients/grep';

export default class TextSearchProvider implements vscode.TextSearchProvider {
    private grepClient: GrepClient;

    constructor(grepClient: GrepClient) {
        this.grepClient = grepClient;
    }

    provideTextSearchResults(query: vscode.TextSearchQuery, options: vscode.TextSearchOptions, progress: vscode.Progress<vscode.TextSearchResult>, token: vscode.CancellationToken): vscode.ProviderResult<vscode.TextSearchComplete> {
        // TODO: support query options
        return this.grepClient.query(query.pattern).then(results => {
            for (const result of results) {
                // TODO: report progress incrementally
                progress.report(result);
            }
            return {}; // TODO
        });
    }
}
