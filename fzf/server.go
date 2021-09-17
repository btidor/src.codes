package fzf

import (
	"container/heap"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/btidor/src.codes/internal"
	"github.com/vmihailenco/msgpack/v5"
)

type Server struct {
	Meta        *url.URL
	Commit      string
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

		// Strip leading '/'
		node.Name = node.Name[1:]

		s.cache[distro] = append(s.cache[distro], node)
	}
	return false
}

func (s *Server) Handle(w http.ResponseWriter, r *http.Request, warm bool) {
	var start = time.Now()

	var parts = strings.SplitN(r.URL.Path, "/", 2)
	if len(parts) < 2 || parts[1] == "" {
		// Request to "/"
		fmt.Fprintf(w, "Hello from fzf@%s!\n", s.Commit)
		c, err := ioutil.ReadFile("/proc/cpuinfo")
		if err != nil {
			panic(err)
		}
		fmt.Fprintf(w, "\n%s\n", c)
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

	// TODO: enforce max query length

	var h = &ResultHeap{}
	heap.Init(h)

	matcher := NewMatcher(query)

	for _, pkg := range data {
		s.score(matcher.Advance(pkg.Name), pkg, h)
	}

	sort.SliceStable(*h, h.Less)

	for i := len(*h) - 1; i >= 0; i-- {
		entry := (*h)[i]
		fmt.Fprintf(w, "%d %s\n", entry.Score, entry.Line)
	}

	// TODO: profile and speed up
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

func (s *Server) score(matcher Matcher, n Node, h *ResultHeap) {
	// Descend into subdirectories
	for _, c := range n.Children {
		s.score(matcher.Advance(c.Name), c, h)
	}

	// Score each file
	for _, f := range n.Files {
		score := matcher.Score(f)

		if score <= 0 {
			continue
		} else if len(*h) < s.ResultLimit {
			heap.Push(h, Result{score, matcher.Target(f)})
		} else if score > h.Peek().Score {
			heap.Pop(h)
			heap.Push(h, Result{score, matcher.Target(f)})
		}
	}
}
