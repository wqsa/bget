package storage

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/pkg/errors"
)

const (
	cmdRead = iota
	cmdWrite
)

type ioCMD struct {
	cmd    int
	offset int64
	length int
	data   []byte
}

type file struct {
	*os.File
	path       string
	length     int64
	lastActive time.Time
	beginIndex int
	beginOffet int
	endIndex   int
	endOffset  int
	downloaded int64
	cmdc       chan ioCMD
	resultc    chan interface{}
	closec     chan struct{}
	mu         sync.Mutex
	err        error
}

func (f *file) run() {
	check := time.NewTicker(3 * time.Second)
	for {
		select {
		case <-check.C:
			if time.Since(f.lastActive) > 10*time.Second {
				f.Close()
				f.File = nil
			}
		case c := <-f.cmdc:
			switch c.cmd {
			case cmdRead:
				data := make([]byte, c.length)
				if _, err := f.readAt(data, c.offset); err != nil {
					f.resultc <- &ReadResult{Error: &fileError{path: f.path, offset: c.offset,
						length: c.length, cmd: c.cmd, err: err}}
				} else {
					f.resultc <- &ReadResult{Data: data}
				}
			case cmdWrite:
				if _, err := f.writeAt(c.data, c.offset); err != nil {
					f.resultc <- WriteResult{Error: err}
				} else {
					f.resultc <- WriteResult{}
				}
			}
		case <-f.closec:
			return
		}
	}
}

func (f *file) readBlock(offset int64, length int) <-chan *ReadResult {
	ch := make(chan *ReadResult)
	go func() {
		f.cmdc <- ioCMD{cmd: cmdRead, offset: offset, length: length}
		result := <-f.resultc
		ch <- result.(*ReadResult)
	}()
	return ch
}

func (f *file) writeBlock(offset int64, data []byte) <-chan WriteResult {
	ch := make(chan WriteResult)
	go func() {
		f.cmdc <- ioCMD{cmd: cmdWrite, offset: offset, data: data}
		result := <-f.resultc
		ch <- result.(WriteResult)
	}()
	return nil
}

func (f *file) readAt(b []byte, off int64) (int, error) {
	l, err := f.File.ReadAt(b, off)
	if err != nil && errors.Is(err, os.ErrClosed) {
		if f.File, err = os.Open(f.path); err != nil {
			return l, errors.WithStack(err)
		}
		l, err = f.File.ReadAt(b, off)
	}
	return l, errors.WithStack(err)
}

func (f *file) writeAt(b []byte, off int64) (int, error) {
	l, err := f.File.WriteAt(b, off)
	if err != nil && (errors.Is(err, os.ErrClosed) || errors.Is(err, os.ErrPermission)) {
		if f.File, err = os.Open(f.path); err != nil {
			return l, errors.WithStack(err)
		}
		l, err = f.File.WriteAt(b, off)
	}
	f.downloaded += int64(l)
	return l, errors.WithStack(err)
}

type fileError struct {
	path   string
	offset int64
	length int
	cmd    int
	err    error
}

func (e *fileError) Error() string {
	cmd := ""
	switch e.cmd {
	case cmdRead:
		cmd = "read"
	case cmdWrite:
		cmd = "write"
	default:
		cmd = "unknow cmd"
	}
	return fmt.Sprintf("%v %v, offset: %v, length: %v, err:%v", cmd, e.path, e.offset, e.length, e.err)
}

func (e *fileError) Unwrap() error { return e.err }
