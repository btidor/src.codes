package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/btidor/src.codes/fzf"
	"github.com/btidor/src.codes/internal"
)

// TODO: cache invalidation
var server fzf.Server

var base = internal.UrlMustParse("https://meta.src.codes")

// For Vercel (production)
// Note: this file must be in a directory called 'api'

func Handler(w http.ResponseWriter, r *http.Request) {
	commit := os.Getenv("VERCEL_GIT_COMMIT_SHA")[:8]
	if commit == "" {
		err := fmt.Errorf("env var VERCEL_GIT_COMMIT_SHA is not set")
		panic(err)
	}

	server = fzf.Server{
		Meta:        base,
		Commit:      commit,
		ResultLimit: 100,
	}

	warm := server.EnsureIndex("hirsute")
	server.Handle(w, r, warm)
}

// For local devleopment

const localAddr = "localhost:7070"

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
