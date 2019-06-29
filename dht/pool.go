package dht

import (
	"bytes"
	"sync"
)

const pkgSize = 1024

var pkgPool sync.Pool

func init() {
	pkgPool = sync.Pool{
		New: func() interface{} {
			return bytes.NewBuffer(make([]byte, pkgSize))
		},
	}
}

func getBuffer() *bytes.Buffer {
	return pkgPool.Get().(*bytes.Buffer)
}

func putBuffer(buff *bytes.Buffer) {
	pkgPool.Put(buff)
}
