import * as vscode from 'vscode';

import { Config, constructUri } from '../types/common';
import HTTPClient from './http';

export default class GrepClient {
    private config: Config;

    constructor(config: Config) {
        this.config = config;
    }

    query(q: string, flags: string, progress: vscode.Progress<vscode.TextSearchResult>): Thenable<void> {
        // vscode.Uri doesn't quite get escaping right, so do it manually
        const params = new URLSearchParams({ q, flags });
        const url = vscode.Uri.joinPath(this.config.grep, this.config.distribution)
            .toString(false) + '?' + params.toString();
        const re = new RegExp(q, "g"); // TODO: adjust flags

        return HTTPClient.streamingFetch(url, line => {
            const parts = line.split(":");
            let uri = constructUri(this.config, parts[0]);
            // The server 1-indexes lines, we 0-index...
            let lineNo = parseInt(parts[1]) - 1;
            let text = parts.slice(2).join(":");
            let ranges: vscode.Range[] = [];
            let matches: vscode.Range[] = [];
            for (const match of text.matchAll(re)) {
                // TODO: support multi-line matching
                ranges.push(new vscode.Range(
                    lineNo, match.index!,
                    lineNo, match.index! + match[0].length,
                ));
                matches.push(new vscode.Range(
                    0, match.index!,
                    0, match.index! + match[0].length,
                ));
            }
            progress.report({
                uri,
                ranges,
                preview: { text, matches },
            });
        });
    }
}
