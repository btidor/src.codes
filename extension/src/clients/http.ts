import * as vscode from 'vscode';

import axios from 'axios';

export default class HTTPClient {
    static streamingFetch(base: vscode.Uri, path: string, params: string, callback: (line: string) => void, token: vscode.CancellationToken): Promise<void> {
        // vscode.Uri doesn't quite get escaping right, so do it manually
        const url = vscode.Uri.joinPath(base, path).toString(false) + '?' + params;
        console.warn(url);
        let line = '';
        let decoder = new TextDecoder('utf-8');
        const handleChunk = (chunk: ArrayBuffer) => {
            let start = 0;
            let arr = new Uint8Array(chunk);
            for (let i = 0; i < arr.length; i++) {
                if (arr[i] == 10) {
                    line += decoder.decode(arr.slice(start, i));
                    if (line == "") {
                        // Stop at the first blank line, since there's debugging
                        // information in a footer which we need to skip.
                        break;
                    }
                    callback(line);
                    line = "";
                    start = i + 1;
                }
            }
            if (start < arr.length) {
                // Unless the buffer ends with a newline, send the trailing end
                // to the decoder as a partial input.
                line += decoder.decode(arr.slice(start), { stream: true });
            }
        };

        let signal: AbortSignal | undefined;
        try {
            const controller = new AbortController();
            token.onCancellationRequested(_ => {
                controller.abort();
            });
            signal = controller.signal;
        } catch (err) {
            if (err instanceof ReferenceError) {
                // Node 14.x doesn't support AbortController, so just omit
                // cancellation support.
                console.warn("AbortController not available, skipping...");
            } else {
                throw err;
            }
        }

        if (typeof process === 'undefined' || process.title === 'browser') {
            return this.streamingFetchBrowser(url, handleChunk, signal);
        } else {
            return this.streamingFetchNode(url, handleChunk, signal);
        }
    }

    private static streamingFetchNode(url: string, handleChunk: (line: ArrayBuffer) => void, signal?: AbortSignal): Promise<void> {
        return axios
            .get(url, { responseType: 'stream', signal })
            .then(res => {
                res.data.on('data', handleChunk);
                return new Promise((resolve, _) => {
                    res.data.on('close', () => {
                        resolve();
                    });
                });
            });
    }

    private static streamingFetchBrowser(url: string, handleChunk: (line: ArrayBuffer) => void, signal?: AbortSignal): Promise<void> {
        return fetch(url, { signal }).then(res => {
            const reader = res.body!.getReader();
            return new Promise(async (resolve, _) => {
                while (true) {
                    const { done, value } = await reader.read();
                    if (done) break;
                    else handleChunk(value!);
                }
                resolve();
            });
        });
    }
}
