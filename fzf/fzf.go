package fzf

import (
	"bufio"
	"container/heap"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/junegunn/fzf/src/algo"
	"github.com/junegunn/fzf/src/util"
)

const ResultLimit = 100

type Index struct {
	data map[string][]string
}

func LoadIndex(distribution string) (*Index, error) {
	var data = make(map[string][]string)

	// TODO: update URLs, fingerprint
	resp, err := http.Get("https://meta.src.codes/" + distribution + ".fzf")
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		data[distribution] = append(data[distribution], line)
	}
	return &Index{data}, nil
}

func (idx Index) Handle(w http.ResponseWriter, r *http.Request, didLoad bool) {
	var start = time.Now()

	parts := strings.SplitN(r.URL.Path, "/", 3)
	if len(parts) < 2 || parts[1] == "" {
		// Request to "/"
		fmt.Fprintf(w, "Hello from fzf@%s!", gitCommit())
		return
	}

	var distribution = parts[1]
	data, found := idx.data[distribution]
	if !found {
		// Request to "/invalid-distribution(/...)?"
		httpError(w, r, http.StatusNotFound)
		return
	} else if len(parts) < 3 {
		// Request to "/distribution"
		fmt.Fprintf(w, "Distribution: %s", distribution)
		return
	}

	var query = parts[2]
	var count = 0

	var h = &ResultHeap{}
	heap.Init(h)

	for j := range data {
		chars := util.ToChars([]byte(data[j]))
		res, _ := algo.FuzzyMatchV1(false, false, true, &chars, []rune(query), false, nil)

		if res.Score <= 0 {
			continue
		} else if len(*h) < ResultLimit {
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
	fmt.Fprintf(w, "\nTime: %s\n", time.Since(start))
	fmt.Fprintf(w, "Query: %s\n", query)
	if count >= ResultLimit {
		fmt.Fprintf(w, "Results: %d (truncated)\n", count)
	} else {
		fmt.Fprintf(w, "Results: %d\n", count)
	}
	fmt.Fprintf(w, "Full index load: %t\n", didLoad)
}

func httpError(w http.ResponseWriter, r *http.Request, code int) {
	w.WriteHeader(code)
	fmt.Fprintf(w, "%d %s", code, http.StatusText(code))
}

func gitCommit() string {
	if val := os.Getenv("VERCEL_GIT_COMMIT_SHA"); val != "" {
		return val[:8]
	} else {
		return "dev"
	}
}
