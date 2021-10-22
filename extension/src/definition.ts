import axios from 'axios';
import * as vscode from 'vscode';

const CTAGS_URL = vscode.Uri.parse('https://ctags.src.codes/');

export default class SourceCodesDefinitionProvider implements vscode.DefinitionProvider {
    private distribution: string;

    constructor(distribution: string) {
        this.distribution = distribution;
    }

    provideDefinition(document: vscode.TextDocument, position: vscode.Position, _token: vscode.CancellationToken): vscode.ProviderResult<vscode.Definition | vscode.LocationLink[]> {
        let wordRange = document.getWordRangeAtPosition(position);
        if (wordRange === undefined) {
            throw new Error('Could not find word at given position.');
        }

        let word = document.getText(wordRange);
        let slug = word.substring(0, 2).replace(/[^A-Za-z0-9]/, '_').toLowerCase();

        let url = vscode.Uri.joinPath(CTAGS_URL, this.distribution, slug).toString();
        console.warn('Requesting URL', url);
        return axios
            .get(url, { responseType: 'text' })
            .then(res => {
                let result: vscode.Location[] = [];
                for (let line of res.data.split("\n")) {
                    let parts = line.split("\t", 3);
                    if (parts[0] == word) {
                        let subparts = parts[2].split(/(;"|\t)/);
                        let uri = vscode.Uri.parse("srccodes:/" + this.distribution + "/" + parts[1]);
                        let range = new vscode.Position(Number(subparts[0]) - 1, 0);
                        result.push(new vscode.Location(uri, range));
                    }
                }
                // TODO: filter and prioritize results
                return result;
            });
    }
}
