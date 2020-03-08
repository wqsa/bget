package storage

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/pkg/errors"
	"github.com/wqsa/bget/common"
	"github.com/wqsa/bget/meta"
)

type resource struct {
	pieceLength int
	//order by piece index
	files         []*file
	pieceCount    int
	piecesHash    map[int][]byte
	partialPieces map[int][][2]int
}

func newResource(metadata *meta.Torrent) (*resource, error) {
	if !metadata.Vaild() {
		return nil, errors.New("metadata invaild")
	}
	res := new(resource)
	res.pieceLength = metadata.Info.PieceLength
	res.pieceCount = len(metadata.Info.Pieces) / 20
	for i := 0; i < res.pieceCount; i++ {
		res.piecesHash[i] = metadata.Info.Pieces[i*20 : (i+1)*20]
	}
	var begin, end int64
	for _, f := range metadata.Info.Files {
		begin = end
		end = begin + f.Length
		file := new(file)
		file.length = f.Length
		file.beginIndex = int(begin / int64(res.pieceLength))
		file.beginOffet = int(begin % int64(res.pieceLength))
		file.endIndex = int(end / int64(res.pieceLength))
		file.endOffset = int(end % int64(res.pieceLength))
		if file.endOffset > 0 {
			file.endIndex++
		}
		file.path = strings.Join(f.PathUTF8, "/")
		file.path = filepath.ToSlash(file.path)
		res.files = append(res.files, file)
	}
	return res, nil
}

func (r *resource) readBlock(b common.BlockInfo) <-chan *ReadResult {
	file1, file2, bound, err := r.locateFile(b)
	ch := make(chan *ReadResult)
	if err != nil {
		go func() {
			ch <- &ReadResult{Error: err}
		}()
		return ch
	}
	offset := int64(b.Index)*int64(r.pieceLength) + int64(b.Begin-file1.beginOffet)
	if file2 != nil {
		go func() {
			ch1 := file2.readBlock(offset, bound)
			result := <-ch1
			if result.Error != nil {
				ch <- result
				return
			}
			ch2 := file2.readBlock(0, b.Length-bound)
			result = <-ch2
			ch <- result
		}()
		return ch
	}
	return file1.readBlock(offset, bound)
}

func (r *resource) writeBlock(b *common.Block) <-chan WriteResult {
	file1, file2, bound, err := r.locateFile(b.BlockInfo)
	ch := make(chan WriteResult)
	if err != nil {
		go func() {
			ch <- WriteResult{Error: err}
		}()
		return ch
	}
	offset := int64(b.Index)*int64(r.pieceLength) + int64(b.Begin-file1.beginOffet)
	if file2 != nil {
		go func() {
			ch1 := file2.writeBlock(offset, b.Data[:bound])
			w := <-ch1
			if w.Error != nil {
				ch <- w
				return
			}
			ch2 := file2.writeBlock(0, b.Data[bound:])
			w = <-ch2
			ch <- w
		}()
		return ch
	}
	return file1.writeBlock(offset, b.Data[:bound])
}

func (r *resource) locateFile(b common.BlockInfo) (*file, *file, int, error) {
	i := sort.Search(len(r.files), func(i int) bool {
		return b.Index >= r.files[i].beginIndex &&
			b.Index <= r.files[i].endIndex
	})
	f := r.files[i]
	bound := f.endOffset - b.Begin
	if bound < b.Length {
		if i+1 >= len(r.files) {
			return nil, nil, 0, errors.New("out of range")
		}
		return r.files[i], r.files[i+1], bound, nil
	}
	return r.files[i], nil, bound, nil
}
