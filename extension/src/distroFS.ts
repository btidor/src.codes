import * as vscode from 'vscode';
import axios from 'axios';

// We don't need to import TextDecoder, it's part of the Node runtime and the
// browser environment. If we let TypeScript put in the explicit 'util' import,
// it'll fail when running in the browser.
declare var TextDecoder: any;

const CAT_BASE = vscode.Uri.parse('https://cat.src.codes/');
const LS_BASE = vscode.Uri.parse('https://ls.src.codes/');

const DISTRO = 'hirsute';

type Packages = {[name: string]: {version: string, hash: string}};

type INode = {
    name?: string,
    type: 'file' | 'directory',
    contents: {[name: string]: INode},
    size: number;
    sha256?: string,
    symlink_to?: string,
};

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
    private packages?: Packages;
    private packageCache: {[key: string]: INode};

    constructor() {
        this.packageCache = {};
    }

    getPackages(): Thenable<Packages> {
        if (this.packages !== undefined) {
            return Promise.resolve(this.packages);
        }
        let url = vscode.Uri.joinPath(LS_BASE, DISTRO, "packages.json")
        console.log('Fetching URL:', url);
        return axios
            .get(url.toString(), {responseType: 'json'})
            .then(res => {
                this.packages = res.data;
                return res.data;
            })
            .catch(err => {
                console.error('Fetch failed:', err);
                throw vscode.FileSystemError.Unavailable(err);
            });
    };

    getPackage(name: string): Thenable<INode> {
        if (this.packageCache[name] !== undefined) {
            return Promise.resolve(this.packageCache[name]);
        }
        return this.getPackages().then(packages => {
            let entry = packages[name];
            if (entry === undefined) {
                throw vscode.FileSystemError.FileNotFound();
            }
            let filename = name + "_" + entry.version + "_" + entry.hash + ".json";
            let url = vscode.Uri.joinPath(LS_BASE, DISTRO, filename);
            console.log('Fetching URL:', url);
            return axios
                .get(url.toString(), {responseType: 'json'})
                .then(res => {
                    this.packageCache[name] = res.data;
                    return res.data;
                })
                .catch(err => {
                    console.error('Fetch failed:', err);
                    throw vscode.FileSystemError.Unavailable(err);
                })
        })
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
            let pkgname = uri.path.split('/')[1];
            return this.getPackage(pkgname)
                .then(pkg => {
                    var parts = uri.path.split('/').slice(2);
                    var node = pkg;
                    while (parts.length > 0) {
                        let part = parts.shift() as string;
                        node = node.contents[part];
                        if (node === undefined) {
                            // TODO: support symlinks
                            throw vscode.FileSystemError.FileNotFound();
                        }
                    }
                    var type;
                    if (node.type == 'file') {
                        type = vscode.FileType.File;
                    } else if (node.type == 'directory') {
                        type = vscode.FileType.Directory;
                    } else {
                        type = vscode.FileType.Unknown;
                    }

                    return new DNode(type, node.size || 0);
                });
        }
    }

    readDirectory(uri: vscode.Uri): [string, vscode.FileType][] | Thenable<[string, vscode.FileType][]> {
        console.log('READDIR', uri);
        if (uri.path === '/') {
            return this.getPackages().then(packages => {
                return Object.keys(packages).map((name, _) => [name, vscode.FileType.Directory]);
            });
        } else {
            let pkgname = uri.path.split('/')[1];
            return this.getPackage(pkgname)
                .then(pkg => {
                    var parts = uri.path.split('/').slice(2);
                    var node = pkg;
                    while (parts.length > 0) {
                        let part = parts.shift() as string;
                        node = node.contents[part];
                        if (node === undefined) {
                            // TODO: support symlinks
                            throw vscode.FileSystemError.FileNotFound();
                        }
                    }
                    return Object.keys(node.contents).map(k => {
                        let n = node.contents[k];
                        var type;
                        if (n.type == 'file') {
                            type = vscode.FileType.File;
                        } else if (n.type == 'directory') {
                            type = vscode.FileType.Directory;
                        } else {
                            type = vscode.FileType.Unknown;
                        }

                        if (n.symlink_to !== undefined) {
                            type |= vscode.FileType.SymbolicLink;
                        }
                        return [k, type];
                    })
                });
        }
    }

    readFile(uri: vscode.Uri): Uint8Array | Thenable<Uint8Array> {
        console.log('READFILE', uri);

        let pkgname = uri.path.split('/')[1];
        return this.getPackage(pkgname)
            .then(pkg => {
                var parts = uri.path.split('/').slice(2);
                var node = pkg;
                while (parts.length > 0) {
                    let part = parts.shift() as string;
                    node = node.contents[part];
                    if (node === undefined) {
                        // TODO: support symlinks
                        throw vscode.FileSystemError.FileNotFound();
                    }
                }

                if (node.sha256 === undefined) {
                    // TODO: support symlinks
                    throw vscode.FileSystemError.Unavailable();
                }
                let url = vscode.Uri.joinPath(
                    CAT_BASE,
                    node.sha256.slice(0, 2),
                    node.sha256.slice(0, 4),
                    node.sha256,
                );
                console.log('Fetching URL:', url);
                return axios
                .get(url.toString(), {responseType: 'arraybuffer'})
                .then(res => new Uint8Array(res.data))
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
