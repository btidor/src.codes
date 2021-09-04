package fzf

type Result struct {
	Score int
	Line  *stri
	old := *r
	n := len(old)
	x := old[n-1]
	*r = old[0 : n-1]
	return x
}

func (r ResultHeap) Peek() Result {
	return r[len(r)-1]
}
