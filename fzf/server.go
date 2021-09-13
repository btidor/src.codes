package fzf

import (
	"container/heap"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/btidor/src.codes/internal"
	"github.com/junegunn/fzf/src/algo"
	"github.com/junegunn/fzf/src/util"
	"github.com/vmihailenco/msgpack/v5"
)

type Server struct {
	Meta        *url.URL
	Commit      string
	Parallelism int
	ResultLimit int

	cache map[string][]Node
}

func (s *Server) EnsureIndex(distro string) bool {
	if s.cache == nil {
		s.cache = make(map[string][]Node)
	}

	if _, found := s.cache[distro]; found {
		return true
	}

	buf := internal.DownloadFile(s.Meta, distro, "paths.fzf")
	dec := msgpack.NewDecoder(buf)
	ct, err := dec.DecodeArrayLen()
	if err != nil {
		panic(err)
	}

	for i := 0; i < ct; i++ {
		b, err := dec.DecodeBytes()
		if err != nil {
			panic(err)
		}

		var node Node
		err = msgpack.Unmarshal(b, &node)
		if err != nil {
			panic(err)
		}

		s.cache[distro] = append(s.cache[distro], node)
	}
	return false
}

func (s *Server) Handle(w http.ResponseWriter, r *http.Request, warm bool) {
	var start = time.Now()

	var parts = strings.SplitN(r.URL.Path, "/", 2)
	if len(parts) < 2 || parts[1] == "" {
		// Request to "/"
		fmt.Fprintf(w, "Hello from fzf@%s!", s.Commit)
		return
	}

	var distro = parts[1]
	data, found := s.cache[distro]
	if !found {
		// Request to "/invalid-distro(/...)?"
		internal.HTTPError(w, r, http.StatusNotFound)
		return
	}

	var query = r.URL.Query().Get("q")
	if query == "" {
		// Request to "/distro" without query
		internal.HTTPError(w, r, http.StatusBadRequest)
		return
	}

	var wg sync.WaitGroup
	var jobs = make(chan Node)
	var results = make(chan Result, s.Parallelism*s.ResultLimit)
	for w := 0; w < s.Parallelism; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			var h = &ResultHeap{}
			heap.Init(h)
			for pkg := range jobs {
				s.score([]rune(query), &pkg, []byte(pkg.Name), h)
			}

			sort.SliceStable(*h, h.Less)

			for i := len(*h) - 1; i >= 0; i-- {
				results <- (*h)[i]
			}
		}()
	}

	for _, pkg := range data {
		jobs <- pkg
	}
	close(jobs)
	wg.Wait()
	close(results)

	var h = &ResultHeap{}
	heap.Init(h)
	for res := range results {
		if len(*h) < s.ResultLimit {
			heap.Push(h, res)
		} else if res.Score > h.Peek().Score {
			heap.Pop(h)
			heap.Push(h, res)
		}
	}

	sort.SliceStable(*h, h.Less)

	for i := len(*h) - 1; i >= 0; i-- {
		entry := (*h)[i]
		fmt.Fprintf(w, "%d %s\n", entry.Score, entry.Line)
	}

	w.Write([]byte("\n"))
	fmt.Fprintf(w, "Query: %#v\n", query)
	if len(*h) >= s.ResultLimit {
		fmt.Fprintf(w, "Results: %d (truncated)\n", len(*h))
	} else {
		fmt.Fprintf(w, "Results: %d\n", len(*h))
	}
	fmt.Fprintf(w, "Time: %s\n", time.Since(start))
	fmt.Fprintf(w, "Warm: %t\n", warm)
}

func (s *Server) score(query []rune, n *Node, pfx []byte, h *ResultHeap) {
	pfx = append(pfx, []byte("/")...)

	// Descend into subdirectories
	for _, c := range n.Children {
		cpfx := append(pfx, []byte(n.Name)...)
		s.score(query, c, cpfx, h)
	}

	// Score each file
	for _, f := range n.Files {
		target := append(pfx, []byte(f)...)
		chars := util.ToChars(target)
		res, _ := algo.FuzzyMatchV1(false, false, true, &chars, query, false, nil)

		if res.Score <= 0 {
			continue
		} else if len(*h) < s.ResultLimit {
			heap.Push(h, Result{res.Score, string(target)})
		} else if res.Score > h.Peek().Score {
			heap.Pop(h)
			heap.Push(h, Result{res.Score, string(target)})
		}
	}
}
