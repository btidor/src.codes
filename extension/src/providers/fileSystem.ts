import * as vscode from 'vscode';
import FileClient from '../clients/file';
import PackageClient from '../clients/package';

import { File } from '../types/inode';

const readme = `# src.codes

src.codes is an online code browser for the Ubuntu package archive.

* Browse the source code for all 2,390 packages in \`main\`.

* Navigate the entire repository with standard VS Code tools:

   - fuzzy-find files by path (\`Ctrl-P\`)
   - full regex search (\`Ctrl-Shift-F\`)
   - cross-package go-to-definition (\`Ctrl-F12\`; C only)

~> https://github.com/btidor/src.codes
`

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
            if (path === "" || path === ".vscode") {
                return {type: vscode.FileType.Directory, ctime:0, mtime:0, size:0};
            } else if (path === ".vscode/README") {
                return {type: vscode.FileType.File, ctime:0, mtime:0, size:0};
            } else {
                // Within package
                return this.packageClient.listPackageContents(path.pkg).then(
                    root => root.walkPath(path.components)
                );
            }
        });
    }

    readDirectory(uri: vscode.Uri): [string, vscode.FileType][] | Thenable<[string, vscode.FileType][]> {
        return this.packageClient.parseUri(uri).then(path => {
            if (path === "") {
                // Workspace root. Enumerate all packages as top-level
                // directories.
                return this.packageClient.listPackages().then(
                    pkgs => [
                        [".vscode", vscode.FileType.Directory],
                        ...pkgs.map(pkg => [pkg.name, vscode.FileType.Directory]),
                    ] as [string, vscode.FileType][]
                );
            } else if (path === ".vscode") {
                return [["README", vscode.FileType.File]];
            } else if (path === ".vscode/README") {
                throw vscode.FileSystemError.FileNotADirectory();
            } else {
                // Within package. Find the directory at the given path and
                // enumerate its contents.
                return this.packageClient.listPackageContents(path.pkg).then(root => {
                    const node = root.walkPath(path.components).resolveLinks();
                    if (node instanceof File) {
                        throw vscode.FileSystemError.FileNotADirectory();
                    } else {
                        return Object.entries(node.contents).map(([name, node]) => [name, node.type]);
                    }
                });
            }
        });
    }

    readFile(uri: vscode.Uri): Uint8Array | Thenable<Uint8Array> {
        return this.packageClient.parseUri(uri).then(path => {
            if (path === "" || path === ".vscode") {
                throw vscode.FileSystemError.FileIsADirectory();
            } else if (path === ".vscode/README") {
                return new TextEncoder().encode(readme);
            } else {
                return this.packageClient.listPackageContents(path.pkg).then(root => {
                    const node = root.walkPath(path.components).resolveLinks();
                    if (node instanceof File) {
                        return this.fileClient.downloadFile(node);
                    } else {
                        throw vscode.FileSystemError.FileIsADirectory();
                    }
                });
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
