// Useful, module-internal Go helper functions
package internal

import (
	"fmt"
	"net/http"
	"net/url"
	"path"
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
