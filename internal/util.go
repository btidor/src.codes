// Useful, module-internal Go helper functions
package internal

import (
	"fmt"
	"net/http"
	"net/url"
	"path"
)

func UrlMustParse(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}

func UrlWithPath(u *url.URL, s ...string) *url.URL {
	c := []string{u.Path}
	c = append(c, s...)

	u2 := *u
	u2.Path = path.Join(c...)
	return &u2
}

func HttpError(w http.ResponseWriter, r *http.Request, code int) {
	w.WriteHeader(code)
	fmt.Fprintf(w, "%d %s\n", code, http.StatusText(code))
}
