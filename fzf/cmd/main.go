// Simple server for local development

package main

import (
	"fmt"
	"net/http"

	"github.com/btidor/src.codes/fzf"
	"github.com/btidor/src.codes/internal"
)

var server fzf.Server

const localAddr = "localhost:7070"

var base = internal.UrlMustParse("https://meta.src.codes")

func main() {
	server = fzf.Server{
		Meta:        base,
		Commit:      "dev",
		ResultLimit: 100,
	}
	server.EnsureIndex("hirsute")

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		server.Handle(w, r, true)
	})

	fmt.Printf("Listening on %s...\n", localAddr)
	err := http.ListenAndServe(localAddr, nil)
	panic(err)
}
