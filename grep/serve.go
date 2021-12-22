package main

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/btidor/src.codes/internal"
	"github.com/google/codesearch/index"
	"github.com/google/codesearch/regexp"
)

func serve() {
	var indexes = make(map[string][]string)
	for distro := range distros {
		matches, err := filepath.Glob(
			filepath.Join(dataDir, "grep", distro, "*", "*.csi"),
		)
		if err != nil {
			panic(err)
		}
		if len(matches) < 1 {
			panic("no indexes found for distro " + distro)
		}
		indexes[distro] = matches
	}

	var server = &http.Server{
		Handler:      grepHandler{indexes},
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 60 * time.Second,
	}
	var err error
	if certPath != "" || keyPath != "" {
		fmt.Println("Listening on :443")
		server.Addr = ":443"
		err = server.ListenAndServeTLS(certPath, keyPath)
	} else {
		fmt.Println("Listening on :5050")
		server.Addr = ":5050"
		err = server.ListenAndServe()
	}
	panic(err)
}

type grepHandler struct {
	indexes map[string][]string
}

func (g grepHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Cache-Control", "no-cache")

	var parts = strings.Split(r.URL.Path, "/")
	if len(parts) != 2 {
		// Invalid path
		internal.HTTPError(w, r, 404)
		return
	}
	distro := parts[1]
	if distro == "" {
		// Requests to `/` show a welcome message
		fmt.Fprintf(w, "Hello from grep@%s!\n", commit)
		return
	} else if distro == "robots.txt" {
		// Requests to `/robots.txt` return our robots config
		fmt.Fprintf(w, "User-agent: *\nDisallow: /\n")
		return
	}
	idxlist, ok := g.indexes[distro]
	if !ok {
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
		internal.HTTPError(w, r, 400)
		return
	}
	grep.Regexp = re
	var iquery = index.RegexpQuery(re.Syntax)
	for _, name := range idxlist {
		ix := index.Open(name)
		for _, fileid := range ix.PostingQuery(iquery) {
			path := filepath.Join(dataDir, "packages", distro, ix.Name(fileid))
			grep.File(path)
		}
	}
}
