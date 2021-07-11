import { types } from 'util';
import * as vscode from 'vscode';
import fetch from 'node-fetch';

export class DNode implements vscode.FileStat {
    type: vscode.FileType;
    ctime: number;
    mtime: number;
    size: number;
    rawUrl?: string | undefined;

    constructor(type: vscode.FileType, size: number, rawUrl?: string | undefined) {
        this.type = type;
        this.ctime = Date.now();
        this.mtime = Date.now();
        this.size = size;
        this.rawUrl = rawUrl;
    }
}

type Directory = [string, vscode.FileType][];
type DebianUri = {package: string, version: string, subpath: string};

const API_BASE = vscode.Uri.parse('https://sources.debian.org/api/src/');
const FILE_BASE = vscode.Uri.parse('https://sources.debian.org/');

export class DebianFS implements vscode.FileSystemProvider {

    // TODO: cache expiry!
    private directoryCache = new Map<DebianUri, Directory>();
    private statCache = new Map<DebianUri, DNode>();

    private getDirectory(uri: DebianUri): Directory | Thenable<Directory> {
        let cached = this.directoryCache.get(uri);
        if (cached !== undefined) {
            return cached;
        }

        let url = vscode.Uri.joinPath(API_BASE, uri.package, uri.version, uri.subpath).toString();
        console.log('Requesting URL:', url);
        // TODO: user-agent
        return fetch(url)
            .then(res => res.json())
            .then(json => {
                let contents = json.content.map((item: { type: string; name: string; }) => {
                    let type: vscode.FileType;
                    if (item.type === "file") {
                        // TODO: handle symlinks (symlink_dest)
                        type = vscode.FileType.File;
                    } else if (item.type === "directory") {
                        type = vscode.FileType.Directory;
                    } else {
                        console.error("Unknown item type: ", item.type);
                        type = vscode.FileType.Unknown;
                    }
                    return [item.name, type];
                });
                this.directoryCache.set(uri, contents);
                return contents;
            })
            .catch(err => {
                console.error('Fetch failed:', err);
                throw vscode.FileSystemError.Unavailable(err);
            });
    }

    private doStat(uri: DebianUri): Thenable<DNode> {
        let cached = this.statCache.get(uri);
        if (cached !== undefined) {
            return Promise.resolve(cached);
        }

        let url = vscode.Uri.joinPath(API_BASE, uri.package, uri.version, uri.subpath).toString();
        console.log('Requesting URL:', url);
        // TODO: user-agent
        return fetch(url)
            .then(res => res.json())
            .then(json => {
                let type: vscode.FileType;
                if (json.type === "file") {
                    // TODO: handle symlinks (symlink_dest)
                    type = vscode.FileType.File;
                } else if (json.type === "directory") {
                    type = vscode.FileType.Directory;
                } else {
                    console.error("Unknown item type: ", json.type);
                    type = vscode.FileType.Unknown;
                }
                let node = new DNode(
                    type, json.stat.size, json.raw_url,
                );
                this.statCache.set(uri, node);
                return node;
            })
            .catch(err => {
                console.error('Fetch failed:', err);
                throw vscode.FileSystemError.Unavailable(err);
            });

    }

    private parseUri(uri: vscode.Uri): DebianUri {
        let parts = uri.path.split('/');
        return {
            package: parts[2],
            version: parts[1],
            subpath: parts.slice(3).join('/'),
        };
    }

    readonly onDidChangeFile: vscode.Event<vscode.FileChangeEvent[]> = (
        // Read-only file system, no change events
        _listener => new vscode.Disposable(() => { })
    );

    watch(_uri: vscode.Uri, _options: { recursive: boolean; excludes: string[]; }): vscode.Disposable {
        // Read-only file system, no change events
        return new vscode.Disposable(() => { });
    }

    stat(uri: vscode.Uri): vscode.FileStat | Thenable<vscode.FileStat> {
        console.log('STAT', uri);
        if (uri.path === '/buster' || uri.path === '/buster/fprintd' || uri.path === '/buster/ocaml') {
            return new DNode(vscode.FileType.Directory, 0);
        } else {
            return this.doStat(this.parseUri(uri));
        }
    }

    readDirectory(uri: vscode.Uri): Directory | Thenable<Directory> {
        console.log('READDIR', uri);
        if (uri.path === '/buster') {
            return [
                // TODO: dynamic package list
                ['fprintd', vscode.FileType.Directory],
                ['ocaml', vscode.FileType.Directory],
            ];
        } else {
            return this.getDirectory(this.parseUri(uri));
        }
    }

    readFile(uri: vscode.Uri): Uint8Array | Thenable<Uint8Array> {
        console.log('READFILE', uri);
        return this.doStat(this.parseUri(uri))
            .then(node => {
                let url = vscode.Uri.joinPath(FILE_BASE, node.rawUrl as string).toString();
                console.log('Fetching URL:', url);
                return fetch(url)
                    .then(res => res.arrayBuffer())
                    .then(arr => new Uint8Array(arr))
                    .catch(err => {
                        console.error('Fetch failed:', err);
                        throw vscode.FileSystemError.Unavailable(err);
                    });

            });
    }

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
