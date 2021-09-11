package fzf

import (
	"bufio"
	"container/heap"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/btidor/src.codes/internal"
	"github.com/junegunn/fzf/src/algo"
	"github.com/junegunn/fzf/src/util"
)

type Server struct {
	Meta        *url.URL
	Commit      string
	ResultLimit int
	data        map[string][]string
}

func (s *Server) EnsureIndex(distribution string) bool {
	if s.data == nil {
		s.data = make(map[string][]string)
	}

	if _, found := s.data[distribution]; found {
		return true
	}

	// TODO: adjust URLs, add asset fingerprinting
	url := internal.URLWithPath(s.Meta, distribution+".fzf")
	resp, err := http.Get(url.String())
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		s.data[distribution] = append(s.data[distribution], line)
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

	var distribution = parts[1]
	data, found := s.data[distribution]
	if !found {
		// Request to "/invalid-distribution(/...)?"
		internal.HTTPError(w, r, http.StatusNotFound)
		return
	}

	var query = r.URL.Query().Get("q")
	if query == "" {
		// Request to "/distribution" without query
		internal.HTTPError(w, r, http.StatusBadRequest)
		return
	}

	var h = &ResultHeap{}
	heap.Init(h)

	for j := range data {
		chars := util.ToChars([]byte(data[j]))
		res, _ := algo.FuzzyMatchV1(false, false, true, &chars, []rune(query), false, nil)

		if res.Score <= 0 {
			continue
		} else if len(*h) < s.ResultLimit {
			heap.Push(h, Result{res.Score, &data[j]})
		} else if res.Score > h.Peek().Score {
			heap.Pop(h)
			heap.Push(h, Result{res.Score, &data[j]})
		}
	}

	sort.SliceStable(*h, h.Less)

	for i := len(*h) - 1; i >= 0; i-- {
		entry := (*h)[i]
		fmt.Fprintf(w, "%d %s\n", entry.Score, *entry.Line)
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
