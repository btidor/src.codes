import axios from 'axios';
import * as vscode from 'vscode';

const FZF_URL = vscode.Uri.parse('https://fzf.src.codes/');

export default class SourceCodesFileSearchProvider implements vscode.FileSearchProvider {
    private distribution: string;

    constructor(distribution: string) {
        this.distribution = distribution;
    }

    provideFileSearchResults(query: vscode.FileSearchQuery, _options: vscode.FileSearchOptions, _token: vscode.CancellationToken): vscode.ProviderResult<vscode.Uri[]> {
        let params = new URLSearchParams();
        params.append('q', query.pattern);
        let url = vscode.Uri.joinPath(FZF_URL, this.distribution)
            .with({ query: params.toString() });
        return axios
            .get(url.toString(), { responseType: 'text' })
            .then(res => {
                let result: vscode.Uri[] = [];
                for (let line of res.data.split("\n")) {
                    if (!line) {
                        // Stop at the first blank line, since there's debugging
                        // information in a footer which we need to skip.
                        break;
                    }
                    let parts = line.split(" ");
                    result.push(vscode.Uri.parse("/" + this.distribution + "/" + parts[1]));
                }
                return result;
            });
    }
}
