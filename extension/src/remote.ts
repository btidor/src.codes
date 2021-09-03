import * as vscode from 'vscode';
import axios from 'axios';

import { File, Directory, SymbolicLink } from './inode';

// We don't need to import TextDecoder, it's part of the Node runtime and the
// browser environment. If we let TypeScript put in the explicit 'util' import,
// it'll fail when running in the browser.
declare var TextDecoder: any;

const API_URLS = {
    META: vscode.Uri.parse('https://meta.src.codes/'),
    LS: vscode.Uri.parse('https://ls.src.codes/'),
    CAT: vscode.Uri.parse('https://cat.src.codes/'),
};

type PackageIndex = { [name: string]: { version: string, hash: string; }; };

export default class RemoteCache {
    // The distribution slug: "hirsute", "bullseye", etc.
    private distribution: string;

    // A listing of all packages in the distribution, with pointers to the
    // individual package manifests. Lazy-loaded.
    private packageIndex?: PackageIndex;

    // A cache of parsed package listings from the individual manifests.
    //
    // TODO: do we ever need to evict items from this cache?
    //
    private packageCache: { [key: string]: Directory; } = {};

    constructor(distribution: string) {
        this.distribution = distribution;
    }

    listPackages(): Thenable<string[]> {
        return this.getOrDownloadPackageIndex().then(idx => Object.keys(idx));
    }

    getPackageRoot(packageName: string): Thenable<Directory> {
        if (this.packageCache[packageName]) {
            return Promise.resolve(this.packageCache[packageName]);
        }

        return this.getOrDownloadPackageIndex()
            .then(idx => {
                let entry = idx[packageName];
                if (!entry) {
                    throw vscode.FileSystemError.FileNotFound();
                }

                let filename = new Array(
                    packageName, entry.version, entry.hash + ".json",
                ).join("_");
                let url = vscode.Uri.joinPath(API_URLS.LS, this.distribution, filename);
                return axios
                    .get(url.toString(), { responseType: 'json' })
                    .then(res => {
                        let parsed = this.parseJSONManifest(res.data);
                        this.packageCache[packageName] ||= parsed;
                        return this.packageCache[packageName]!;
                    })
                    .catch(err => {
                        throw vscode.FileSystemError.Unavailable(err);
                    });
            });
    }

    getFile(file: File) {
        let url = vscode.Uri.joinPath(
            API_URLS.CAT,
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

    private getOrDownloadPackageIndex(): Thenable<PackageIndex> {
        if (this.packageIndex) {
            return Promise.resolve(this.packageIndex);
        }

        let url = vscode.Uri.joinPath(API_URLS.META, this.distribution + ".json");
        return axios
            .get(url.toString(), { responseType: 'json' })
            .then(res => {
                this.packageIndex = res.data;
                return this.packageIndex!;
            })
            .catch(err => {
                throw vscode.FileSystemError.Unavailable(err);
            });
    }

    private parseJSONManifest(json: any, grandparent?: Directory): Directory {
        let parent = new Directory(grandparent);
        for (let item of Object.values(json.contents || []) as any) {
            var child;
            if (item.type! == 'symlink') {
                child = new SymbolicLink(parent, item.symlink_to);
            } else if (item.type! == 'file') {
                child = new File(parent, item.size!, item.sha256!);
            } else if (item.type! == 'directory') {
                child = this.parseJSONManifest(item, parent);
            } else {
                throw new Error("Unknown member type: " + item.type!);
            }
            parent.addChild(item.name!, child);
        }
        return parent;
    }
}
