package main

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/btidor/src.codes/internal"
	"github.com/google/codesearch/index"
	"github.com/google/codesearch/regexp"
)

func serve() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		var parts = strings.Split(r.URL.Path, "/")
		if len(parts) != 2 {
			// Invalid path
			internal.HTTPError(w, r, 404)
			return
		}
		distro := parts[1]
		if distro == "" {
			// Requests to `/` show a welcome message
			fmt.Fprintf(w, "Hello from grep@dev!\n")
			return
		}
		if _, ok := distros[distro]; !ok {
			// Unknown distro
			internal.HTTPError(w, r, 404)
			return
		}

		var query = r.URL.Query().Get("q")
		if query == "" {
			// Missing query
			internal.HTTPError(w, r, 400)
			return
		}

		var grep = regexp.Grep{Stdout: w, Stderr: w}
		re, err := regexp.Compile("(?m)" + query)
		if err != nil {
			// TODO: handle gracefully
			panic(err)
		}
		grep.Regexp = re
		var iquery = index.RegexpQuery(re.Syntax)
		search(distro, grep, iquery)
	})
	var err error
	if certPath != "" || keyPath != "" {
		fmt.Println("Listening on :443")
		err = http.ListenAndServeTLS(":443", certPath, keyPath, nil)
	} else {
		fmt.Println("Listening on :5050")
		err = http.ListenAndServe(":5050", nil)
	}
	panic(err)
}

func search(distro string, grep regexp.Grep, iquery *index.Query) {
	indexes, err := filepath.Glob(
		filepath.Join(dataDir, "grep", distro, "*", "*.csi"),
	)
	if err != nil {
		panic(err)
	}
	if len(indexes) < 1 {
		panic("no indexes found")
	}

	for _, name := range indexes {
		ix := index.Open(name)
		for _, fileid := range ix.PostingQuery(iquery) {
			path := filepath.Join(dataDir, "packages", distro, ix.Name(fileid))
			grep.File(path)
		}
	}
}
