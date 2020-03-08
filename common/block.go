package common

import "encoding/binary"

type BlockInfo struct {
	Begin  int
	Index  int
	Length int
}

type Block struct {
	BlockInfo
	Data []byte
}

func (b *BlockInfo) Hash() string {
	// 3 * uint32 len
	buf := make([]byte, 3*4)
	binary.BigEndian.PutUint32(buf, uint32(b.Index))
	binary.BigEndian.PutUint32(buf, uint32(b.Begin))
	binary.BigEndian.PutUint32(buf, uint32(b.Length))
	return string(buf)
}
