package main

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/btidor/src.codes/internal"
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

		var results = search(query)
		for _, result := range results {
			fmt.Fprintln(w, result)
		}
	})
	err := http.ListenAndServeTLS(":443", certPath, keyPath, nil)
	panic(err)
}

func search(query string) []string {
	// TODO
	return []string{}
}
