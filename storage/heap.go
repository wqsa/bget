package storage

import "github.com/wqsa/bget/common"

type blockHeap []*common.Block

func (h blockHeap) Len() int { return len(h) }

func (h blockHeap) Less(i, j int) bool {
	if h[i].Index != h[j].Index {
		return h[i].Index < h[j].Index
	} else if h[i].Begin != h[j].Begin {
		return h[i].Begin < h[j].Begin
	} else {
		return h[i].Length < h[j].Length
	}
}

func (h blockHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h *blockHeap) Push(x interface{}) {
	item := x.(*common.Block)
	*h = append(*h, item)
}

func (h *blockHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	old[n-1] = nil // avoid memory leak
	*h = old[0 : n-1]
	return item
}
