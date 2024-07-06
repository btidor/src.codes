// Package internal provides useful, private Go helper functions.
package internal

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
)

// URLMustParse parses a URL and panics if an error occurs. It's useful for
// assigning constants.
func URLMustParse(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}

// URLWithPath takes a base URL and a sequence of path components and
// concatenates the two together using path.Join's forward-slash separators.
func URLWithPath(u *url.URL, s ...string) *url.URL {
	c := []string{u.Path}
	c = append(c, s...)

	res := *u
	res.Path = path.Join(c...)
	return &res
}

// HTTPError is for HTTP servers. It responds with the provided status code and
// a simple text-based error like "404 Not Found".
func HTTPError(w http.ResponseWriter, r *http.Request, code int) {
	w.WriteHeader(code)
	fmt.Fprintf(w, "%d %s\n", code, http.StatusText(code))
}

// DownloadFile downloads a URL and panics if an error occurs or if the HTTP
// request returns a non-200 status code. For convenience, callers may pass
// either a complete URL or a base URL followed by a sequence of path segments
// as in URLWithPath.
func DownloadFile(u *url.URL, s ...string) *bytes.Buffer {
	us := URLWithPath(u, s...).String()
	r, err := http.Get(us)
	if err != nil {
		panic(err)
	}
	defer r.Body.Close()

	if r.StatusCode != 200 {
		err = fmt.Errorf("could not download %s: HTTP %s", us, r.Status)
		panic(err)
	}

	var b bytes.Buffer
	b.ReadFrom(r.Body)
	return &b
}

// SaveFile downloads a URL to disk and panics if an error occurs or if the HTTP
// request returns a non-200 status code. The operation is atomic: the file is
// streamed to a temporary file which is then renamed into place. For
// convenience, callers may pass either a complete URL or a base URL followed by
// a sequence of path segments as in URLWithPath.
func SaveFile(dest string, u *url.URL, s ...string) {
	tmp, err := os.CreateTemp(filepath.Dir(dest), "tmp-")
	if err != nil {
		panic(err)
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	us := URLWithPath(u, s...).String()
	r, err := http.Get(us)
	if err != nil {
		panic(err)
	}
	defer r.Body.Close()

	if r.StatusCode != 200 {
		err = fmt.Errorf("could not download %s: HTTP %s", us, r.Status)
		panic(err)
	}

	_, err = io.Copy(tmp, r.Body)
	if err != nil {
		panic(err)
	}

	err = tmp.Close()
	if err != nil {
		panic(err)
	}
	err = os.Rename(tmp.Name(), dest)
	if err != nil {
		panic(err)
	}
}
