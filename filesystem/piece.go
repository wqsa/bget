package filesystem

import (
	"bytes"
	"crypto/sha1"
	"errors"
	"time"

	"github.com/wqsa/bget/meta"
	"github.com/gohugoio/hugo/bufferpool"
)

var (
	errInitPeer    = errors.New("there isn't peer")
	ErrPieceCheck  = errors.New("check piece sha1 fail")
	errPieceIndex  = errors.New("piece index out of range")
	errPieceFinish = errors.New("piece has completed")
)

type piece struct {
	*bytes.Buffer
	hash     meta.Hash
	length   int
	complete bool
	atDisk   bool
	timeout  *time.Timer
}

func (p *piece) check() bool {
	if p.Len() != p.length {
		return false
	}
	return sha1.Sum(p.Bytes()) == p.hash
}

func (p *piece) Write(data []byte) (l int, err error) {
	if p.complete {
		return 0, errPieceFinish
	}
	if p.Buffer == nil {
		p.Buffer = bufferpool.GetBuffer()
	}
	l, err = p.Buffer.Write(data)
	if p.Buffer.Len() == p.length {
		if p.check() {
			p.complete = true
			p.timeout.Stop()
		} else {
			bufferpool.PutBuffer(p.Buffer)
			p.Buffer = nil
			return 0, ErrPieceCheck
		}
	}
	return
}

func newPieces(hash []byte) []*piece {
	if len(hash)%meta.HashSize != 0 {
		return nil
	}
	pieces := make([]*piece, len(hash)/meta.HashSize)
	for i := range pieces {
		pieces[i] = new(piece)
		copy(pieces[i].hash[:], hash[i*meta.HashSize:(i+1)*meta.HashSize])
	}
	return pieces
}
