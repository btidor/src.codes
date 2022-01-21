import * as vscode from 'vscode';
import GrepClient from '../clients/grep';

export default class TextSearchProvider implements vscode.TextSearchProvider {
    private grepClient: GrepClient;

    constructor(grepClient: GrepClient) {
        this.grepClient = grepClient;
    }

    provideTextSearchResults(query: vscode.TextSearchQuery, options: vscode.TextSearchOptions, progress: vscode.Progress<vscode.TextSearchResult>, token: vscode.CancellationToken): vscode.ProviderResult<vscode.TextSearchComplete> {
        // TODO: support includes, excludes & ignores
        // TODO: support beforeContext & afterContext
        let pattern = query.pattern;
        if (!query.isRegExp) {
            // https://developer.mozilla.org/en-US/docs/Web/JavaScript/Guide/Regular_Expressions#escaping
            pattern = pattern.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
        }

        let flags = "";
        if (!query.isCaseSensitive) flags += "i";
        if (query.isMultiline) flags += "ms"; // TODO: is this right
        if (query.isWordMatch) pattern = '\\b' + pattern + '\\b';

        return this.grepClient.query(pattern, flags, progress, token).then(_ => {
            return {}; // TODO
        });
    }
}
