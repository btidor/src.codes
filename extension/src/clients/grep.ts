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

        let currentFile: vscode.Uri | null = null;
        let contextLines = new Map();
        let matchLines = new Set();

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

                // Flush context lines when we reach the end of a file.
                if (uri.toString() != currentFile?.toString()) {
                    for (const [lineNumber, text] of contextLines) {
                        if (matchLines.has(lineNumber)) continue;
                        progress.report({ uri: currentFile!, text, lineNumber });
                    }
                    currentFile = uri;
                    contextLines.clear();
                    matchLines.clear();
                }

                // Queue up context (lines before and after match). Line numbers
                // are 1-indexed.
                for (const [i, line] of lines.entries()) {
                    const lineNumber = contextStart + i;
                    if (lineNumber < startLine || lineNumber > endLine) {
                        contextLines.set(lineNumber, line);
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

                // Any lines reported as part of a match must be *removed* from
                // the context, otherwise they'll be duplicated in the output.
                for (let i = startLine; i <= endLine; i++) {
                    matchLines.add(i);
                }
            }, token).then(() => {
                if (currentFile) {
                    for (const [lineNumber, text] of contextLines) {
                        if (matchLines.has(lineNumber)) continue;
                        progress.report({ uri: currentFile!, text, lineNumber });
                    }
                }
            });
    }
}
