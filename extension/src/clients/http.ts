import axios from 'axios';

export default class HTTPClient {
    static streamingFetch(url: string, callback: (line: string) => void): Promise<void> {
        if (typeof process !== 'undefined') {
            return this.streamingFetchNode(url, callback);
        } else {
            return this.streamingFetchBrowser(url, callback);
        }
    }

    private static streamingFetchNode(url: string, callback: (line: string) => void): Promise<void> {
        return axios
            .get(url, { responseType: 'stream' })
            .then(res => {
                let line = "";
                let decoder = new TextDecoder();
                res.data.on('data', (chunk: ArrayBuffer) => {
                    let start = 0;
                    let arr = new Uint8Array(chunk);
                    for (let i = 0; i < arr.length; i++) {
                        if (arr[i] == 10) {
                            line += decoder.decode(arr.slice(start, i));
                            if (line == "") {
                                // Stop at the first blank line, since there's
                                // debugging information in a footer which we
                                // need to skip.
                                break;
                            }
                            callback(line);
                            line = "";
                            start = i + 1;
                        }
                    }
                    if (start < arr.length) {
                        // Unless the buffer ends with a newline, send the
                        // trailing end to decoder as a partial input.
                        line += decoder.decode(arr.slice(start), { stream: true });
                    }
                });
                return new Promise((resolve, _) => {
                    res.data.on('close', () => {
                        resolve();
                    });
                });
            });
    }

    private static streamingFetchBrowser(url: string, callback: (line: string) => void): Promise<void> {
        return fetch(url).then(res => {
            const reader = res.body!.getReader();
            return new Promise(async (resolve, _) => {
                let line = "";
                let decoder = new TextDecoder();
                while (true) {
                    const { done, value } = await reader.read();
                    if (done) {
                        resolve();
                        return;
                    }
                    let start = 0;
                    let arr = new Uint8Array(value!);
                    for (let i = 0; i < arr.length; i++) {
                        if (arr[i] == 10) {
                            line += decoder.decode(arr.slice(start, i));
                            if (line == "") {
                                // Stop at the first blank line, since there's
                                // debugging information in a footer which we
                                // need to skip.
                                break;
                            }
                            callback(line);
                            line = "";
                            start = i + 1;
                        }
                    }
                    if (start < arr.length) {
                        // Unless the buffer ends with a newline, send the
                        // trailing end to decoder as a partial input.
                        line += decoder.decode(arr.slice(start), { stream: true });
                    }
                }
            });
        });
    }
}
