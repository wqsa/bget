package storage

import (
	"github.com/pkg/errors"
	"github.com/wqsa/bget/common"
	"github.com/wqsa/bget/meta"

	lru "github.com/hashicorp/golang-lru"
)

type Storage struct {
	resources map[string]*resource
	cache     *lru.ARCCache
	tempCache blockHeap
}

func NewStorage(cacheSize int) (*Storage, error) {
	s := &Storage{
		resources: make(map[string]*resource),
	}
	var err error
	s.cache, err = lru.NewARC(cacheSize)
	return s, err
}

func (s *Storage) run() {
	for {

	}
}

func (s *Storage) AddResource(metadata *meta.Torrent) error {
	res, err := newResource(metadata)
	if err != nil {
		return err
	}
	s.resources[metadata.Info.Hash.String()] = res
	return nil
}

func (s *Storage) ReadBlock(infoHash string, b common.BlockInfo) <-chan *ReadResult {
	ch := make(chan *ReadResult)
	res, ok := s.resources[infoHash]
	if !ok {
		go func() {
			ch <- &ReadResult{Error: errors.New("no this resource")}
		}()
		return ch
	}
	v, ok := s.cache.Get(b.Hash())
	if ok {
		go func() {
			ch <- &ReadResult{Data: v.([]byte)}
		}()
		return ch
	}
	return res.readBlock(b)
}

func (s *Storage) WriteBlock(infoHash string, b *common.Block) <-chan WriteResult {
	res, ok := s.resources[infoHash]
	if !ok {
		ch := make(chan WriteResult)
		go func() {
			ch <- WriteResult{Error: errors.New("no this resource")}
		}()
		return ch
	}
	s.cache.Add(b.Hash(), b)
	return res.writeBlock(b)
}
