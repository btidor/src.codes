import * as vscode from 'vscode';

import { Config, constructUri } from '../types/common';
import HTTPClient from './http';

export default class GrepClient {
    private config: Config;
    private cache: Map<string, [string | undefined | null, boolean, vscode.TextSearchResult[]]>;

    constructor(config: Config) {
        this.config = config;
        this.cache = new Map();
    }

    query(q: string, flags: string, includes: string[], excludes: string[], context: number, progress: vscode.Progress<vscode.TextSearchResult>, token: vscode.CancellationToken): Thenable<[boolean, boolean]> {
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
        let contextLines = new Map<number, string>();
        let matchLines = new Set<number>();

        const cacheKey = JSON.stringify([q, flags, includes, excludes, context]);
        const existing = this.cache.get(cacheKey);
        let results: vscode.TextSearchResult[] = [];
        if (existing !== undefined) {
            results = existing[2];
            for (const result of results) {
                progress.report(result);
            }

            switch (existing[0]) {
                case undefined:
                    // Previous request crashed, try again
                    results.splice(0, results.length);
                    break;
                case null:
                    // Previous request completed fully
                    return Promise.resolve([existing[1], false]);
                default:
                    // Load next page of results
                    params.append("after", existing[0]);
            }
        } else {
            this.cache.set(cacheKey, [undefined, false, results]);
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

                // Flush context lines when we reach the end of a file.
                if (uri.toString() != currentFile?.toString()) {
                    for (const [lineNumber, text] of contextLines) {
                        if (matchLines.has(lineNumber)) continue;
                        const context = { uri: currentFile!, text, lineNumber };
                        progress.report(context);
                        results.push(context);
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
                const match = {
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
                };
                progress.report(match);
                results.push(match);

                // Any lines reported as part of a match must be *removed* from
                // the context, otherwise they'll be duplicated in the output.
                for (let i = startLine; i <= endLine; i++) {
                    matchLines.add(i);
                }
            }, token).then(footers => {
                if (currentFile) {
                    for (const [lineNumber, text] of contextLines) {
                        if (matchLines.has(lineNumber)) continue;
                        const context = { uri: currentFile!, text, lineNumber };
                        progress.report(context);
                        results.push(context);
                    }
                }

                const errors = footers.get("Errors:");
                if (errors !== undefined) {
                    console.warn("Grep error:", errors);
                }
                const hasErrors = (existing && existing[1]) || errors !== undefined;

                const resume = footers.get("Resume:");
                if (resume !== undefined) {
                    this.cache.set(cacheKey, [resume, hasErrors, results]);
                }

                return [hasErrors, resume !== undefined];
            });
    }
}
