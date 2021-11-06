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
            return results;
        });
    }
}
