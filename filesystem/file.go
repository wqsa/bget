package filesystem

import (
	"errors"
	"log"
	"os"

	"github.com/gohugoio/hugo/bufferpool"
)

const (
	statusIgnore = iota
	statusDownload
	statusFinish
)

//filesystem error
var (
	ErrInvaildIndex = errors.New("invaild index")
	ErrWrongOrder   = errors.New("wrong order")
)

type file struct {
	begin       int
	beginOffset int
	length      int64
	pieceLength int
	path        string
	pieces      []*piece
	download    int64
	fd          *os.File
	status      int
	freeMemory  int64
	need        bool
}

func (f *file) addDownload(length int) {
	f.download += int64(length)
	if f.download >= f.length {
		f.status = statusFinish
	}
}

func (f *file) isFinish() bool {
	return f.download >= f.length
}

func (f *file) create() (err error) {
	log.Printf("create file path:%v, length:%v", f.path, f.length)
	f.fd, err = os.Create(f.path)
	if err != nil {
		return
	}
	err = f.fd.Truncate(f.length)
	if err != nil {
		return
	}
	_, err = f.fd.Seek(0, 0)
	if err != nil {
		return
	}
	return
}

func (f *file) save() (l int64, err error) {
	if f.freeMemory < 1024*1024 {
		return
	}
	if f.fd == nil {
		f.fd, err = os.OpenFile(f.path, os.O_WRONLY, 0)
		if os.IsNotExist(err) {
			err = f.create()
		}
		if err != nil {
			return
		}
	}
	if f.pieces[0].complete && !f.pieces[0].atDisk {
		w, err := f.fd.Write(f.pieces[0].Bytes()[f.beginOffset:])
		l += int64(w)
		if err != nil {
			return l, err
		}
		f.pieces[0].atDisk = true
		bufferpool.PutBuffer(f.pieces[0].Buffer)
		f.pieces[0].Buffer = nil
	} else {
		f.fd.Seek(int64(f.pieceLength-f.beginOffset), 1)
	}
	pieceNum := len(f.pieces)
	for i := 1; i < pieceNum-1; i++ {
		if f.pieces[i].complete && !f.pieces[i].atDisk {
			w, err := f.pieces[i].WriteTo(f.fd)
			l += int64(w)
			if err != nil {
				return l, err
			}
			f.pieces[i].atDisk = true
			bufferpool.PutBuffer(f.pieces[i].Buffer)
			f.pieces[i].Buffer = nil
		} else {
			_, err := f.fd.Seek(int64(f.pieceLength), 1)
			if err != nil {
				return l, err
			}
		}
	}
	if pieceNum > 1 {
		endSize := f.length - int64((f.pieceLength - f.beginOffset)) - int64(f.pieceLength*(pieceNum-1))
		if f.pieces[pieceNum-1].complete && !f.pieces[pieceNum-1].atDisk {
			w, err := f.fd.Write(f.pieces[pieceNum-1].Bytes()[:endSize])
			l += int64(w)
			if err != nil {
				return l, err
			}
			//if piece is divied
			if int(endSize) == f.pieces[pieceNum-1].Len() {
				f.pieces[pieceNum-1].atDisk = true
				bufferpool.PutBuffer(f.pieces[pieceNum-1].Buffer)
				f.pieces[pieceNum-1].Buffer = nil
			}
		}
	}
	f.freeMemory -= l
	return
}
