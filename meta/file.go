package meta

import (
	"bytes"
)

const (
	HashSize = 20
)

//Piece is a block of a file
type Piece struct {
	bytes.Buffer
	hash     Hash
	download int
}

//File is file
type File struct {
	path        []string
	length      int64
	beginPiece  int64
	beginOffset int
}
