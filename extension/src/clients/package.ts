import * as vscode from 'vscode';
import axios from 'axios';

import { Config, Package } from '../types/common';
import { File, Directory, SymbolicLink } from '../types/inode';

type Path = {
    pkg: Package,
    components: string[],
};

type ManifestEntry = {
    type: 'file',
    size: number,
    sha256: string,
} | {
    type: 'directory',
    contents: { [key: string]: ManifestEntry; },
} | {
    type: 'symlink',
    symlink_to: string,
    is_directory: boolean,
};

export default class PackageClient {
    private config: Config;

    // A listing of all packages in the distribution, with pointers to the
    // individual package manifests.
    private packages: Thenable<{ [pkgName: string]: Package; }>;

    // A cache of the file system trees for each package, listing all files and
    // directories in the package. Lazy-loaded.
    private contents: { [pkgName: string]: Thenable<Directory>; };

    constructor(config: Config) {
        this.config = config;
        this.contents = {};

        // Download package index
        const url = vscode.Uri.joinPath(this.config.meta, this.config.distribution, "packages.json");
        this.packages = axios
            .get(url.toString(), { responseType: 'json' })
            .then(res => {
                const pkgs: { [pkgName: string]: Package; } = {};
                for (const name in res.data) {
                    const values = res.data[name];
                    pkgs[name] = {
                        name,
                        version: values.version,
                        epoch: values.epoch,
                    };
                }
                return pkgs;
            })
            .catch(err => {
                throw vscode.FileSystemError.Unavailable(err);
            });
    }

    getPackage(pkgName: string): Thenable<Package> {
        return this.packages.then(p => {
            if (pkgName in p) {
                return p[pkgName];
            } else {
                throw vscode.FileSystemError.FileNotFound();
            }
        });
    };

    parseUri(uri: vscode.Uri): Thenable<Path | undefined> {
        if (uri.scheme != this.config.scheme) {
            throw new Error("Unknown scheme: " + uri.scheme);
        }
        const parts = uri.path.split("/");
        if (parts.length < 3) {
            // Workspace root
            return Promise.resolve(undefined);
        }
        const pkgName = parts[2];
        return this.getPackage(pkgName).then(pkg => {
            return {
                pkg,
                components: parts.slice(3),
            };
        });
    }

    listPackages(): Thenable<Package[]> {
        return this.packages.then(p => Object.values(p));
    };

    listPackageContents(pkg: Package): Thenable<Directory> {
        if (!(pkg.name in this.contents)) {
            const filename = pkg.name + "_" + pkg.version + ":" + pkg.epoch + ".json";
            const url = vscode.Uri.joinPath(this.config.ls, this.config.distribution, pkg.name, filename);
            this.contents[pkg.name] = axios
                .get(url.toString(), { responseType: 'json' })
                .then(res => this.parsePackageManifest(res.data))
                .catch(err => {
                    throw vscode.FileSystemError.Unavailable(err);
                });
        }
        return this.contents[pkg.name];
    }

    private parsePackageManifest(json: any, grandparent?: Directory): Directory {
        const parent = new Directory(grandparent);
        for (const [name, item] of Object.entries(json.contents) as [string, ManifestEntry][]) {
            let child;
            if (item.type === 'symlink') {
                child = new SymbolicLink(parent, item.symlink_to, item.is_directory);
            } else if (item.type === 'file') {
                child = new File(parent, item.size, item.sha256);
            } else if (item.type === 'directory') {
                child = this.parsePackageManifest(item, parent);
            } else {
                throw new Error("Unknown manifest item: " + item);
            }
            parent.addChild(name, child);
        }
        return parent;
    }
}
