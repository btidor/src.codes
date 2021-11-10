import * as vscode from 'vscode';
import axios from 'axios';

import { Config } from '../types/common';
import { File } from '../types/inode';

export default class FileClient {
    private config: Config;

    constructor(config: Config) {
        this.config = config;
    }

    downloadFile(file: File): Thenable<Uint8Array> {
        const url = vscode.Uri.joinPath(
            this.config.cat,
            file.sha256.slice(0, 2),
            file.sha256.slice(0, 4),
            file.sha256,
        );
        return axios
            .get(url.toString(), { responseType: 'arraybuffer' })
            .then(res => new Uint8Array(res.data))
            .catch(err => {
                throw vscode.FileSystemError.Unavailable(err);
            });
    }
}
