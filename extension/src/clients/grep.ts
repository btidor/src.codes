import * as vscode from 'vscode';

import { Config, constructUri } from '../types/common';
import HTTPClient from './http';

export default class GrepClient {
    private config: Config;

    constructor(config: Config) {
        this.config = config;
    }

    query(q: string, flags: string, includes: string[], excludes: string[], context: number, progress: vscode.Progress<vscode.TextSearchResult>, token: vscode.CancellationToken): Thenable<void> {
        let params = new URLSearchParams({ q, flags });
        for (const i of includes) {
            params.append("include", i);
        }
        for (const x of excludes) {
            params.append("exclude", x);
        }
        if (context > 0) {
            params.append("context", context.toString());
        }

        return HTTPClient.streamingFetch(
            this.config.grep, this.config.distribution, params.toString(),
            line => {
                const parts = line.split(" ");
                const uri = constructUri(this.config, parts[0]);
                const contextStart = parseInt(parts[1]);
                const beforeContext = parseInt(parts[2]);
                const afterContext = parseInt(parts[3]);
                const startCol = parseInt(parts[4]);
                const endCol = parseInt(parts[5]);
                const lines = JSON.parse(parts.slice(6).join(" ")).split('\n');
                const startLine = contextStart + beforeContext;
                const endLine = contextStart + lines.length - afterContext - 1;

                // Report context (lines before and after match). Line numbers
                // are 1-indexed.
                for (const [i, line] of lines.entries()) {
                    const lineNumber = contextStart + i;
                    if (lineNumber < startLine || lineNumber > endLine) {
                        progress.report({
                            uri, text: line, lineNumber: lineNumber,
                        });
                    }
                }

                // Report match. Line numbers and column numbers are 0-indexed.
                // Go figure.
                progress.report({
                    uri,
                    ranges: [new vscode.Range(
                        startLine - 1, startCol - 1,
                        endLine - 1, endCol - 1,
                    )],
                    preview: {
                        text: lines.join('\n'),
                        matches: [new vscode.Range(
                            beforeContext, startCol - 1,
                            lines.length - afterContext - 1, endCol - 1,
                        )],
                    },
                });
            }, token);
    }
}
