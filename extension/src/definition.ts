import axios from 'axios';
import * as vscode from 'vscode';

import RemoteCache from './remote';

const LS_URL = vscode.Uri.parse('https://ls.src.codes/');
const CTAGS_URL = vscode.Uri.parse('https://ctags.src.codes/');

export default class SourceCodesDefinitionProvider implements vscode.DefinitionProvider {
    private distribution: String;
    private remoteCache: RemoteCache;

    constructor(distribution: string) {
        this.distribution = distribution;
        this.remoteCache = new RemoteCache(distribution);
    }

    provideDefinition(document: vscode.TextDocument, position: vscode.Position, _token: vscode.CancellationToken): vscode.ProviderResult<vscode.Definition | vscode.LocationLink[]> {
        let wordRange = document.getWordRangeAtPosition(position);
        if (wordRange === undefined) {
            throw new Error('Could not find word at given position.');
        }

        let word = document.getText(wordRange);

        let pkg = document.uri.path.split('/')[2]; // TODO: validation
        return this.remoteCache.getPackageTags(pkg).then(data => {
            let results: vscode.Location[] = [];
            for (let line of data.split("\n")) {
                let parts = line.split("\t");
                if (parts[0] == word) {
                    let subparts = parts[2].split(/(;"|\t)/);
                    let uri = vscode.Uri.parse("srccodes:/" + this.distribution + "/" + pkg + "/" + parts[1]);
                    let range = new vscode.Position(Number(subparts[0]) - 1, 0);
                    results.push(new vscode.Location(uri, range));
                }
            }

            // TODO: run the two requests in parallel
            return this.remoteCache.getSymbolsIndex().then(idx => {
                let target = " " + word + "@";
                let matching = false;
                let pkg2 = undefined;
                for (let line of idx.split("\n")) {
                    if (line.startsWith("### ")) {
                        pkg2 = line.split(" ")[1];
                    } else if (matching && line.startsWith(" - ") && pkg2 != pkg) {
                        let parts = line.substring(3).split(/(;"|\t)/);
                        let uri = vscode.Uri.parse("srccodes:/" + this.distribution + "/" + pkg2! + "/" + parts[0]);
                        let range = new vscode.Position(Number(parts[2]) - 1, 0);
                        results.push(new vscode.Location(uri, range));
                    } else if (line.startsWith(target)) {
                        matching = true;
                    } else {
                        matching = false;
                    }
                }
                return results;
            });
        });
    }
}
