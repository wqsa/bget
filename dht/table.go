package dht

import (
	"time"

	"github.com/pkg/errors"
)

const (
	maxDistance = 160

	checkTime = 15 * time.Minute
)

type bucket struct {
	nodes map[int][]*node
	count int
	begin int
	end   int
	taskC chan func(*node) error
	stopC chan struct{}
}

func newBucket(begin, end int) *bucket {
	b := &bucket{
		nodes: make(map[int][]*node),
		begin: begin,
		end:   end,
		taskC: make(chan func(*node) error, bucketNum),
		stopC: make(chan struct{}),
	}
	go func() {
		for {
			select {
			case f := <-b.taskC:
				for d, ns := range b.nodes {
					for i, n := range ns {
						if f(n) != nil {
							b.nodes[d] = append(b.nodes[d][:i], b.nodes[d][i+1:]...)
							if len(b.nodes[d]) == 0 {
								delete(b.nodes, d)
							}
						}
					}
				}
			case <-b.stopC:
				return
			}
		}
	}()
	return b
}

func (b *bucket) full() bool {
	return b.count == bucketNum
}

func (b *bucket) addNode(distance int, n *node) error {
	if b.full() {
		return errors.New("bucket is full")
	}
	if distance < b.begin || distance > b.end {
		return errors.New("not in range")
	}

	if _, ok := b.nodes[distance]; !ok {
		b.nodes[distance] = make([]*node, 0, 1)
	}
	b.nodes[distance] = append(b.nodes[distance], n)
	b.count++
	return nil
}

func (b *bucket) split() (*bucket, error) {
	max := 0
	for i := b.begin; i <= b.end && max < bucketNum; i++ {
		max += (2 << uint(i))
	}
	if max < bucketNum {
		return nil, errors.New("can't split")
	}
	nb := newBucket(b.end/2, b.end)
	b.end /= 2
	for d, ns := range b.nodes {
		if d > b.end {
			for _, n := range ns {
				nb.addNode(d, n)
			}
		}
	}
	return nb, nil
}

func (b *bucket) addTask(task func(*node) error) {
	b.taskC <- task
}

type table struct {
	own     *node
	count   int
	buckets []*bucket
}

func newTable(own *node) *table {
	b := newBucket(0, 160)
	b.addNode(0, own)
	return &table{
		own:     own,
		buckets: []*bucket{b},
	}
}

func (t *table) addNode(n *node) {
	d := t.own.distance(n)
	b := t.findBucket(d)
	for b.full() {
		nb, err := b.split()
		if err != nil {
			return
		}
		t.buckets = append(t.buckets, nb)
		b = t.findBucket(d)
	}
	b.addNode(d, n)
	t.count++
}

func (t *table) findBucket(distance int) *bucket {
	for _, b := range t.buckets {
		if distance >= b.begin && distance <= b.end {
			return b
		}
	}
	return nil
}

func (t *table) foreach(f func(n *node) error) {
	for _, b := range t.buckets {
		b.addTask(f)
	}
}
