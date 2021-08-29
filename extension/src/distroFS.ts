import * as vscode from 'vscode';
import fetch from 'node-fetch';
import { TextDecoder } from 'util';

const CAT_BASE = vscode.Uri.parse('https://cat.src.codes/');``

export class DNode implements vscode.FileStat {
    type: vscode.FileType;
    ctime: number;
    mtime: number;
    size: number;

    constructor(type: vscode.FileType, size: number) {
        this.type = type;
        this.ctime = Date.now();
        this.mtime = Date.now();
        this.size = size;
    }
}

export class DistroFS implements vscode.FileSystemProvider {
    private context: vscode.ExtensionContext;

    constructor(context: vscode.ExtensionContext) {
        this.context = context;
    }

    readonly onDidChangeFile: vscode.Event<vscode.FileChangeEvent[]> = (
        // Static read-only file system, no change events
        _listener => new vscode.Disposable(() => { })
    );

    watch(_uri: vscode.Uri, _options: { recursive: boolean; excludes: string[]; }): vscode.Disposable {
        // Static read-only file system, no change events
        return new vscode.Disposable(() => { });
    }

    stat(uri: vscode.Uri): vscode.FileStat | Thenable<vscode.FileStat> {
        console.log('STAT', uri);
        if (uri.path === '/') {
            return new DNode(vscode.FileType.Directory, 0);
        } else {
            return vscode.workspace.fs.readFile(
                vscode.Uri.joinPath(this.context.extensionUri, 'stats.txt'),
            ).then(raw => {
                let parts = (new TextDecoder().decode(raw))
                    .trim()
                    .split("\n")
                    .map(line => line.split(" "))
                    .find(parts => parts[0] == uri.path.slice(1));

                if (parts === undefined) {
                    throw vscode.FileSystemError.FileNotFound();
                }

                var type;
                if (parts[3] == 'regular') {
                    type = vscode.FileType.File;
                } else if (parts[3] == 'directory') {
                    type = vscode.FileType.Directory;
                } else if (parts[3] == 'symbolic') {
                    type = vscode.FileType.SymbolicLink;
                } else {
                    type = vscode.FileType.Unknown;
                }
                let size = Number(parts[2]);
                return new DNode(type, size);
            });
        }
    }

    readDirectory(uri: vscode.Uri): [string, vscode.FileType][] | Thenable<[string, vscode.FileType][]> {
        console.log('READDIR', uri);
        if (uri.path === '/') {
            return vscode.workspace.fs.readFile(
                vscode.Uri.joinPath(this.context.extensionUri, 'packages.txt'),
            ).then(raw => {
                let root = (new TextDecoder().decode(raw))
                    .trim()
                    .split("\n")
                    .map(line => [line, vscode.FileType.Directory]) as [string, vscode.FileType][];
                return root;
            });
        } else {
            return vscode.workspace.fs.readFile(
                vscode.Uri.joinPath(this.context.extensionUri, 'stats.txt'),
            ).then(raw => {
                let search = uri.path.slice(1)
                    .replace(/[-\/\\^$*+?.()|[\]{}]/g, '\\$&');
                let re = new RegExp("^" + search + "/[^/]+$")
                return (new TextDecoder().decode(raw))
                    .trim()
                    .split("\n")
                    .map(line => line.split(" "))
                    .filter(parts => parts[0].match(re))
                    .map(parts => {
                        var type;
                        if (parts[3] == 'regular') {
                            type = vscode.FileType.File;
                        } else if (parts[3] == 'directory') {
                            type = vscode.FileType.Directory;
                        } else if (parts[3] == 'symbolic') {
                            type = vscode.FileType.SymbolicLink;
                        } else {
                            type = vscode.FileType.Unknown;
                        }
                        return [parts[0], type];
                    });
            });
        }
    }

    readFile(uri: vscode.Uri): Uint8Array | Thenable<Uint8Array> {
        console.log('READFILE', uri);
        return vscode.workspace.fs.readFile(
            vscode.Uri.joinPath(this.context.extensionUri, 'shasums.txt'),
        ).then(raw => {
            let parts = (new TextDecoder().decode(raw))
                .trim()
                .split("\n")
                .map(line => line.split(" "))
                .find(parts => parts[2] == uri.path.slice(1));

            if (parts === undefined) {
                throw vscode.FileSystemError.FileNotFound();
            }

            let hash = parts[0];
            let url = vscode.Uri.joinPath(
                CAT_BASE,
                hash.slice(0, 1),
                hash.slice(0, 2),
                hash.slice(0, 3),
                hash,
            );
            console.log('Fetching URL:', url);

            return fetch(url.toString())
                .then(res => res.arrayBuffer())
                .then(arr => new Uint8Array(arr))
                .catch(err => {
                    console.error('Fetch failed:', err);
                    throw vscode.FileSystemError.Unavailable(err);
                });
        });
    }

    // VS Code shouldn't try to call these methods...
    createDirectory(uri: vscode.Uri): void | Thenable<void> {
        throw new Error('File system is read-only.');
    }

    writeFile(uri: vscode.Uri, content: Uint8Array, options: { create: boolean; overwrite: boolean; }): void | Thenable<void> {
        throw new Error('File system is read-only.');
    }

    delete(uri: vscode.Uri, options: { recursive: boolean; }): void | Thenable<void> {
        throw new Error('File system is read-only.');
    }

    rename(oldUri: vscode.Uri, newUri: vscode.Uri, options: { overwrite: boolean; }): void | Thenable<void> {
        throw new Error('File system is read-only.');
    }
}
