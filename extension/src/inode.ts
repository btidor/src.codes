import * as vscode from 'vscode';

export interface Node {
    parent?: Directory;

    type: vscode.FileType;
    ctime: number;
    mtime: number;
    size: number;

    descend(childName: string): Node;
    resolveLinks(): File | Directory;
}

export class File implements vscode.FileStat, Node {
    parent: Directory;

    type: vscode.FileType;
    ctime: number;
    mtime: number;
    size: number;

    sha256: string;

    constructor(parent: Directory, size: number, sha256: string) {
        this.parent = parent;
        this.type = vscode.FileType.File;
        this.ctime = 0;
        this.mtime = 0;
        this.size = size;
        this.sha256 = sha256;
    }

    descend(_: string): Node {
        throw vscode.FileSystemError.FileNotADirectory();
    }

    resolveLinks = () => this;
}

export class Directory implements vscode.FileStat, Node {
    parent?: Directory;

    type: vscode.FileType;
    ctime: number;
    mtime: number;
    size: number;

    contents: { [name: string]: Node; };

    constructor(parent?: Directory) {
        this.parent = parent;
        this.type = vscode.FileType.Directory;
        this.ctime = 0;
        this.mtime = 0;
        this.size = 0;
        this.contents = {};
    }

    addChild(name: string, child: Node) {
        this.contents[name] = child;
    }

    descend(childName: string): Node {
        if (this.contents[childName]) {
            return this.contents[childName];
        } else {
            throw vscode.FileSystemError.FileNotFound();
        }
    }

    resolveLinks = () => this;

    walkPath(path: string[]): Node {
        var node: Node = this;
        for (let part of path) {
            node = node.descend(part);
        }
        return node;
    }
}

export class SymbolicLink implements vscode.FileStat, Node {
    parent: Directory;

    type: vscode.FileType;
    ctime: number;
    mtime: number;
    size: number;

    destination: string;

    constructor(parent: Directory, destination: string, isDir: boolean) {
        this.parent = parent;
        if (isDir) {
            this.type = vscode.FileType.SymbolicLink | vscode.FileType.Directory;
        } else {
            this.type = vscode.FileType.SymbolicLink | vscode.FileType.File;
        }
        this.ctime = 0;
        this.mtime = 0;
        this.size = 0;
        if (destination.startsWith("/")) {
            throw new Error("Symbolic links must not specify an absolute path");
        }
        this.destination = destination;
    }

    resolveLinks(): File | Directory {
        var node: File | Directory = this.parent;
        for (let part of this.destination.split("/")) {
            if (part === ".") {
                continue;
            } else if (part === "..") {
                if (node.parent) {
                    node = node.parent;
                } else {
                    throw new Error("Symbolic link backtracks past package root");
                }
            } else {
                var tmp: Node = node.descend(part);
                if (tmp === this) {
                    throw new Error("Symbolic link loop detected");
                }
                node = tmp.resolveLinks();
            }
        }
        return node;
    }

    descend(childName: string): Node {
        if (this.type & vscode.FileType.File) {
            throw vscode.FileSystemError.FileNotADirectory();
        }
        return this.resolveLinks().descend(childName);
    }
}
