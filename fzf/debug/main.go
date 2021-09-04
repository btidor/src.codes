package main

import (
	"fmt"
	"net/http"

	"github.com/btidor/src.codes/fzf"
)

const addr = "localhost:7070"

func main() {
	index, err := fzf.LoadIndex("hirsute")
	if err != nil {
		panic(err)
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		index.Handle(w, r, false)
	})

	fmt.Printf("Listening on %s...\n", addr)
	err = http.ListenAndServe(addr, nil)
	panic(err)
}
