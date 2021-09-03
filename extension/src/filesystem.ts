import * as vscode from 'vscode';

import RemoteCache from './remote';
import { File, Directory, SymbolicLink } from './inode';

// Type: the workspace root, /workspace
interface Root {
    type: 'root',
}

// Type: a path inside of a package, /workspace/package/subpath?...
interface InPackage {
    type: 'inpackage',
    package: string,
    subpath: string[],
}

function absurd(p: never): never {
    throw new Error('Invalid type: ' + p);
}

export default class SourceCodesFilesystem implements vscode.FileSystemProvider {
    private remoteCache: RemoteCache;

    constructor(distribution: string) {
        this.remoteCache = new RemoteCache(distribution);
    }

    readonly onDidChangeFile: vscode.Event<vscode.FileChangeEvent[]> = (
        // Static read-only file system, no change events
        _listener => new vscode.Disposable(() => { })
    );

    watch(_uri: vscode.Uri, _options: { recursive: boolean; excludes: string[]; }): vscode.Disposable {
        // Static read-only file system, no change events
        return new vscode.Disposable(() => { });
    }

    private parsePath(uri: vscode.Uri): Root | InPackage {
        var parts = uri.path.split('/');
        parts.shift(); // skip blank member for leading slash
        parts.shift(); // skip workspace name
        if (parts.length > 0) {
            return {
                type: 'inpackage',
                package: parts.shift()!,
                subpath: parts,
            };
        } else {
            return { type: 'root' };
        }
    }

    stat(uri: vscode.Uri): vscode.FileStat | Thenable<vscode.FileStat> {
        const path = this.parsePath(uri);
        if (path.type == 'root') {
            // This node doesn't have its real contents, but that's okay, VS
            // Code doesn't know about `.contents` anyway...
            return new Directory();
        } else if (path.type == 'inpackage') {
            return this.remoteCache
                .getPackageRoot(path.package)
                .then(node => node.walkPath(path.subpath));
        } else {
            absurd(path);
        }
    }

    readDirectory(uri: vscode.Uri): [string, vscode.FileType][] | Thenable<[string, vscode.FileType][]> {
        const path = this.parsePath(uri);
        if (path.type == 'root') {
            return this.remoteCache
                .listPackages()
                .then(packages => packages.map((name, _) => [name, vscode.FileType.Directory]));
        } else if (path.type == 'inpackage') {
            return this.remoteCache
                .getPackageRoot(path.package)
                .then(root => {
                    let node = root.walkPath(path.subpath).resolveLinks();

                    if (node instanceof File) {
                        throw vscode.FileSystemError.FileNotADirectory();
                    } else if (node instanceof Directory) {
                        let contents = node.contents;
                        return Object.keys(contents).map(filename => {
                            let child = contents[filename];
                            if (child instanceof SymbolicLink) {
                                child.updateType();
                            }
                            return [filename, child.type];
                        });
                    } else {
                        absurd(node);
                    }
                });
        } else {
            absurd(path);
        }
    }

    readFile(uri: vscode.Uri): Uint8Array | Thenable<Uint8Array> {
        const path = this.parsePath(uri);
        if (path.type == 'root') {
            throw vscode.FileSystemError.FileIsADirectory();
        } else if (path.type == 'inpackage') {
            return this.remoteCache
                .getPackageRoot(path.package)
                .then(root => {
                    let node = root.walkPath(path.subpath).resolveLinks();

                    if (node instanceof File) {
                        return this.remoteCache.getFile(node);
                    } else if (node instanceof Directory) {
                        throw vscode.FileSystemError.FileIsADirectory();
                    } else {
                        absurd(node);
                    }
                });
        } else {
            absurd(path);
        }
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
