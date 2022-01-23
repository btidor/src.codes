import * as vscode from 'vscode';

import { Config, constructUri } from '../types/common';
import HTTPClient from './http';

export default class GrepClient {
    private config: Config;

    constructor(config: Config) {
        this.config = config;
    }

    query(q: string, flags: string, includes: string[], excludes: string[], context: number, progress: vscode.Progress<vscode.TextSearchResult>, token: vscode.CancellationToken): Thenable<void> {
        const re = new RegExp(q, "gum" + flags);
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
                const subparts = parts[0].split(":");
                const uri = constructUri(this.config, subparts[0]);
                const text = JSON.parse(parts.slice(1).join(" "));
                let lineNo = parseInt(subparts[1]);

                // TODO: inconsistent greediness
                const matches = text.matchAll(re);
                if (matches === null) {
                    console.warn("Could not match result", text, re);
                    return;
                }
                const match = [...matches][0];

                // Report before-context and advance line number
                const bsplit = text.lastIndexOf('\n', match.index - 1);
                let asplit = text.indexOf('\n', match.index + match[0].length);
                if (asplit < 0) {
                    asplit = text.length;
                }
                if (bsplit > 0) {
                    const before = text.slice(0, bsplit).split('\n');
                    for (const line of before) {
                        progress.report({
                            uri, text: line, lineNumber: lineNo,
                        });
                        lineNo++;
                    }
                }

                // Report match, advance line no. for internal matches
                const inner = text.slice(bsplit + 1, asplit).split('\n');
                const startCol = match.index! - text.lastIndexOf('\n', match.index!) - 1;
                const endCol = (match.index! + match[0].length - 1) -
                    text.lastIndexOf('\n', match.index! + match[0].length - 1);
                progress.report({
                    uri,
                    ranges: [new vscode.Range(
                        lineNo - 1, startCol,
                        lineNo + inner.length - 2, endCol,
                    )],
                    preview: {
                        text: inner.join('\n'),
                        matches: [new vscode.Range(
                            0, startCol,
                            inner.length - 1, endCol,
                        )]
                    },
                });
                lineNo += inner.length;

                // Report after-context
                if (asplit < text.length) {
                    let after = text.slice(asplit + 1).split('\n');
                    if (after.slice(-1)[0].length == 0) {
                        after = after.slice(0, -1);
                    }
                    for (const line of after) {
                        progress.report({
                            uri, text: line, lineNumber: lineNo,
                        });
                        lineNo++;
                    }
                }
            }, token);
    }
}
