import * as vscode from 'vscode';
import FileClient from '../clients/file';
import PackageClient from '../clients/package';

import { File, Directory } from '../types/inode';

export default class FileSystemProvider implements vscode.FileSystemProvider {
    private packageClient: PackageClient;
    private fileClient: FileClient;

    constructor(packageClient: PackageClient, fileClient: FileClient) {
        this.packageClient = packageClient;
        this.fileClient = fileClient;
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
        return this.packageClient.parseUri(uri).then(path => {
            if (path) {
                // Within package
                return this.packageClient.listPackageContents(path.pkg).then(
                    root => root.walkPath(path.components)
                );
            } else {
                // Workspace root. This node doesn't have its real contents, but
                // that's okay, VS Code doesn't know about `.contents` anyway...
                return new Directory();
            }
        });
    }

    readDirectory(uri: vscode.Uri): [string, vscode.FileType][] | Thenable<[string, vscode.FileType][]> {
        return this.packageClient.parseUri(uri).then(path => {
            if (path) {
                // Within package. Find the directory at the given path and
                // enumerate its contents.
                return this.packageClient.listPackageContents(path.pkg).then(root => {
                    const node = root.walkPath(path.components).resolveLinks();
                    if (node instanceof File) {
                        throw vscode.FileSystemError.FileNotADirectory();
                    } else {
                        return Object.entries(node.contents).map(x => [x[0], x[1].type]);
                    }
                });
            } else {
                // Workspace root. Enumerate all packages as top-level
                // directories.
                return this.packageClient.listPackages().then(
                    pkgs => pkgs.map(pkg => [pkg.name, vscode.FileType.Directory])
                );
            }
        });
    }

    readFile(uri: vscode.Uri): Uint8Array | Thenable<Uint8Array> {
        return this.packageClient.parseUri(uri).then(path => {
            if (path) {
                return this.packageClient.listPackageContents(path.pkg).then(root => {
                    const node = root.walkPath(path.components).resolveLinks();
                    if (node instanceof File) {
                        return this.fileClient.downloadFile(node);
                    } else {
                        throw vscode.FileSystemError.FileIsADirectory();
                    }
                });
            } else {
                // Workspace root. Not a file!
                throw vscode.FileSystemError.FileIsADirectory();
            }
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
