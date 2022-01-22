package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar"
	"github.com/btidor/src.codes/internal"
	"github.com/google/codesearch/index"
	"github.com/google/codesearch/regexp"
)

func serve() {
	var indexes = make(map[string][]string)
	for distro := range distros {
		matches, err := filepath.Glob(
			filepath.Join(fastDir, "grep", distro, "*", "*", "*.csi"),
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
	w.Header().Add("Access-Control-Allow-Origin", "*")
	w.Header().Add("Cache-Control", "no-cache")
	w.Header().Add("Content-Security-Policy", "default-src 'none';")

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

	if !strings.Contains(r.Header.Get("User-Agent"), "Firefox/") ||
		r.Header.Get("Sec-Fetch-Mode") != "navigate" {
		// Workaround: this should be text/plain, but it's the only way I've found
		// to make the Fetch API actually stream responses back. (I think they're
		// being blocked by CORB?) See:
		// https://www.reddit.com/r/webdev/comments/bwrpjl/how_to_stop_browser_from_buffering_a_streaming/
		w.Header().Add("Content-Type", "text/event-stream; charset=utf-8")
	}

	var start = time.Now()
	var query = r.URL.Query().Get("q")
	if query == "" {
		// Missing query
		internal.HTTPError(w, r, 400)
		return
	}

	var includes = r.URL.Query()["include"]
	var excludes = r.URL.Query()["exclude"]

	// Always allow ^ and $ to match start/end of line
	var flags = "m"

	// Turn on case-insensitivity if requested
	var rawFlags = r.URL.Query().Get("flags")
	if strings.Contains(rawFlags, "i") {
		flags += "i"
	}

	re, err := regexp.Compile("(?" + flags + ")" + query)
	if err != nil {
		internal.HTTPError(w, r, 400)
		return
	}
	var grep = Grep{Regexp: re, Stdout: w}
	var iquery = index.RegexpQuery(re.Syntax)
	var count int
	var errors []error
	for _, name := range idxlist {
		ix := index.Open(name)
	perfile:
		for _, fileid := range ix.PostingQuery(iquery) {
			relative := ix.Name(fileid)

			// Handle include and exclude options
			if len(includes) > 0 {
				included := false
				for _, include := range includes {
					if ok, _ := doublestar.Match(include, relative); ok {
						included = true
					} else if ok, _ = doublestar.Match(include+"/**", relative); ok {
						included = true
					}
				}
				if !included {
					continue perfile
				}
			}
			for _, exclude := range excludes {
				if ok, _ := doublestar.Match(exclude, relative); ok {
					continue perfile
				} else if ok, _ = doublestar.Match(exclude+"/**", relative); ok {
					continue perfile
				}
			}

			// Find source file
			prefix := string(relative[0])
			if strings.HasPrefix(relative, "lib") {
				prefix = relative[0:4]
			}
			absolute := filepath.Join(bulkDir, "grep", distro, prefix, relative)
			f, err := os.Open(absolute)
			if err != nil {
				errors = append(errors, err)
				continue
			}

			// Search source file to confirm matches
			n, err := grep.Reader(f, relative)
			count += n
			if err != nil {
				errors = append(errors, err)
			}
		}
	}

	fmt.Fprintf(w, "\nQuery: %q\n", query)
	fmt.Fprintf(w, "Flags: %q  Includes: %v  Excludes: %v\n", flags, includes, excludes)
	fmt.Fprintf(w, "Results: %d\n", count)
	fmt.Fprintf(w, "Time: %s\n", time.Since(start))
	if len(errors) > 0 {
		fmt.Fprintf(w, "Errors:")
		for _, err := range errors {
			fmt.Fprintf(w, " %#v", err)
		}
		fmt.Fprintf(w, "\n")
	}
}
