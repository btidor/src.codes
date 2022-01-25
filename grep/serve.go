package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"regexp/syntax"
	"strconv"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar"
	"github.com/btidor/src.codes/internal"
	"github.com/google/codesearch/index"
)

const (
	maxFileSize   = (1 << 20)
	maxFileVisits = 500
	maxContext    = 10
	nl            = '\n'
)

type Index struct {
	*index.Index
	Package string
}

func serve() {
	var indexes = make(map[string][]*Index)
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
		for _, filename := range matches {
			pkg := filepath.Base(filepath.Dir(filename))
			indexes[distro] = append(indexes[distro], &Index{index.Open(filename), pkg})
		}
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

type Globs struct {
	G []string
}

func (gs Globs) MatchPath(path string) bool {
	for _, glob := range gs.G {
		if ok, _ := doublestar.Match(glob, path); ok {
			return true
		} else if ok, _ = doublestar.Match(glob+"/**", path); ok {
			return true
		}
	}
	return false
}

func (gs Globs) CanMatchPrefix(part string) bool {
	prefix := part + "/"
	for _, glob := range gs.G {
		var gfx string
		if i := strings.IndexRune(glob, '*'); i < 0 {
			gfx = glob
		} else {
			gfx = glob[:i]
		}
		if len(gfx) <= len(prefix) && strings.HasPrefix(prefix, gfx) {
			return true
		} else if strings.HasPrefix(gfx, prefix) {
			return true
		}
	}
	return false
}

func (gs Globs) MustMatchPrefix(part string) bool {
	for _, glob := range gs.G {
		components := strings.Split(glob, "/")
		// fmt.Printf("%s %s %s\n", part, glob, components)
		for i, comp := range components[:len(components)-1] {
			if comp == "**" {
				continue
			} else if ok, _ := doublestar.Match(comp, part); ok {
				if components[i+1] == "**" {
					return true
				}
			}
			break
		}
	}
	return false
}

type grepHandler struct {
	indexes map[string][]*Index
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
	ixlist, ok := g.indexes[distro]
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

	var after = r.URL.Query().Get("after")

	var includes = Globs{r.URL.Query()["include"]}
	var excludes = Globs{r.URL.Query()["exclude"]}

	var context int
	if c := r.URL.Query().Get("context"); c != "" {
		if i, err := strconv.ParseInt(c, 10, 64); err == nil {
			if i < 0 {
				context = 0
			} else if i > maxContext {
				context = maxContext
			} else {
				context = int(i)
			}
		}
	}

	// Set up regexp flags. Compile(), and also Javascript, default to Perl
	// matching options, so we should start with that flag. Then add `m` to
	// always allow ^ and $ to match the start/end of the line.
	var intFlags = syntax.Perl & ^syntax.OneLine & syntax.NonGreedy
	var charFlags = "mU"

	// Turn on case-insensitivity if requested
	var rawFlags = r.URL.Query().Get("flags")
	if strings.Contains(rawFlags, "i") {
		intFlags |= syntax.FoldCase
		charFlags += "i"
	}
	if strings.Contains(rawFlags, "s") {
		intFlags |= syntax.DotNL
		charFlags += "s"
	}

	rsyntax, err := syntax.Parse(query, intFlags)
	if err != nil {
		internal.HTTPError(w, r, 400)
		return
	}
	rsyntax = rsyntax.Simplify()

	re, err := regexp.Compile("(?" + charFlags + ")" + rsyntax.String())
	if err != nil {
		internal.HTTPError(w, r, 400)
		return
	}

	var grep = Grep{Context: context, Regexp: re, Stdout: w}
	var iquery = index.RegexpQuery(rsyntax)
	var count, files int
	var resume string
	var errors []error
overall:
	for _, ix := range ixlist {
		if len(includes.G) > 0 && !includes.CanMatchPrefix(ix.Package) {
			continue
		} else if excludes.MustMatchPrefix(ix.Package) {
			continue
		} else if after != "" && ix.Package <= after {
			continue
		}

	perfile:
		for _, fileid := range ix.PostingQuery(iquery) {
			relative := ix.Name(fileid)

			// Handle include and exclude options
			if len(includes.G) > 0 && !includes.MatchPath(relative) {
				continue perfile
			} else if excludes.MatchPath(relative) {
				continue perfile
			} else if after != "" && relative <= after {
				continue perfile
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
			files++
			if err != nil {
				errors = append(errors, err)
			}
			if files >= maxFileVisits {
				resume = relative
				break overall
			}
		}
	}

	fmt.Fprintf(w, "\nQuery:\t%q\n", query)
	if after != "" {
		fmt.Fprintf(w, "After:\t%s\n", after)
	}
	fmt.Fprintf(w, "Flags:\t%q\tContext:\t%d\tIncludes:\t%v\tExcludes:\t%v\n",
		charFlags, context, includes, excludes)
	fmt.Fprintf(w, "Count:\t%d\n", count)
	fmt.Fprintf(w, "Time:\t%s\n", time.Since(start))
	if resume != "" {
		fmt.Fprintf(w, "Resume:\t%s\n", resume)
	}
	if len(errors) > 0 {
		fmt.Fprintf(w, "Errors:\t")
		for _, err := range errors {
			fmt.Fprintf(w, "%#v ", err)
		}
		fmt.Fprintf(w, "\n")
	}
}
