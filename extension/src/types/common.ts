import * as vscode from 'vscode';

export type Config = {
    scheme: string,
    distribution: string,

    meta: vscode.Uri,
    ls: vscode.Uri,
    cat: vscode.Uri,
    fzf: vscode.Uri,
};

export type Package = {
    name: string,
    version: string,
    epoch: number,
};

export function constructUri(config: Config, ...parts: string[]): vscode.Uri {
    return vscode.Uri.from({
        scheme: config.scheme,
        path: ["", config.distribution, ...parts].join("/"),
    });
}
