import * as vscode from 'vscode';
import axios from 'axios';

import { Config, constructUri } from '../types/common';

export default class GrepClient {
    private config: Config;

    constructor(config: Config) {
        this.config = config;
    }

    query(q: string): Thenable<vscode.TextSearchResult[]> {
        // vscode.Uri doesn't quite get escaping right, so do it manually
        const params = new URLSearchParams({ q });
        const url = vscode.Uri.joinPath(this.config.grep, this.config.distribution)
            .toString(false) + '?' + params.toString();

        return axios
            .get(url, { responseType: 'text' })
            .then(res => {
                const results: vscode.TextSearchResult[] = [];
                for (const line of res.data.split("\n")) {
                    if (!line) {
                        // Stop at the first blank line, since there's debugging
                        // information in a footer which we need to skip.
                        break;
                    }
                    const parts = line.split(":");
                    let uri = constructUri(this.config, parts[0]);
                    // Server 1-indexes lines, we 0-index
                    let lineNo = parseInt(parts[1]) - 1;
                    let text = parts.slice(2).join(":"); // TODO: strip whitespace
                    let ranges: vscode.Range[] = [];
                    let matches: vscode.Range[] = [];
                    let re = new RegExp(q, "g");
                    for (const match of text.matchAll(re)) {
                        // TODO: support multi-line matching
                        ranges.push(new vscode.Range(
                            lineNo, match.index,
                            lineNo, match.index + match[0].length,
                        ));
                        matches.push(new vscode.Range(
                            0, match.index,
                            0, match.index + match[0].length,
                        ));
                    }
                    results.push({
                        uri,
                        ranges,
                        preview: { text, matches },
                    });
                }
                return results;
            });

    }
}
