import * as vscode from 'vscode';
import FzfClient from '../clients/fzf';

export default class FileSearchProvider implements vscode.FileSearchProvider {
    private fzfClient: FzfClient;

    constructor(fzfClient: FzfClient) {
        this.fzfClient = fzfClient;
    }

    provideFileSearchResults(query: vscode.FileSearchQuery, _options: vscode.FileSearchOptions, _token: vscode.CancellationToken): vscode.ProviderResult<vscode.Uri[]> {
        return this.fzfClient.query(query.pattern);
    }
}
