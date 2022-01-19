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
                const matches: vscode.TextSearchResult[] = [];
                for (const line of res.data.split("\n")) {
                    if (!line) {
                        // Stop at the first blank line, since there's debugging
                        // information in a footer which we need to skip.
                        break;
                    }
                    const parts = line.split(":");
                    let uri = constructUri(this.config, parts[0]);
                    matches.push({
                        uri,
                        ranges: new vscode.Range(0, 0, 0, 1), // TODO
                        preview: {
                            text: 'TODO',
                            matches: new vscode.Range(0, 0, 0, 1),
                        },
                    });
                }
                return matches;
            });

    }
}
