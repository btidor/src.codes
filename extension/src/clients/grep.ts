import * as vscode from 'vscode';

import { Config, constructUri } from '../types/common';
import HTTPClient from './http';

export default class GrepClient {
    private config: Config;

    constructor(config: Config) {
        this.config = config;
    }

    query(q: string, flags: string, includes: string[], excludes: string[], progress: vscode.Progress<vscode.TextSearchResult>, token: vscode.CancellationToken): Thenable<void> {
        const re = new RegExp(q, "g" + flags);
        let params = new URLSearchParams({ q, flags });
        for (const i of includes) {
            params.append("include", i);
        }
        for (const x of excludes) {
            params.append("exclude", x);
        }

        return HTTPClient.streamingFetch(
            this.config.grep, this.config.distribution, params.toString(),
            line => {
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
            }, token);
    }
}
