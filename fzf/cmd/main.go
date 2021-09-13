// A simple server for local development.
package main

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime/pprof"

	"github.com/btidor/src.codes/fzf"
	"github.com/btidor/src.codes/internal"
)

var server fzf.Server

const localAddr = "localhost:7070"

var base = internal.URLMustParse("https://meta.src.codes")

func main() {
	var benchmark = false
	for _, a := range os.Args[1:] {
		if a == "--benchmark" {
			benchmark = true
		} else {
			log.Fatalf("usage: %s [--benchmark]", os.Args[0])
		}
	}

	server = fzf.Server{
		Meta:        base,
		Commit:      "dev",
		Parallelism: 8,
		ResultLimit: 100,
	}
	server.EnsureIndex("hirsute")

	if benchmark {
		// Benchmark mode
		// go tool pprof -http=":8000" cmd.pprof
		f, err := os.Create("cmd.pprof")
		if err != nil {
			panic(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()

		fmt.Println("Benchmarking...")
		for i := 0; i < 10; i++ {
			r := httptest.NewRequest("GET", "http://localhost/hirsute?q=asdf1234", nil)
			w := httptest.NewRecorder()
			server.Handle(w, r, true)
			fmt.Printf("%02d ", i+1)
		}
		fmt.Println()
	} else {
		// Server mode
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			server.Handle(w, r, true)
		})

		fmt.Printf("Listening on %s...\n", localAddr)
		err := http.ListenAndServe(localAddr, nil)
		panic(err)
	}
}
