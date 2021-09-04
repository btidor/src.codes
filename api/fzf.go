package api

import (
	"net/http"

	"github.com/btidor/src.codes/fzf"
)

// TODO: invalidation
var index *fzf.Index

func Handler(w http.ResponseWriter, r *http.Request) {
	didLoad := false
	if index == nil {
		i, err := fzf.LoadIndex("hirsute")
		if err != nil {
			panic(err)
		}
		index = i
		didLoad = true
	}
	index.Handle(w, r, didLoad)
}
