import * as vscode from 'vscode';
import GrepClient from '../clients/grep';

export default class TextSearchProvider implements vscode.TextSearchProvider {
    private grepClient: GrepClient;

    constructor(grepClient: GrepClient) {
        this.grepClient = grepClient;
    }

    provideTextSearchResults(query: vscode.TextSearchQuery, options: vscode.TextSearchOptions, progress: vscode.Progress<vscode.TextSearchResult>, token: vscode.CancellationToken): vscode.ProviderResult<vscode.TextSearchComplete> {
        let pattern = query.pattern;
        if (!query.isRegExp) {
            // https://developer.mozilla.org/en-US/docs/Web/JavaScript/Guide/Regular_Expressions#escaping
            pattern = pattern.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
        }

        let flags = "";
        if (!query.isCaseSensitive) flags += "i";
        if (query.isMultiline) flags += "s";
        if (query.isWordMatch) pattern = '\\b' + pattern + '\\b';


        let messages: vscode.TextSearchCompleteMessage[] = [];

        let context = 0;
        if (options.beforeContext || options.afterContext) {
            context = Math.max(options.beforeContext || 0, options.afterContext || 0);
            if (context > 10) {
                messages.push({
                    text: "TODO: context too large",
                    type: vscode.TextSearchCompleteMessageType.Warning,
                });
            }
        }


        return this.grepClient.query(
            pattern, flags, options.includes, options.excludes, context, progress, token,
        ).then(_ => {
            return { limitHit: false, message: messages };
        });
    }
}
