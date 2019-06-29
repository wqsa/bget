package filesystem

import (
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wqsa/bget/common/bitmap"
	"github.com/wqsa/bget/meta"

	"github.com/google/logger"

	"github.com/gohugoio/hugo/bufferpool"
)

//Block is minimum unit for transport
type Block struct {
	Index int
	Begin int
	Data  []byte
}

//FileSystem manager data for memory and disk
type FileSystem struct {
	path        string
	files       []file
	pieceNum    int
	pieceLength int
	diskMap     *bitmap.Bitmap
}

//NewFileSystem return a FileSystem instance
func NewFileSystem(torrent *meta.Torrent, path string) *FileSystem {
	if len(torrent.Info.Pieces)%20 != 0 {
		return nil
	}
	if len(path) == 0 {
		path = "./"
	}
	if path[len(path)-1] != '/' || path[len(path)-1] != '\\' {
		path = path + "/"
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return nil
	}
	fl := &FileSystem{path: path, pieceNum: len(torrent.Info.Pieces) / 20, files: make([]file,
		0, len(torrent.Info.Files)), pieceLength: torrent.Info.PieceLength}
	fl.diskMap = bitmap.NewEmptyBitmap(fl.pieceNum)
	begin, end, offset := 0, 0, 0
	for _, f := range torrent.Info.Files {
		p := ""
		if len(f.PathUTF8) != 0 {
			p = strings.Join(f.PathUTF8, "/")
		} else if len(f.Path) != 0 {
			p = strings.Join(f.Path, "/")
		} else {
			return nil
		}
		end = begin + int((f.Length-1)/int64(torrent.Info.PieceLength)+1)
		if end*20 > len(torrent.Info.Pieces) {
			return nil
		}
		fl.files = append(fl.files, file{
			begin:       begin,
			beginOffset: offset,
			pieces:      newPieces(torrent.Info.Pieces[begin*20 : end*20]),
			path:        p,
			length:      f.Length,
			need:        f.Download,
		})
		offset = int(f.Length % int64(torrent.Info.PieceLength))
	}
	return fl
}

//Need return the need bitmap
func (fs *FileSystem) Need() *bitmap.Bitmap {
	n := bitmap.NewEmptyBitmap(fs.pieceNum)
	for _, f := range fs.files {
		if f.need {
			n.SetRangeBitOn(f.begin, f.begin+int(f.length-1)/fs.pieceLength)
		}
	}
	return n
}

func (fs *FileSystem) queryFile(index int) *file {
	i := sort.Search(len(fs.files), func(i int) bool {
		return fs.files[i].begin <= index && fs.files[i].begin+len(fs.files[i].pieces) >= index
	})
	if i == len(fs.files) {
		return nil
	}
	return &(fs.files[i])
}

func (fs *FileSystem) queryPiece(index int) *piece {
	f := fs.queryFile(index)
	if f == nil {
		return nil
	}
	return f.pieces[index-f.begin]
}

//SavePiece save a block to correspond piece
func (fs *FileSystem) SavePiece(index, begin int, data []byte) (int, error) {
	f := fs.queryFile(index)
	if f == nil {
		return 0, ErrInvaildIndex
	}
	p := f.pieces[index-f.begin]
	if p.Buffer == nil {
		p.Buffer = bufferpool.GetBuffer()
	}
	if p.Len() != begin {
		return 0, ErrWrongOrder
	}
	length, err := p.Write(data)
	if err != nil {
		return 0, err
	}
	logger.Infof("save piece %v, now have:%v\n", index, p.Len())
	if f.download += int64(length); f.isFinish() {
		logger.Infof("file:%v finish\n", f.path[len(f.path)-1])
		f.save()
	}
	if begin+length >= fs.pieceLength {
		f.freeMemory += int64(fs.pieceLength)
	}
	return length, nil
}

//RequestData get demand block from memory or disk
func (fs *FileSystem) RequestData(index, begin, length int, blockc chan *Block) {
	if begin+length > fs.pieceLength {
		return
	}
	f := fs.queryFile(index)
	if f == nil || !f.pieces[index-f.begin].complete {
		return
	}
	b := &Block{Index: index, Begin: begin}
	p := f.pieces[index-f.begin]
	if p.Buffer != nil {
		b.Data = p.Bytes()[begin : begin+length]
		blockc <- b
		return
	}
	p.Buffer = bufferpool.GetBuffer()
	s := io.NewSectionReader(f.fd, int64((index-f.begin)*fs.pieceLength), int64(fs.pieceLength))
	if s == nil {
		return
	}
	_, err := p.ReadFrom(s)
	if err != nil || !p.check() {
		logger.Fatalln("read data from disk error!")
	}
	b.Data = p.Bytes()[begin : begin+length]
	blockc <- b
	return
}

//Dump save data from memory to disk
func (fs *FileSystem) Dump() (int64, error) {
	length := int64(0)
	for _, f := range fs.files {
		w, err := f.save()
		length += w
		for i, p := range f.pieces {
			if p.atDisk {
				if err = fs.diskMap.SetBitOn(f.begin + i); err != nil {
					panic(err.Error())
				}
			}
		}
		if err != nil {
			return length, err
		}
	}
	return length, nil
}

func (fs *FileSystem) cancel() error {
	return nil
}
