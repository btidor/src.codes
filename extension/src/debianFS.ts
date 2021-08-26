import * as vscode from 'vscode';
import fetch from 'node-fetch';
import { TextDecoder } from 'util';

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
type DebianUri = {package: string, subpath: string};

const API_BASE = vscode.Uri.parse('https://sources.debian.org/api/src/');
const FILE_BASE = vscode.Uri.parse('https://sources.debian.org/');
const CTAG_BASE = vscode.Uri.parse('https://sources.debian.org/api/ctag/');

export class DebianFS implements vscode.FileSystemProvider {

    // TODO: cache expiry!
    private directoryCache = new Map<DebianUri, Directory>();
    private statCache = new Map<DebianUri, DNode>();
    private context: vscode.ExtensionContext;
    private root?: Directory;

    constructor(context: vscode.ExtensionContext) {
        this.context = context;
    }

    private async getDirectory(uri: DebianUri): Promise<Directory> {
        let cached = this.directoryCache.get(uri);
        if (cached !== undefined) {
            return Promise.resolve(cached);
        }

        // TODO: proper version
        let url = vscode.Uri.joinPath(API_BASE, uri.package, 'buster', uri.subpath).toString();
        console.log('Requesting URL:', url);
        // TODO: user-agent
        try {
            const res = await fetch(url);
            const json = await res.json();
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
        } catch (err) {
            console.error('Fetch failed:', err);
            throw vscode.FileSystemError.Unavailable(err);
        }
    }

    private async doStat(uri: DebianUri): Promise<DNode> {
        let cached = this.statCache.get(uri);
        if (cached !== undefined) {
            return Promise.resolve(cached);
        }

        // TODO: proper version
        let url = vscode.Uri.joinPath(API_BASE, uri.package, 'buster', uri.subpath).toString();
        console.log('Requesting URL:', url);
        // TODO: user-agent
        try {
            const res = await fetch(url);
            const json = await res.json();
            let type: vscode.FileType;
            let node: DNode;
            if (json.type === "file") {
                // TODO: handle symlinks (symlink_dest)
                node = new DNode(
                    vscode.FileType.File, json.stat.size, json.raw_url
                );
            } else if (json.type === "directory") {
                node = new DNode(
                    vscode.FileType.Directory, 0,
                );
            } else {
                console.error("Unknown item type: ", json.type);
                throw vscode.FileSystemError.Unavailable();
            }
            this.statCache.set(uri, node);
            return node;
        } catch (err) {
            console.error('Fetch failed:', err);
            throw vscode.FileSystemError.Unavailable(err);
        }

    }

    private parseUri(uri: vscode.Uri): DebianUri {
        let parts = uri.path.split('/');
        return {
            package: parts[1],
            subpath: parts.slice(2).join('/'),
        };
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
        let parts = uri.path.split('/');
        console.log(parts);
        if (parts.length <= 2) {
            // TODO: does not properly handle invalid package names
            return new DNode(vscode.FileType.Directory, 0);
        } else {
            return this.doStat(this.parseUri(uri));
        }
    }

    readDirectory(uri: vscode.Uri): Directory | Thenable<Directory> {
        console.log('READDIR', uri);
        if (uri.path === '/') {
            if (this.root !== undefined) {
                return this.root;
            }

            return vscode.workspace.fs.readFile(
                vscode.Uri.joinPath(this.context.extensionUri, 'packages.txt'),
            ).then(raw => {
                let root = (new TextDecoder().decode(raw))
                    .trim()
                    .split("\n")
                    .map(line => [line, vscode.FileType.Directory]) as Directory;
                this.root = root;
                return root;
            });
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

export class Definer implements vscode.DefinitionProvider {
    provideDefinition(document: vscode.TextDocument, position: vscode.Position, token: vscode.CancellationToken): vscode.ProviderResult<vscode.Definition | vscode.LocationLink[]> {
        let wordRange = document.getWordRangeAtPosition(position /*, /[A-Za-z_][A-Za-z0-9_]*/);
        if (wordRange === undefined) {
            console.error('Could not extract word at position', position);
            // TODO: is this the right error?
            throw vscode.FileSystemError.Unavailable();
        }
        let word = document.getText(wordRange);

        // TODO: escape word, cache results, user-agent, paginated results
        let url = CTAG_BASE.toString() + '?ctag=' + word;
        console.log('Querying URL:', url);
        return fetch(url)
            .then(res => res.json())
            .then(json => {
                return json.results.map((result: { package: string; path: string; line: number; }) => {
                    return new vscode.Location(
                        vscode.Uri.joinPath(
                            vscode.Uri.parse('debian:/'),
                            // TODO: handle search not scoping by version
                            result.package, result.path,
                        ),
                        // TODO: figure out character offset?
                        new vscode.Position(result.line - 1, 0),
                    );
                });
            })
            .catch(err => {
                console.error('Fetch failed:', err);
                throw vscode.FileSystemError.Unavailable(err);
            });
    }
}
