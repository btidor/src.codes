import * as vscode from 'vscode';
import axios from 'axios';

import { Config, constructUri } from '../types/common';

export default class GrepClient {
    private config: Config;

    constructor(config: Config) {
        this.config = config;
    }

    query(q: string, flags: string, progress: vscode.Progress<vscode.TextSearchResult>): Thenable<boolean> {
        // vscode.Uri doesn't quite get escaping right, so do it manually
        const params = new URLSearchParams({ q, flags });
        const url = vscode.Uri.joinPath(this.config.grep, this.config.distribution)
            .toString(false) + '?' + params.toString();
        const re = new RegExp(q, "g"); // TODO: adjust flags

        return axios
            .get(url, { responseType: 'stream' })
            .then(res => {
                let line = "";
                res.data.on('data', (chunk: ArrayBuffer) => {
                    let arr = new Uint8Array(chunk);
                    for (let i = 0; i < arr.length; i++) {
                        line += String.fromCharCode(arr[i]);
                        if (arr[i] == 10) {
                            if (line == "") {
                                // Stop at the first blank line, since there's
                                // debugging information in a footer which we
                                // need to skip.
                                break;
                            }
                            const result = this.processResult(re, line);
                            progress.report(result);
                            line = "";
                        }
                    }
                });
                return new Promise((resolve, _) => {
                    res.data.on('close', () => {
                        resolve(true);
                    });
                });
            });
    }

    private processResult(re: RegExp, line: string): vscode.TextSearchResult {
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
        return {
            uri,
            ranges,
            preview: { text, matches },
        };
    }
}
