package fzf

type Result struct {
	Score int
	Line  string
}

type ResultHeap []Result

func (r ResultHeap) Len() int           { return len(r) }
func (r ResultHeap) Less(i, j int) bool { return r[i].Score < r[j].Score }
func (r ResultHeap) Swap(i, j int)      { r[i], r[j] = r[j], r[i] }

func (r *ResultHeap) Push(x interface{}) {
	*r = append(*r, x.(Result))
}

func (r *ResultHeap) Pop() interface{} {
	old := *r
	n := len(old)
	x := old[n-1]
	*r = old[0 : n-1]
	return x
}

func (r ResultHeap) Peek() Result {
	return r[len(r)-1]
}
