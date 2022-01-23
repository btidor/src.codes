import * as vscode from 'vscode';

import { Config, constructUri } from '../types/common';
import HTTPClient from './http';

export default class GrepClient {
    private config: Config;

    constructor(config: Config) {
        this.config = config;
    }

    query(q: string, flags: string, includes: string[], excludes: string[], progress: vscode.Progress<vscode.TextSearchResult>, token: vscode.CancellationToken): Thenable<void> {
        const re = new RegExp(q, "gum" + flags);
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
                const parts = line.split(" ");
                const subparts = parts[0].split(":");
                const uri = constructUri(this.config, subparts[0]);
                const lineNo = parseInt(subparts[1]) - 1;
                const text = JSON.parse(parts.slice(1).join(" "));
                let ranges: vscode.Range[] = [];
                let matches: vscode.Range[] = [];
                for (const match of text.matchAll(re)) {
                    // TODO: inconsistent greediness
                    const sublines = match[0].split('\n');
                    const last = sublines[sublines.length - 1];
                    let lastCol = last.length;
                    if (sublines.length == 1) {
                        lastCol += match.index!;
                    }
                    ranges.push(new vscode.Range(
                        lineNo, match.index!,
                        lineNo + sublines.length - 1, lastCol,
                    ));
                    matches.push(new vscode.Range(
                        0, match.index!,
                        sublines.length - 1, lastCol,
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
