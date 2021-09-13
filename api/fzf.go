package api

import (
	"fmt"
	"net/http"
	"os"

	"github.com/btidor/src.codes/fzf"
	"github.com/btidor/src.codes/internal"
)

// TODO: cache invalidation
var server *fzf.Server

var base = internal.URLMustParse("https://meta.src.codes")

func Handler(w http.ResponseWriter, r *http.Request) {
	commit := os.Getenv("VERCEL_GIT_COMMIT_SHA")[:8]
	if commit == "" {
		err := fmt.Errorf("env var VERCEL_GIT_COMMIT_SHA is not set")
		panic(err)
	}

	if server == nil {
		server = &fzf.Server{
			Meta:        base,
			Commit:      commit,
			Parallelism: 8,
			ResultLimit: 100,
		}
	}

	warm := server.EnsureIndex("hirsute")
	server.Handle(w, r, warm)
}
